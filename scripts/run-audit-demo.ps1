# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

$ErrorActionPreference = "Stop"

$DemoRoot = Split-Path -Parent $PSScriptRoot
$OtelRoot = Split-Path -Parent $DemoRoot
Set-Location $DemoRoot

$ComposeFiles = @("-f", "compose.yaml", "-f", "compose.audit.yaml")

function Test-Prerequisite {
    param([string]$Path, [string]$Label)
    if (-not (Test-Path $Path)) {
        Write-Host "ERROR: Missing $Label at $Path" -ForegroundColor Red
        exit 1
    }
}

Test-Prerequisite (Join-Path $OtelRoot "opentelemetry-go") "opentelemetry-go"

if (-not $env:GITHUB_TOKEN) {
    try {
        $env:GITHUB_TOKEN = (gh auth token 2>$null).Trim()
    } catch {
        # gh not installed or not logged in
    }
}

Write-Host "Pulling audit collector image..."
docker compose @ComposeFiles pull otel-collector-audit

Write-Host "Building audit-enabled checkout..."
docker compose @ComposeFiles build checkout

if ($env:GITHUB_TOKEN) {
    Write-Host "GITHUB_TOKEN set — building all services..."
    docker compose @ComposeFiles up --build -d
} else {
    Write-Host "GITHUB_TOKEN not set — pulling prebuilt demo images (checkout built locally)."
    Write-Host "(ad uses eu.apeirora.opentelemetry from GitHub Packages; set GITHUB_TOKEN to rebuild it.)"
    docker compose @ComposeFiles pull --ignore-buildable
    docker compose @ComposeFiles up -d
}

Write-Host ""
Write-Host "Waiting for core services..."
$deadline = (Get-Date).AddMinutes(3)
$healthy = $false
while ((Get-Date) -lt $deadline) {
    $collector = docker inspect otel-collector --format "{{.State.Status}}" 2>$null
    $auditCol = docker compose @ComposeFiles ps --format json otel-collector-audit 2>$null | ConvertFrom-Json
    $proxy = docker inspect frontend-proxy --format "{{.State.Status}}" 2>$null
    if ($collector -eq "running" -and $auditCol.State -eq "running" -and $proxy -eq "running") {
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
    Write-Host "WARNING: Some services may still be starting or unhealthy. Check logs below." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Useful commands:"
Write-Host "  docker compose @($ComposeFiles -join ' ') logs -f checkout otel-collector-audit otel-collector"
Write-Host "  Open http://localhost:8080 and place an order"
Write-Host "  Enable audit logs: http://localhost:8080/feature/ -> toggle auditLogging to on"
Write-Host "  Verified audit records appear in otel-collector logs (forwarded via OTLP HTTP)"
Write-Host ""
Write-Host "Stop: docker compose @($ComposeFiles -join ' ') down"

if (-not $healthy) {
    exit 1
}
