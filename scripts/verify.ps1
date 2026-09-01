[CmdletBinding()]
param(
    [ValidateSet('all', 'test', 'build', 'compose-config', 'compose-ps')]
    [string]$Mode = 'all',
    [int]$DockerTimeoutSeconds = 180
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$ensureDockerScript = Join-Path $PSScriptRoot 'ensure-docker.ps1'
$composeFile = Join-Path $projectRoot 'deployments\local\docker-compose.yml'

$dockerCli = & $ensureDockerScript -TimeoutSeconds $DockerTimeoutSeconds -PassThru
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace(($dockerCli -join '').Trim())) {
    throw 'Docker preflight failed; project validation was not started.'
}

Push-Location $projectRoot
try {
    switch ($Mode) {
        'test' {
            & go test ./...
            if ($LASTEXITCODE -ne 0) { throw 'go test ./... failed.' }
        }
        'build' {
            & go build ./...
            if ($LASTEXITCODE -ne 0) { throw 'go build ./... failed.' }
        }
        'compose-config' {
            & $dockerCli compose -f $composeFile config --quiet
            if ($LASTEXITCODE -ne 0) { throw 'Docker Compose config validation failed.' }
        }
        'compose-ps' {
            & $dockerCli compose -f $composeFile ps
            if ($LASTEXITCODE -ne 0) { throw 'Docker Compose service status check failed.' }
        }
        'all' {
            & go test ./...
            if ($LASTEXITCODE -ne 0) { throw 'go test ./... failed.' }

            & go build ./...
            if ($LASTEXITCODE -ne 0) { throw 'go build ./... failed.' }

            & $dockerCli compose -f $composeFile config --quiet
            if ($LASTEXITCODE -ne 0) { throw 'Docker Compose config validation failed.' }

            & git diff --check
            if ($LASTEXITCODE -ne 0) { throw 'git diff --check failed.' }
        }
    }
} finally {
    Pop-Location
}

Write-Host "Validation completed: $Mode"
