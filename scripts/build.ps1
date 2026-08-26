<#
.SYNOPSIS
    Builds gg with version metadata baked in via -ldflags.

.DESCRIPTION
    Populates internal/buildinfo's Version/Commit/Date vars at link time so
    `gg --version` reports something meaningful. Falls back to "dev"/"none"
    when git isn't available (e.g. before the repo is initialized).
#>
param(
    [string]$Output = "bin/gg.exe"
)

$ErrorActionPreference = "Stop"

function Get-GitValue {
    param([string[]]$GitArgs, [string]$Fallback)

    if (-not (Get-Command git -ErrorAction SilentlyContinue)) { return $Fallback }
    try {
        $value = & git @GitArgs 2>$null
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($value)) { return $Fallback }
        return $value.Trim()
    } catch {
        return $Fallback
    }
}

$version = Get-GitValue -GitArgs @("describe", "--tags", "--always", "--dirty") -Fallback "dev"
$commit  = Get-GitValue -GitArgs @("rev-parse", "--short", "HEAD") -Fallback "none"
$date    = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$ldflags = "-X gg/internal/buildinfo.Version=$version " +
           "-X gg/internal/buildinfo.Commit=$commit " +
           "-X gg/internal/buildinfo.Date=$date"

$outDir = Split-Path -Parent $Output
if ($outDir) {
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null
}

go build -ldflags $ldflags -o $Output ./cmd/gg

Write-Host "Built $Output  (version=$version, commit=$commit, date=$date)"
