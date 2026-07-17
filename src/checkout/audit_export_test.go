//go:build audit

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/log"
	auditlog "go.opentelemetry.io/otel/sdk/auditlog"
)

type captureExporter struct {
	batches [][]auditlog.Record
}

func (c *captureExporter) Export(_ context.Context, records []auditlog.Record) (auditlog.ExportResult, error) {
	cp := make([]auditlog.Record, len(records))
	copy(cp, records)
	c.batches = append(c.batches, cp)
	return auditlog.ExportResult{Receipts: auditlog.ReceiptsFromRecords(records)}, nil
}

func (c *captureExporter) Shutdown(context.Context) error  { return nil }
func (c *captureExporter) ForceFlush(context.Context) error { return nil }

func TestCheckoutAuditExportIncludesIntegrity(t *testing.T) {
	key := []byte("testapp-dev-hmac-key-change-in-production")
	capture := &captureExporter{}
	store := auditlog.NewAuditLogInMemoryStore()
	builder, err := auditlog.NewAuditLogProcessorBuilder(capture, store)
	if err != nil {
		t.Fatal(err)
	}
	processor, err := builder.SetWaitOnExport(true).Build()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = processor.Shutdown(context.Background()) })

	provider := auditlog.NewAuditLoggerProvider(
		auditlog.WithAuditRecordProcessor(processor),
		auditlog.WithAuditHashAlgorithm("sha256"),
		auditlog.WithAuditHMACVerificationKey(key),
		auditlog.WithAuditRecordSigning(auditlog.AuditIntegrityHMAC, auditlog.AuditSignContentMeta),
	)
	logger := provider.Logger("checkout")

	recordID := uuid.NewString()
	body, err := json.Marshal(map[string]any{"order_id": recordID, "amount": 42})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	base := newAuditBaseRecord()
	base.SetTimestamp(now)
	base.SetObservedTimestamp(now)
	base.SetSeverity(log.SeverityInfo)
	base.SetBody(log.StringValue(string(body)))

	actionUpper := "CHARGE"
	auditAttrs := []log.KeyValue{
		log.String("audit.record.id", recordID),
		log.String("audit.actor.id", "user-1"),
		log.String("audit.actor.type", "user"),
		log.String("audit.action", actionUpper),
		log.String("audit.outcome", "success"),
		log.String("audit.schema.version", "1.0"),
		log.String("audit.target.id", "tx-1"),
		log.String("audit.target.type", "order"),
	}
	base.AddAttributes(auditAttrs...)

	rec := auditlog.AuditRecord{
		Record:        base,
		EventName:     "payment.charged",
		Actor:         log.StringValue("user-1"),
		ActorType:     "user",
		Action:        actionUpper,
		Resource:      log.StringValue("tx-1"),
		TargetID:      "tx-1",
		TargetType:    "order",
		Outcome:       "success",
		RecordID:      recordID,
		SchemaVersion: "1.0",
	}

	result := logger.EmitWithResult(context.Background(), rec)
	if result.StatusCode >= 400 {
		t.Fatalf("emit failed: status=%d reason=%q", result.StatusCode, result.Reason)
	}
	if len(capture.batches) != 1 || len(capture.batches[0]) != 1 {
		t.Fatalf("expected one exported record, got batches=%d", len(capture.batches))
	}
	exported := capture.batches[0][0]
	found := false
	exported.WalkAttributes(func(kv log.KeyValue) bool {
		if string(kv.Key) == "audit.integrity.value" {
			found = true
			if strings.TrimSpace(kv.Value.AsString()) == "" {
				t.Fatal("audit.integrity.value is empty")
			}
		}
		return true
	})
	if !found {
		t.Fatal("missing audit.integrity.value on exported record")
	}
}
