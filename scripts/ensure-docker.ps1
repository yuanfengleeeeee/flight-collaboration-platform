[CmdletBinding()]
param(
    [int]$TimeoutSeconds = 180,
    [switch]$SkipLaunch,
    [switch]$PassThru
)

$ErrorActionPreference = 'Stop'

function Test-FileCandidate {
    param([Parameter(Mandatory = $true)][string]$CandidatePath)

    try {
        return (Test-Path -LiteralPath $CandidatePath -PathType Leaf)
    } catch {
        # A restricted environment may deny probing a known installation path.
        # Treat that candidate as unavailable and continue checking other paths.
        return $false
    }
}

function Find-DockerCli {
    $dockerCommand = Get-Command docker.exe -ErrorAction SilentlyContinue
    if ($null -ne $dockerCommand) {
        return $dockerCommand.Source
    }

    $dockerCandidates = @()
    if ($env:ProgramFiles) {
        $dockerCandidates += Join-Path $env:ProgramFiles 'Docker\Docker\resources\bin\docker.exe'
    }
    if (${env:ProgramW6432}) {
        $dockerCandidates += Join-Path ${env:ProgramW6432} 'Docker\Docker\resources\bin\docker.exe'
    }
    if ($env:LOCALAPPDATA) {
        $dockerCandidates += Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\resources\bin\docker.exe'
    }

    foreach ($dockerCandidate in $dockerCandidates) {
        if (Test-FileCandidate -CandidatePath $dockerCandidate) {
            return (Resolve-Path -LiteralPath $dockerCandidate).Path
        }
    }

    throw 'Docker CLI was not found. Install Docker Desktop or add docker.exe to PATH.'
}

function Find-DockerDesktop {
    $desktopCandidates = @()
    if ($env:ProgramFiles) {
        $desktopCandidates += Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
    }
    if (${env:ProgramW6432}) {
        $desktopCandidates += Join-Path ${env:ProgramW6432} 'Docker\Docker\Docker Desktop.exe'
    }
    if ($env:LOCALAPPDATA) {
        $desktopCandidates += Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\Docker Desktop.exe'
        $desktopCandidates += Join-Path $env:LOCALAPPDATA 'Docker\Docker Desktop.exe'
    }

    foreach ($desktopCandidate in $desktopCandidates) {
        if (Test-FileCandidate -CandidatePath $desktopCandidate) {
            return (Resolve-Path -LiteralPath $desktopCandidate).Path
        }
    }

    throw 'Docker Desktop was not found. Check the Docker Desktop installation path.'
}

function Test-DockerEngine {
    param([Parameter(Mandatory = $true)][string]$DockerCli)

    try {
        $serverVersion = & $DockerCli info --format '{{.ServerVersion}}' 2>$null
        return ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace(($serverVersion -join '').Trim()))
    } catch {
        return $false
    }
}

$dockerCli = Find-DockerCli

if (Test-DockerEngine -DockerCli $dockerCli) {
    Write-Host "Docker Engine is ready: $dockerCli"
} elseif ($SkipLaunch) {
    throw 'Docker Engine is not ready and -SkipLaunch was specified; validation stopped.'
} else {
    $desktopPath = Find-DockerDesktop
    $desktopProcess = Get-Process -Name 'Docker Desktop' -ErrorAction SilentlyContinue

    if ($null -eq $desktopProcess) {
        Write-Host "Starting Docker Desktop: $desktopPath"
        Start-Process -FilePath $desktopPath | Out-Null
    } else {
        Write-Host 'Docker Desktop is already running; waiting for Docker Engine.'
    }

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        if (Test-DockerEngine -DockerCli $dockerCli) {
            Write-Host 'Docker Engine is ready.'
            break
        }

        if ((Get-Date) -ge $deadline) {
            throw "Docker Engine was not ready within $TimeoutSeconds seconds. Check Docker Desktop and try again."
        }

        Start-Sleep -Seconds 2
    } while ($true)
}

if ($PassThru) {
    Write-Output $dockerCli
}
