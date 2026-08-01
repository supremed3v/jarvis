# ============================================
# JARVIS GO WORKSPACE BUILD/VET/TEST RUNNER
# ============================================
#
# Runs `go build ./...`, `go vet ./...`, and `go test ./...` inside every
# module listed in go.work's `use` block, one module at a time, and reports
# one aggregated pass/fail result.
#
# Why this exists: `go build ./...` from the repo root fails with
# "pattern ./...: directory prefix . does not contain modules listed in
# go.work or their selected dependencies", because the repo root has no
# go.mod of its own and isn't itself a workspace member - only its
# subdirectories are. Go's relative patterns (./...) only resolve inside a
# directory that's part of a workspace module, and that's a workspace-mode
# design constraint, not something go.work can be configured around. This
# script automates the per-module workaround (cd into each module, run the
# command, repeat) that was previously done by hand for every SPEC.
#
# Run from inside scripts/, since it uses a relative ../go.work path.
#
# Usage:
#   ./go_all.ps1            # build + vet + test (default)
#   ./go_all.ps1 build      # build only
#   ./go_all.ps1 vet        # vet only
#   ./go_all.ps1 test       # test only
#
# ============================================

param(
    [ValidateSet("all", "build", "vet", "test")]
    [string]$Task = "all"
)


$goWorkPath = "../go.work"


if (!(Test-Path $goWorkPath)) {

    Write-Host "ERROR: go.work not found"
    exit 1

}


# Module paths come from go.work's `use ( ... )` block (one per line, e.g.
# "./services/core"), parsed rather than hardcoded so a future SPEC adding a
# module to go.work doesn't also require updating this script.
$modules = @()
$inUseBlock = $false

Get-Content $goWorkPath | ForEach-Object {

    if ($_ -match "^\s*use\s*\(") {
        $inUseBlock = $true
        return
    }

    if ($inUseBlock -and $_ -match "^\s*\)") {
        $inUseBlock = $false
        return
    }

    if ($inUseBlock -and $_ -match "^\s*(\./\S+)") {
        $modules += $matches[1]
    }

}


if ($modules.Count -eq 0) {

    Write-Host "ERROR: no modules found in go.work"
    exit 1

}


$tasks = if ($Task -eq "all") { @("build", "vet", "test") } else { @($Task) }

$failures = @()


foreach ($module in $modules) {

    Write-Host ""
    Write-Host "================================"
    Write-Host $module
    Write-Host "================================"

    Push-Location (Join-Path ".." $module)

    foreach ($t in $tasks) {

        Write-Host ""
        Write-Host "-> go $t ./..."

        go $t "./..."

        if ($LASTEXITCODE -ne 0) {
            $failures += "${module}: go $t"
        }

    }

    Pop-Location

}


Write-Host ""
Write-Host "================================"

if ($failures.Count -eq 0) {

    Write-Host "All modules clean ($($modules.Count) modules: $($tasks -join ', '))"
    Write-Host "================================"
    exit 0

} else {

    Write-Host "FAILURES:"
    $failures | ForEach-Object { Write-Host "  $_" }
    Write-Host "================================"
    exit 1

}
