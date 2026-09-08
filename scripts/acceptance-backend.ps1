[CmdletBinding()]
param(
    [string]$ProjectName = 'flight-backend-acceptance',
    [int]$CoreMySQLPort = 13310,
    [int]$EdgeMySQLPort = 13311,
    [int]$CoreAPIPort = 18081,
    [int]$EdgeGatewayPort = 18082,
    [int]$WorkerMetricsPort = 19090
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $projectRoot 'deployments\local\docker-compose.yml'
$ensureDockerScript = Join-Path $PSScriptRoot 'ensure-docker.ps1'

$dockerCli = & $ensureDockerScript -PassThru
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace(($dockerCli -join '').Trim())) {
    throw 'Docker preflight failed; isolated backend acceptance was not started.'
}
$dockerCli = ($dockerCli | Select-Object -Last 1).ToString().Trim()

Push-Location $projectRoot
try {
    $env:CORE_MYSQL_HOST_PORT = $CoreMySQLPort
    $env:EDGE_MYSQL_HOST_PORT = $EdgeMySQLPort
    $env:CORE_API_HOST_PORT = $CoreAPIPort
    $env:EDGE_GATEWAY_HOST_PORT = $EdgeGatewayPort
    $env:WORKER_METRICS_HOST_PORT = $WorkerMetricsPort

    # This project name gives the acceptance run dedicated containers and
    # volumes. It never calls down -v, so an interrupted run remains inspectable.
    & $dockerCli compose -p $ProjectName -f $composeFile up -d --build
    if ($LASTEXITCODE -ne 0) { throw 'isolated Compose environment failed to start.' }

    & $dockerCli compose -p $ProjectName -f $composeFile ps
    if ($LASTEXITCODE -ne 0) { throw 'isolated Compose service inspection failed.' }

    $env:FLIGHT_CORE_DB_HOST = '127.0.0.1'
    $env:FLIGHT_CORE_DB_PORT = $CoreMySQLPort
    $env:FLIGHT_EDGE_DB_HOST = '127.0.0.1'
    $env:FLIGHT_EDGE_DB_PORT = $EdgeMySQLPort
    $env:FLIGHT_SYNC_EDGE_BASE_URL = "http://127.0.0.1:$EdgeGatewayPort"

    function Invoke-Migration {
        param([ValidateSet('core', 'edge')][string]$Target)
        $output = & go run ./cmd/migrate -config configs/config.v2.yaml -target $Target -command up 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "${Target} migration up failed:`n$($output -join [Environment]::NewLine)"
        }
        return $output
    }

    function Get-MigrationStatus {
        param([ValidateSet('core', 'edge')][string]$Target)
        $output = & go run ./cmd/migrate -config configs/config.v2.yaml -target $Target -command status 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "${Target} migration status failed:`n$($output -join [Environment]::NewLine)"
        }
        return ($output -join [Environment]::NewLine)
    }

    Invoke-Migration -Target core | Out-Host
    Invoke-Migration -Target edge | Out-Host
    $coreStatus = Get-MigrationStatus -Target core
    $edgeStatus = Get-MigrationStatus -Target edge
    $coreStatus | Out-Host
    $edgeStatus | Out-Host

    foreach ($version in 10..14) {
        if ($coreStatus -notmatch ("(?m)^\s*{0:D6}\s+.*\sapplied\s*$" -f $version)) {
            throw "Core migration $version is not applied in the isolated database."
        }
    }
    if ($edgeStatus -notmatch '(?m)^\s*000007\s+.*\sapplied\s*$') {
        throw 'Edge migration 000007 is not applied in the isolated database.'
    }

    function Wait-Ready {
        param([string]$URL)
        for ($attempt = 1; $attempt -le 60; $attempt++) {
            try {
                $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec 3
                if ($response.StatusCode -eq 200) { return }
            } catch {
                # Compose healthchecks and API startup can complete at different times.
            }
            Start-Sleep -Seconds 2
        }
        throw "Service did not become ready: $URL"
    }

    Wait-Ready -URL "http://127.0.0.1:$CoreAPIPort/health/ready"
    Wait-Ready -URL "http://127.0.0.1:$EdgeGatewayPort/health/ready"

    $env:FLIGHT_RUN_DB_PROBE = '1'
    & go test ./internal/integration/sync -run TestArchitectureProbeAgainstRunningServices -count=1 -v
    if ($LASTEXITCODE -ne 0) { throw 'SQL/HTTP architecture probe failed in the isolated environment.' }

    Write-Host "Backend isolated acceptance completed: project=$ProjectName"
    Write-Host 'The isolated Compose project is intentionally left running for inspection.'
} finally {
    Pop-Location
}
