[CmdletBinding()]
param(
    [ValidateSet('dev', 'test', 'dangerous')]
    [string]$Mode = 'dev',
    [string]$ProjectName = '',
    [int64]$Seed = 0,
    [int]$PersonnelCount = 96,
    [int]$FlightCount = 72
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $projectRoot 'deployments\local\docker-compose.dev.yml'
$ensureDockerScript = Join-Path $PSScriptRoot 'ensure-docker.ps1'

function Set-StackEnvironment {
    param(
        [int]$CoreMySQLPort,
        [int]$EdgeMySQLPort,
        [int]$RedisPort,
        [int]$AdminPort,
        [int]$EmployeePort,
        [int]$CoreAPIPort,
        [int]$EdgeAPIPort,
        [int]$MetricsPort
    )

    $env:DEV_CORE_MYSQL_HOST_PORT = $CoreMySQLPort
    $env:DEV_EDGE_MYSQL_HOST_PORT = $EdgeMySQLPort
    $env:DEV_EDGE_REDIS_HOST_PORT = $RedisPort
    $env:DEV_ADMIN_HOST_PORT = $AdminPort
    $env:DEV_EMPLOYEE_HOST_PORT = $EmployeePort
    $env:DEV_CORE_API_HOST_PORT = $CoreAPIPort
    $env:DEV_EDGE_API_HOST_PORT = $EdgeAPIPort
    $env:DEV_WORKER_METRICS_HOST_PORT = $MetricsPort
}

function Invoke-ComposeStep {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)

    & $script:dockerCli compose -p $script:stackProject -f $script:composeFile @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Compose command failed: $($Arguments -join ' ')"
    }
}

function Invoke-AppCommand {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)

    & $script:dockerCli compose -p $script:stackProject -f $script:composeFile exec -T app @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "App command failed: $($Arguments -join ' ')"
    }
}

function Wait-Ready {
    param([string]$URL)

    for ($attempt = 1; $attempt -le 60; $attempt++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec 3
            if ($response.StatusCode -eq 200) {
                return
            }
        } catch {
            # Database migration and the three application processes start at
            # different times inside the all-in-one application container.
        }
        Start-Sleep -Seconds 2
    }
    throw "Service did not become ready: $URL"
}

$dockerOutput = & $ensureDockerScript -PassThru
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace(($dockerOutput -join '').Trim())) {
    throw 'Docker preflight failed; the development stack was not started.'
}
$dockerCli = ($dockerOutput | Select-Object -Last 1).ToString().Trim()

switch ($Mode) {
    'dev' {
        $stackProject = 'flight-dev'
        Set-StackEnvironment -CoreMySQLPort 4310 -EdgeMySQLPort 4311 -RedisPort 4380 -AdminPort 44174 -EmployeePort 44175 -CoreAPIPort 48081 -EdgeAPIPort 48082 -MetricsPort 49090
        $prefix = 'DEV'
    }
    'test' {
        $stackProject = 'flight-test'
        Set-StackEnvironment -CoreMySQLPort 5310 -EdgeMySQLPort 5311 -RedisPort 5380 -AdminPort 55174 -EmployeePort 55175 -CoreAPIPort 58081 -EdgeAPIPort 58082 -MetricsPort 59090
        $prefix = 'TEST'
    }
    'dangerous' {
        $runSuffix = Get-Date -Format 'yyyyMMdd-HHmmss'
        if ([string]::IsNullOrWhiteSpace($ProjectName)) {
            $stackProject = "flight-danger-$runSuffix"
        } else {
            $stackProject = $ProjectName.Trim()
        }
        $portBase = Get-Random -Minimum 20000 -Maximum 30000
        Set-StackEnvironment -CoreMySQLPort $portBase -EdgeMySQLPort ($portBase + 1) -RedisPort ($portBase + 2) -AdminPort ($portBase + 3) -EmployeePort ($portBase + 4) -CoreAPIPort ($portBase + 5) -EdgeAPIPort ($portBase + 6) -MetricsPort ($portBase + 7)
        $prefix = 'DANGER'
    }
}

if ($Seed -eq 0) {
    $Seed = Get-Random -Minimum 100000000 -Maximum 2147483647
}

Push-Location $projectRoot
try {
    Invoke-ComposeStep -Arguments @('up', '-d', '--build')
    Invoke-AppCommand -Arguments @('/app/bin/migrate', '-config', '/app/config.yaml', '-target', 'core', '-command', 'up')
    Invoke-AppCommand -Arguments @('/app/bin/migrate', '-config', '/app/config.yaml', '-target', 'edge', '-command', 'up')
    Invoke-AppCommand -Arguments @('/app/bin/seed', '-config', '/app/config.yaml', '-target', 'all', '-seed', $Seed, '-personnel', $PersonnelCount, '-flights', $FlightCount, '-prefix', $prefix)

    Wait-Ready -URL "http://127.0.0.1:$($env:DEV_CORE_API_HOST_PORT)/health/ready"
    Wait-Ready -URL "http://127.0.0.1:$($env:DEV_EDGE_API_HOST_PORT)/health/ready"

    Write-Host "Backend stack is ready: mode=$Mode project=$stackProject seed=$Seed prefix=$prefix"
    Write-Host "Admin Web:    http://127.0.0.1:$($env:DEV_ADMIN_HOST_PORT)"
    Write-Host "Employee Web: http://127.0.0.1:$($env:DEV_EMPLOYEE_HOST_PORT)"
    Write-Host "Core API:     http://127.0.0.1:$($env:DEV_CORE_API_HOST_PORT)"
    Write-Host "Edge API:     http://127.0.0.1:$($env:DEV_EDGE_API_HOST_PORT)"
    Write-Host 'Database data is stored in the named Core/Edge MySQL Volumes for this Compose project.'
    if ($Mode -eq 'dangerous') {
        Write-Host 'Dangerous/version-test project is intentionally left running for inspection.'
    } else {
        Write-Host 'The fixed development/test project is reusable; subsequent runs keep its Volumes and add a new seeded dataset.'
    }
} finally {
    Pop-Location
}
