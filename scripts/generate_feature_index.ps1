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


        $content += @"

## $specID $emDash $prettyName


File:

$fence
$($_.Name)
$fence


Status:

Planned


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
