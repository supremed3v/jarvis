# ============================================
# JARVIS FEATURE SETUP
# ============================================
#
# Bridge between FEATURE_INDEX.md / SPEC files and the Claude feature
# workflow. Populates context/current-feature.md for a given SPEC.
#
# Usage (run from inside scripts/):
#   ./setup_feature.ps1 SPEC-0007-go-runtime-bootstrap
#   ./setup_feature.ps1 SPEC-0007
#
# ============================================

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$SpecName
)

. "$PSScriptRoot/_lib_feature_index.ps1"

$featuresPath = "../context/features"
$indexPath = "$featuresPath/FEATURE_INDEX.md"
$currentFeaturePath = "../context/current-feature.md"

if (!(Test-Path $featuresPath)) {
    Write-Host "ERROR: Features folder not found"
    exit 1
}

# --- Step 2/3: read FEATURE_INDEX.md and find the matching entry ---

$entries = Get-FeatureIndexEntries -IndexPath $indexPath
$entry = Find-FeatureIndexEntry -Entries $entries -SpecNameOrId $SpecName

if (!$entry) {
    Write-Host "ERROR: No entry for '$SpecName' found in FEATURE_INDEX.md"
    exit 1
}

if (!$entry.File) {
    Write-Host "ERROR: $($entry.SpecId) has no File: reference in FEATURE_INDEX.md"
    exit 1
}

# --- Step 4: extract feature name / dependencies / implementation area / related ---

$featureName = $entry.Name
$dependencies = $entry.Dependencies
$related = $entry.Related

# --- Step 5: validate dependencies (same logic as check_dependencies.ps1) ---

$missingDeps = @()
foreach ($depId in $dependencies) {
    $found = Test-SpecFileExists -FeaturesPath $featuresPath -SpecId $depId
    if (!$found) {
        $missingDeps += $depId
    }
}

if ($missingDeps.Count -gt 0) {
    Write-Host "ERROR:"
    foreach ($m in $missingDeps) {
        Write-Host "$($entry.SpecId) requires $m but it does not exist."
    }
    Write-Host ""
    Write-Host "Aborting setup - resolve missing dependencies first."
    exit 1
}

# --- Step 6: read the actual SPEC file ---

$specFilePath = Join-Path $featuresPath $entry.File

if (!(Test-Path $specFilePath)) {
    Write-Host "ERROR: SPEC file listed in FEATURE_INDEX.md does not exist on disk: $($entry.File)"
    exit 1
}

$specContent = Get-Content $specFilePath -Raw
$specLines = $specContent -split "`r?`n"

# Best-effort extraction of Goals from the "## Requirements" section.
$goals = @()
$inRequirements = $false
$reqTextLines = @()

foreach ($line in $specLines) {
    if ($line -match '^##\s+Requirements\s*$') {
        $inRequirements = $true
        continue
    }
    if ($inRequirements -and $line -match '^##\s+\S') {
        break
    }
    if ($inRequirements) {
        $reqTextLines += $line
    }
}

if ($reqTextLines.Count -gt 0) {
    $reqText = ($reqTextLines -join " ") -replace '\s+', ' '
    $parts = $reqText -split '\s-\s'
    foreach ($p in $parts) {
        $trimmed = $p.Trim()
        if ($trimmed -eq "") { continue }
        if ($trimmed -match '^\w+:$') { continue }  # drop lead-ins like "Implement:"
        $goals += $trimmed
    }
}

if ($goals.Count -eq 0) {
    $goals = @("(see Requirements section in $($entry.File))")
}

# --- Step 7: generate/update context/current-feature.md ---

$prettyName = if ($featureName) { $featureName } else { $entry.SpecId }

$depLines = if ($dependencies.Count -gt 0) {
    ($dependencies | ForEach-Object { "- $_" }) -join "`n"
} else {
    "- (none declared in FEATURE_INDEX.md)"
}

$relatedLines = if ($related.Count -gt 0) {
    ($related | ForEach-Object { "- $_" }) -join "`n"
} else {
    "(none declared in FEATURE_INDEX.md)"
}

$goalLines = ($goals | ForEach-Object { "- $_" }) -join "`n"

$statusLine = if ($entry.Status) { $entry.Status } else { "Planned" }

$content = @"
# Current Feature: $prettyName

## Working In

Not specified in FEATURE_INDEX.md - confirm target directory against
docs/architecture/ and docs/execution/JARVIS_IMPLEMENTATION_ORDER.md before
running /feature start.

## Status

Not Started

## Goals

$goalLines

## Dependencies

$depLines

## Notes

Specification:

context/features/$($entry.File)

Index status at load time: $statusLine

Related specs: $relatedLines

## History

- $(Get-Date -Format "yyyy-MM-dd HH:mm") setup_feature.ps1 loaded $($entry.SpecId) ($($entry.File))
"@

Set-Content -Path $currentFeaturePath -Value $content -Encoding utf8

Write-Host ""
Write-Host "# Current Feature: $prettyName"
Write-Host ""
Write-Host "## Working In"
Write-Host ""
Write-Host "(not specified in FEATURE_INDEX.md)"
Write-Host ""
Write-Host "## Status"
Write-Host ""
Write-Host "Not Started"
Write-Host ""
Write-Host "## Goals"
Write-Host ""
Write-Host $goalLines
Write-Host ""
Write-Host "## Dependencies"
Write-Host ""
Write-Host $depLines
Write-Host ""
Write-Host "## Notes"
Write-Host ""
Write-Host "Specification:"
Write-Host ""
Write-Host "context/features/$($entry.File)"
Write-Host ""
Write-Host "Feature loaded successfully."
Write-Host ""
Write-Host "Ready for:"
Write-Host "/feature start"
Write-Host ""

exit 0
