# ============================================
# JARVIS SPEC VALIDATOR
# ============================================
#
# Validates every file inside:
# context/features/
#
# Checks:
# 1. Filename format        (SPEC-0001-feature-name.md)
# 2. Required sections       (# Title, ## Purpose, ## Requirements, ## Acceptance Criteria)
# 3. Duplicate SPEC IDs
#
# Run from inside scripts/ (uses relative ../context/features paths).
#
# ============================================

$featuresPath = "../context/features"

if (!(Test-Path $featuresPath)) {
    Write-Host "ERROR: Features folder not found"
    exit 1
}

# Files that are navigation/spec-of-specs, not SPEC-NNNN feature files.
$metaFiles = @(
    "FEATURE_INDEX.md",
    "FEATURE_INDEX_SPEC.md",
    "JARVIS_FEATURE_INDEX_INTEGRATION_SPEC.md"
)

$requiredSections = @(
    "## Purpose",
    "## Requirements",
    "## Acceptance Criteria"
)

$filenamePattern = '^SPEC-\d{4}-[a-z0-9]+(-[a-z0-9]+)*\.md$'
$looseIdPattern  = '^(SPEC-\d{4})'

$allFiles = Get-ChildItem $featuresPath -Filter "*.md" | Sort-Object Name

$specFiles = $allFiles | Where-Object { $metaFiles -notcontains $_.Name }

$validCount = 0
$invalidCount = 0
$idOccurrences = @{}

Write-Host ""
Write-Host "================================"
Write-Host "JARVIS Spec Validation"
Write-Host "================================"
Write-Host ""

foreach ($file in $specFiles) {

    $name = $file.Name
    $errors = @()

    $formatValid = $name -cmatch $filenamePattern

    if (!$formatValid) {
        $errors += "invalid filename format (expected SPEC-NNNN-feature-name.md)"
    }

    # Track SPEC ID occurrences (loosely) for duplicate detection,
    # even if the rest of the filename format is malformed.
    if ($name -match $looseIdPattern) {
        $specId = $matches[1].ToUpper()
        if ($idOccurrences.ContainsKey($specId)) {
            $idOccurrences[$specId] += @($name)
        } else {
            $idOccurrences[$specId] = @($name)
        }
    } else {
        $specId = $null
    }

    $missingSections = @()

    if ($formatValid) {

        $content = Get-Content $file.FullName -Raw

        $lines = $content -split "`r?`n"
        $hasTitle = $false
        foreach ($line in $lines) {
            if ($line -match '^#\s+\S') {
                $hasTitle = $true
                break
            }
        }
        if (!$hasTitle) {
            $missingSections += "Title (# heading)"
        }

        foreach ($section in $requiredSections) {
            $sectionName = $section -replace '^##\s+', ''
            $found = $false
            foreach ($line in $lines) {
                if ($line.Trim() -eq $section) {
                    $found = $true
                    break
                }
            }
            if (!$found) {
                $missingSections += $sectionName
            }
        }
    }

    $displayId = if ($specId) { $specId } else { $name }
    $prettyName = ($name -replace '^SPEC-\d{4}-', '' -replace '\.md$', '' -replace '-', ' ')

    if ($formatValid -and $missingSections.Count -eq 0) {
        Write-Host "VALID:"
        Write-Host "  $([char]0x2713) $displayId $prettyName"
        $validCount++
    } else {
        Write-Host "INVALID:"
        Write-Host "  $([char]0x2717) $displayId $prettyName"
        if (!$formatValid) {
            foreach ($e in $errors) {
                Write-Host "    - $e"
            }
        }
        if ($missingSections.Count -gt 0) {
            Write-Host "    missing:"
            foreach ($m in $missingSections) {
                Write-Host "    - $m"
            }
        }
        $invalidCount++
    }
    Write-Host ""
}

$duplicates = $idOccurrences.GetEnumerator() | Where-Object { $_.Value.Count -gt 1 }

if ($duplicates) {
    Write-Host "================================"
    Write-Host "Duplicate SPEC IDs"
    Write-Host "================================"
    foreach ($dup in $duplicates) {
        Write-Host "  $([char]0x2717) $($dup.Key) used by:"
        foreach ($f in $dup.Value) {
            Write-Host "    - $f"
        }
    }
    Write-Host ""
}

Write-Host "================================"
Write-Host "Summary"
Write-Host "================================"
Write-Host "Files scanned:  $($specFiles.Count)"
Write-Host "Valid:          $validCount"
Write-Host "Invalid:        $invalidCount"
Write-Host "Duplicate IDs:  $($duplicates.Count)"
Write-Host ""

if ($invalidCount -gt 0 -or $duplicates) {
    exit 1
} else {
    exit 0
}
