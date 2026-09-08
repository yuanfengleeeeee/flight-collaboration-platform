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

function Get-ProjectDockerCli {
    $pathFile = Join-Path $PSScriptRoot 'docker-cli-path.local.txt'
    if (-not (Test-FileCandidate -CandidatePath $pathFile)) {
        return $null
    }

    try {
        $configuredPath = (Get-Content -LiteralPath $pathFile -Raw).Trim()
        if ([string]::IsNullOrWhiteSpace($configuredPath)) {
            return $null
        }
        # The final process invocation is the authoritative check. Keeping
        # this explicit local override usable also helps restricted shells
        # where probing a per-user installation returns Access Denied.
        return $configuredPath
    } catch {
        # A stale or inaccessible local path must not prevent the normal
        # PATH and Docker Desktop fallback discovery from running.
    }

    return $null
}

function Find-DockerCli {
    $projectDockerCli = Get-ProjectDockerCli
    if (-not [string]::IsNullOrWhiteSpace($projectDockerCli)) {
        return $projectDockerCli
    }

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

    # Prefer the registered installation location so per-user and custom
    # Docker Desktop installs do not depend on a hard-coded path.
    $uninstallKeys = @(
        'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Docker Desktop',
        'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Docker Desktop',
        'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Docker Desktop'
    )
    foreach ($uninstallKey in $uninstallKeys) {
        try {
            $installLocation = (Get-ItemProperty -LiteralPath $uninstallKey -Name 'InstallLocation' -ErrorAction SilentlyContinue).InstallLocation
            if (-not [string]::IsNullOrWhiteSpace($installLocation)) {
                $desktopCandidates += Join-Path $installLocation 'Docker Desktop.exe'
            }
        } catch {
            # Registry access is optional; continue with environment paths.
        }
    }

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
        # `version` is a lighter readiness probe than `info` and still
        # verifies the selected context can reach the Docker server.
        $serverVersion = & $DockerCli version --format '{{.Server.Version}}' 2>&1
        if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace(($serverVersion -join '').Trim())) {
            $script:LastDockerEngineError = $null
            return $true
        }

        $script:LastDockerEngineError = (($serverVersion | Out-String).Trim())
        return $false
    } catch {
        $script:LastDockerEngineError = $_.Exception.Message
        return $false
    }
}

function Start-DockerDesktopWithCli {
    param([Parameter(Mandatory = $true)][string]$DockerCli)

    try {
        # The Docker Desktop CLI knows the active per-user installation and
        # avoids relying on a fixed Docker Desktop.exe path.
        & $DockerCli desktop start --detach 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) {
            return $true
        }
    } catch {
        $script:LastDockerDesktopStartError = $_.Exception.Message
        return $false
    }

    return $false
}

$dockerCli = Find-DockerCli

if (Test-DockerEngine -DockerCli $dockerCli) {
    Write-Host "Docker Engine is ready: $dockerCli"
} elseif ($SkipLaunch) {
    throw 'Docker Engine is not ready and -SkipLaunch was specified; validation stopped.'
} else {
    $startedWithCli = Start-DockerDesktopWithCli -DockerCli $dockerCli

    if ($startedWithCli) {
        Write-Host 'Starting Docker Desktop through the Docker CLI; waiting for Docker Engine.'
    } else {
        $desktopPath = Find-DockerDesktop
        $desktopProcess = Get-Process -Name 'Docker Desktop' -ErrorAction SilentlyContinue

        if ($null -eq $desktopProcess) {
            Write-Host "Starting Docker Desktop: $desktopPath"
            Start-Process -FilePath $desktopPath | Out-Null
        } else {
            Write-Host 'Docker Desktop is already running; waiting for Docker Engine.'
        }
    }

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        if (Test-DockerEngine -DockerCli $dockerCli) {
            Write-Host 'Docker Engine is ready.'
            break
        }

        if ((Get-Date) -ge $deadline) {
            $context = (& $dockerCli context show 2>$null | Out-String).Trim()
            $detail = if ([string]::IsNullOrWhiteSpace($script:LastDockerEngineError)) {
                'The Docker server did not return a version.'
            } else {
                $script:LastDockerEngineError
            }
            throw "Docker Engine was not ready within $TimeoutSeconds seconds. Context='$context'; CLI='$dockerCli'; detail='$detail'. Check Docker Desktop and the selected Docker context."
        }

        Start-Sleep -Seconds 2
    } while ($true)
}

if ($PassThru) {
    Write-Output $dockerCli
}
