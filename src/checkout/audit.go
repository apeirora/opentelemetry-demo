//go:build audit

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/open-feature/go-sdk/openfeature"
	"go.opentelemetry.io/otel/log"
	auditlog "go.opentelemetry.io/otel/sdk/auditlog"
	"go.opentelemetry.io/otel/sdk/auditlog/otlpexport"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	flags "github.com/open-telemetry/opentelemetry-demo/src/checkout/flags"
)

var (
	auditProvider *auditlog.AuditLoggerProvider
	auditLogger   auditlog.AuditLogger
)

// initAuditFromEnv configures the checkout audit SDK when OTEL_AUDITLOG_ENDPOINT is set.
// It wires OTLP HTTP export to otel-collector-audit and HMAC signing for audit records.
func initAuditFromEnv() error {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_AUDITLOG_ENDPOINT"))
	if endpoint == "" {
		return nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse OTEL_AUDITLOG_ENDPOINT: %w", err)
	}
	if u.Scheme == "" {
		u, err = url.Parse("http://" + endpoint)
		if err != nil {
			return fmt.Errorf("parse OTEL_AUDITLOG_ENDPOINT: %w", err)
		}
	}
	if u.Host == "" {
		return fmt.Errorf("OTEL_AUDITLOG_ENDPOINT missing host")
	}

	opts := []otlpexport.Option{otlpexport.WithEndpoint(u.Host)}
	switch u.Scheme {
	case "http":
		opts = append(opts, otlpexport.WithInsecure())
	case "https":
	default:
		return fmt.Errorf("OTEL_AUDITLOG_ENDPOINT unsupported scheme %q", u.Scheme)
	}
	if p := strings.TrimSuffix(u.EscapedPath(), "/"); p != "" {
		opts = append(opts, otlpexport.WithURLPath(p))
	}

	exporter, err := otlpexport.NewHTTP(context.Background(), opts...)
	if err != nil {
		return fmt.Errorf("audit OTLP exporter: %w", err)
	}

	store := auditlog.NewAuditLogInMemoryStore()
	builder, err := auditlog.NewAuditLogProcessorBuilder(exporter, store)
	if err != nil {
		return fmt.Errorf("audit processor builder: %w", err)
	}
	processor, err := builder.
		SetWaitOnExport(true).
		SetExporterTimeout(10 * time.Second).
		Build()
	if err != nil {
		return fmt.Errorf("audit processor: %w", err)
	}

	key, keyErr := auditlog.HMACVerificationKeyFromEnvironment()
	if keyErr != nil {
		return fmt.Errorf("load audit HMAC key: %w", keyErr)
	}
	if len(key) == 0 {
		return fmt.Errorf("audit HMAC key required: set OTEL_AUDITLOG_HMAC_KEY or OTEL_AUDITLOG_HMAC_KEY_FILE")
	}

	auditProvider = auditlog.NewAuditLoggerProvider(
		auditlog.WithAuditRecordProcessor(processor),
		auditlog.WithAuditHashAlgorithm("sha256"),
		auditlog.WithAuditHMACVerificationKey(key),
		auditlog.WithAuditRecordSigning(auditlog.AuditIntegrityHMAC, auditlog.AuditSignContentMeta),
		auditlog.WithAuditExportIntegrity(auditlog.AuditIntegrityHMAC),
	)
	auditLogger = auditProvider.Logger("checkout")
	logger.Info("audit logging enabled", slog.String("endpoint", endpoint))
	return nil
}

// shutdownAudit flushes and shuts down the audit provider on service exit.
func shutdownAudit(ctx context.Context) {
	if auditProvider == nil {
		return
	}
	if err := auditProvider.Shutdown(ctx); err != nil {
		logger.Error("audit provider shutdown failed", slog.Any("error", err))
	}
}

// emitCheckoutAudit emits a signed audit record when the auditLogging feature flag is enabled.
func emitCheckoutAudit(ctx context.Context, eventName, action, userID, targetID, outcome string, details map[string]any) {
	if auditLogger == nil {
		return
	}
	if !flags.AuditLogging.Value(ctx, openfeature.EvaluationContext{}) {
		return
	}

	recordID := uuid.NewString()
	body, err := json.Marshal(details)
	if err != nil {
		logger.Warn("audit body marshal failed", slog.Any("error", err))
		return
	}

	now := time.Now().UTC()
	base := newAuditBaseRecord()
	base.SetTimestamp(now)
	base.SetObservedTimestamp(now)
	base.SetSeverity(log.SeverityInfo)
	base.SetBody(log.StringValue(string(body)))

	actionUpper := strings.ToUpper(action)
	auditAttrs := []log.KeyValue{
		log.String("audit.record.id", recordID),
		log.String("audit.actor.id", userID),
		log.String("audit.actor.type", "user"),
		log.String("audit.action", actionUpper),
		log.String("audit.outcome", outcome),
		log.String("audit.schema.version", "1.0"),
	}
	if targetID != "" {
		auditAttrs = append(auditAttrs,
			log.String("audit.target.id", targetID),
			log.String("audit.target.type", "order"),
		)
	}
	base.AddAttributes(auditAttrs...)

	rec := auditlog.AuditRecord{
		Record:        base,
		EventName:     eventName,
		Actor:         log.StringValue(userID),
		ActorType:     "user",
		Action:        actionUpper,
		Resource:      log.StringValue(targetID),
		TargetID:      targetID,
		Outcome:       outcome,
		RecordID:      recordID,
		SchemaVersion: "1.0",
	}
	if targetID != "" {
		rec.TargetType = "order"
	}

	result := auditLogger.EmitWithResult(ctx, rec)
	if result.StatusCode >= 400 {
		logger.Warn("audit emit failed",
			slog.String("event", eventName),
			slog.Int("status", result.StatusCode),
			slog.String("reason", result.Reason),
			slog.String("record_id", recordID),
		)
	}
}

// newAuditBaseRecord creates a base log record with unlimited attribute limits for audit payloads.
func newAuditBaseRecord() auditlog.Record {
	r := new(sdklog.Record)
	setSDKRecordField(r, "attributeValueLengthLimit", -1)
	setSDKRecordField(r, "attributeCountLimit", -1)
	return *r
}

// setSDKRecordField sets unexported sdklog.Record fields via reflection.
func setSDKRecordField(r *sdklog.Record, name string, value any) {
	rVal := reflect.ValueOf(r).Elem()
	rf := rVal.FieldByName(name)
	rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
	rf.Set(reflect.ValueOf(value))
}
