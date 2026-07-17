//go:build !audit

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package main

import "context"

func initAuditFromEnv() error { return nil }

func shutdownAudit(context.Context) {}

func emitCheckoutAudit(context.Context, string, string, string, string, string, map[string]any) {
}
