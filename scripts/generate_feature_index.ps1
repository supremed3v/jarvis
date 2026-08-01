# ============================================
# JARVIS FEATURE INDEX GENERATOR
# ============================================
#
# Generates:
# context/features/FEATURE_INDEX.md
#
# Reads:
# context/features/SPEC-*.md
#
# ============================================

$featuresPath = "../context/features"

$outputFile = "../context/features/FEATURE_INDEX.md"


if (!(Test-Path $featuresPath)) {

    Write-Host "ERROR: Features folder not found"
    exit 1

}

# Built from character codes rather than typed as literals, so the
# generated output is correct regardless of what encoding this .ps1 file
# itself gets saved/read as (Windows PowerShell 5.1 reads script files
# without a BOM using the system ANSI codepage, which previously mangled
# a literal em dash into "â€”" once round-tripped through Out-File -Encoding
# utf8). A single-quoted string does not run backtick escape processing,
# so it also gives us a real triple-backtick fence to interpolate below -
# writing "```" directly inside the @"..."@ here-string is escape-processed
# by PowerShell and collapses to a single backtick.
$emDash = [string][char]0x2014
$fence = '```'

# Status is tracked authoritatively in docs/agents/JARVIS_BUILD_TRACKER.md,
# not derived from anything in context/features/. Parse it here so the
# generated index reflects real progress instead of a hardcoded "Planned"
# for every spec forever. Matching on the tracker's own declared status
# words (rather than slicing by column position) avoids fragile dependence
# on whitespace alignment in that table.
$buildTrackerPath = "../docs/agents/JARVIS_BUILD_TRACKER.md"
$statusValues = "Planned|In Progress|Blocked|Completed|Verified"
$specStatus = @{}

if (Test-Path $buildTrackerPath) {

    Get-Content $buildTrackerPath | ForEach-Object {

        if ($_ -match "^\s*(SPEC-\d{4})\s+($statusValues)\b") {
            $specStatus[$matches[1]] = $matches[2]
        }

    }

}

$header = @"
# JARVIS Feature Index

Generated automatically.

This file provides navigation for AI agents.

---

"@


$content = $header


Get-ChildItem $featuresPath -Filter "SPEC-*.md" |
Sort-Object Name |
ForEach-Object {


    $filename = $_.BaseName


    if ($filename -match "^(SPEC-\d+)-(.*)$") {


        $specID = $matches[1]
        $featureName = $matches[2]


        $prettyName = $featureName -replace "-", " "
        $prettyName = (Get-Culture).TextInfo.ToTitleCase($prettyName)


        $status = if ($specStatus.ContainsKey($specID)) { $specStatus[$specID] } else { "Planned" }


        $content += @"

## $specID $emDash $prettyName


File:

$fence
$($_.Name)
$fence


Status:

$status


---

"@

    }

}


$content | Out-File `
    -FilePath $outputFile `
    -Encoding utf8


Write-Host ""
Write-Host "================================"
Write-Host "Feature index generated"
Write-Host "================================"

Write-Host ""
Write-Host "Created:"
Write-Host $outputFile
