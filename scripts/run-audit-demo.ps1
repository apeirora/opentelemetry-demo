# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

$ErrorActionPreference = "Stop"

$DemoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $DemoRoot

$ComposeFiles = @("-f", "compose.yaml", "-f", "compose.audit.yaml")

Write-Host "Pulling otelauditcol from GitHub Container Registry..."
docker compose @ComposeFiles pull otel-collector-audit

Write-Host "Pulling checkout-audit from GitHub Container Registry..."
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
docker compose @ComposeFiles pull checkout 2>&1 | ForEach-Object { Write-Host $_ }
$checkoutPullOk = $LASTEXITCODE -eq 0
$ErrorActionPreference = $prevEAP
if (-not $checkoutPullOk) {
    Write-Host "checkout-audit not available on GHCR yet - building locally..." -ForegroundColor Yellow
    docker compose @ComposeFiles build checkout
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to build checkout-audit" -ForegroundColor Red
        exit 1
    }
}

Write-Host "Pulling remaining demo images..."
docker compose @ComposeFiles pull --ignore-buildable

Write-Host "Starting audit demo..."
docker compose @ComposeFiles up -d

Write-Host ""
Write-Host "Waiting for core services..."
$deadline = (Get-Date).AddMinutes(5)
$healthy = $false
while ((Get-Date) -lt $deadline) {
    $collector = docker inspect otel-collector --format "{{.State.Status}}" 2>$null
    $auditCol = docker compose @ComposeFiles ps --format json otel-collector-audit 2>$null | ConvertFrom-Json
    $checkoutSvc = docker inspect checkout --format "{{.State.Health.Status}}" 2>$null
    $proxy = docker inspect frontend-proxy --format "{{.State.Status}}" 2>$null
    if ($collector -eq "running" -and $auditCol.State -eq "running" -and $proxy -eq "running" -and $checkoutSvc -eq "healthy") {
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
Write-Host ("  docker compose " + ($ComposeFiles -join ' ') + " logs -f checkout otel-collector-audit otel-collector")
Write-Host "  Open http://localhost:8080 and place an order"
Write-Host "  Enable audit logs: http://localhost:8080/feature/ -> toggle auditLogging to on"
Write-Host ""
Write-Host ("Stop: docker compose " + ($ComposeFiles -join ' ') + " down")

if (-not $healthy) {
    exit 1
}
