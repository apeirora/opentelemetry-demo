//go:build audit

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-feature/go-sdk/openfeature"

	flags "github.com/open-telemetry/opentelemetry-demo/src/checkout/flags"
)

var (
	auditEndpoint string
	auditClient   *http.Client
)

func initAuditFromEnv() error {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_AUDITLOG_ENDPOINT"))
	if endpoint == "" {
		return nil
	}
	auditEndpoint = endpoint
	auditClient = &http.Client{Timeout: 10 * time.Second}
	logger.Info("audit logging enabled (raw OTLP to sidecar)", slog.String("endpoint", endpoint))
	return nil
}

func shutdownAudit(context.Context) {}

func emitCheckoutAudit(ctx context.Context, eventName, action, userID, targetID, outcome string, details map[string]any) {
	if auditEndpoint == "" || auditClient == nil {
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
	nanos := fmt.Sprintf("%d", now.UnixNano())
	actionUpper := strings.ToUpper(action)

	attrs := []map[string]any{
		otlpStr("audit.record.id", recordID),
		otlpStr("audit.event.name", eventName),
		otlpStr("audit.actor.id", userID),
		otlpStr("audit.actor.type", "user"),
		otlpStr("audit.action", actionUpper),
		otlpStr("audit.outcome", outcome),
		otlpStr("audit.schema.version", "1.0"),
	}
	if targetID != "" {
		attrs = append(attrs,
			otlpStr("audit.target.id", targetID),
			otlpStr("audit.target.type", "order"),
		)
	}

	payload := map[string]any{
		"resourceLogs": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						otlpStr("service.name", "checkout"),
					},
				},
				"scopeLogs": []map[string]any{
					{
						"scope": map[string]any{
							"name": "checkout.audit",
						},
						"logRecords": []map[string]any{
							{
								"timeUnixNano":         nanos,
								"observedTimeUnixNano": nanos,
								"severityNumber":       9,
								"severityText":         "INFO",
								"body":                 map[string]any{"stringValue": string(body)},
								"attributes":           attrs,
							},
						},
					},
				},
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("audit OTLP marshal failed", slog.Any("error", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, auditEndpoint, bytes.NewReader(raw))
	if err != nil {
		logger.Warn("audit request create failed", slog.Any("error", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := auditClient.Do(req)
	if err != nil {
		logger.Warn("audit emit failed",
			slog.String("event", eventName),
			slog.String("record_id", recordID),
			slog.Any("error", err),
		)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		logger.Warn("audit emit failed",
			slog.String("event", eventName),
			slog.Int("status", resp.StatusCode),
			slog.String("record_id", recordID),
		)
		return
	}
	if resp.StatusCode == http.StatusAccepted {
		logger.Info("audit record accepted for retry",
			slog.String("event", eventName),
			slog.Int("status", resp.StatusCode),
			slog.String("record_id", recordID),
		)
	}
}

func otlpStr(key, value string) map[string]any {
	return map[string]any{
		"key":   key,
		"value": map[string]any{"stringValue": value},
	}
}
