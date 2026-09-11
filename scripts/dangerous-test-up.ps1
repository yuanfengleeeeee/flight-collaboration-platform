[CmdletBinding()]
param(
    [string]$ProjectName = '',
    [int64]$Seed = 0,
    [int]$PersonnelCount = 96,
    [int]$FlightCount = 72
)

$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'start-backend-stack.ps1') -Mode dangerous -ProjectName $ProjectName -Seed $Seed -PersonnelCount $PersonnelCount -FlightCount $FlightCount
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
