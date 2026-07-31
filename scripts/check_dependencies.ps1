# ============================================
# JARVIS DEPENDENCY CHECKER
# ============================================
#
# Validates dependency references declared inside:
# context/features/FEATURE_INDEX.md
#
# For each SPEC entry with a "Dependencies:" block, confirms every
# referenced SPEC-NNNN has a matching file in context/features/.
#
# Run from inside scripts/ (uses relative ../context/features paths).
#
# ============================================

. "$PSScriptRoot/_lib_feature_index.ps1"

$featuresPath = "../context/features"
$indexPath = "$featuresPath/FEATURE_INDEX.md"

if (!(Test-Path $featuresPath)) {
    Write-Host "ERROR: Features folder not found"
    exit 1
}

$entries = Get-FeatureIndexEntries -IndexPath $indexPath

$checked = 0
$missing = @()

Write-Host ""
Write-Host "================================"
Write-Host "JARVIS Dependency Check"
Write-Host "================================"
Write-Host ""

foreach ($entry in $entries) {

    foreach ($depId in $entry.Dependencies) {

        $checked++

        $found = Test-SpecFileExists -FeaturesPath $featuresPath -SpecId $depId

        if (!$found) {
            $missing += [PSCustomObject]@{
                Spec       = $entry.SpecId
                Dependency = $depId
            }
            Write-Host "ERROR:"
            Write-Host ""
            Write-Host "$($entry.SpecId) requires $depId but it does not exist."
            Write-Host ""
        }
    }
}

Write-Host "================================"
Write-Host "Summary"
Write-Host "================================"
Write-Host ""
Write-Host "Dependencies checked:"
Write-Host $checked
Write-Host ""
Write-Host "Missing:"
Write-Host $missing.Count
Write-Host ""

if ($checked -eq 0) {
    Write-Host "NOTE: FEATURE_INDEX.md currently declares no 'Dependencies:' blocks for"
    Write-Host "any SPEC entry, so 0 references were found to check. This script is"
    Write-Host "forward-compatible: once entries gain a 'Dependencies:' list (e.g."
    Write-Host "  Dependencies:"
    Write-Host "  - SPEC-0001"
    Write-Host "  - SPEC-0003"
    Write-Host ") it will validate them automatically."
    Write-Host ""
}

if ($missing.Count -gt 0) {
    exit 1
} else {
    exit 0
}
