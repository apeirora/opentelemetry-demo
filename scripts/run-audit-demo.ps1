# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

$ErrorActionPreference = "Stop"

$DemoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $DemoRoot

$ComposeFiles = @("-f", "compose.yaml", "-f", "compose.observability.yaml", "-f", "compose.audit.yaml")

Write-Host "Building main collector with certificatelogverify..."
docker compose @ComposeFiles build otel-collector
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to build otelcol-audit-main" -ForegroundColor Red
    exit 1
}

Write-Host "Building audit sidecar (auditlogreceiver + signingprocessor)..."
docker compose @ComposeFiles build otel-collector-audit-sidecar
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to build otelauditsidecar" -ForegroundColor Red
    exit 1
}

Write-Host "Building checkout (raw OTLP audit emit, no auditlog SDK)..."
docker compose @ComposeFiles build checkout
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to build checkout-audit" -ForegroundColor Red
    exit 1
}

Write-Host "Pulling remaining demo images..."
docker compose @ComposeFiles pull --ignore-buildable

Write-Host "Starting audit demo (app -> sidecar sign -> main verify -> Grafana failover)..."
docker compose @ComposeFiles up -d

Write-Host ""
Write-Host "Waiting for core services..."
$deadline = (Get-Date).AddMinutes(5)
$healthy = $false
while ((Get-Date) -lt $deadline) {
    $collector = docker inspect otel-collector --format "{{.State.Status}}" 2>$null
    $sidecar = docker compose @ComposeFiles ps --format json otel-collector-audit-sidecar 2>$null | ConvertFrom-Json
    $checkoutSvc = docker inspect checkout --format "{{.State.Health.Status}}" 2>$null
    $proxy = docker inspect frontend-proxy --format "{{.State.Status}}" 2>$null
    $grafana = docker inspect grafana --format "{{.State.Status}}" 2>$null
    $grafanaStandby = docker inspect grafana-standby --format "{{.State.Status}}" 2>$null
    if ($collector -eq "running" -and $sidecar.State -eq "running" -and $proxy -eq "running" -and $checkoutSvc -eq "healthy" -and $grafana -eq "running" -and $grafanaStandby -eq "running") {
        $healthy = $true
        break
    }
    Start-Sleep -Seconds 5
}

docker compose @ComposeFiles ps

Write-Host ""
if ($healthy) {
    Write-Host "Audit demo is running." -ForegroundColor Green
} else {
    Write-Host "WARNING: Some services may still be starting or unhealthy." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Useful commands:"
Write-Host ("  docker compose " + ($ComposeFiles -join ' ') + " logs -f checkout otel-collector-audit-sidecar otel-collector")
Write-Host "  Open http://localhost:8080 and place an order"
Write-Host "  Enable audit logs: http://localhost:8080/feature/ -> toggle auditLogging to on"
Write-Host "  Main Grafana: http://localhost:8080/grafana/"
Write-Host "  Standby Grafana: http://localhost:3001/grafana/"
Write-Host "  Explore query: attributes.verify_status:passed OR _exists_:attributes.audit.action"
Write-Host "  Fail over verified audit logs: docker stop grafana"
Write-Host "  Restore: docker start grafana; docker restart grafana-primary-proxy"
Write-Host ""
Write-Host ("Stop: docker compose " + ($ComposeFiles -join ' ') + " down")

if (-not $healthy) {
    exit 1
}
