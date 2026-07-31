# ============================================
# JARVIS FEATURE INDEX HELPERS
# ============================================
#
# Shared parsing helpers for FEATURE_INDEX.md,
# dot-sourced by check_dependencies.ps1 and setup_feature.ps1
# so both scripts use identical extraction logic.
#
# Not meant to be run directly.
#
# ============================================

function Get-FeatureIndexEntries {
    param(
        [Parameter(Mandatory = $true)]
        [string]$IndexPath
    )

    if (!(Test-Path $IndexPath)) {
        Write-Host "ERROR: FEATURE_INDEX.md not found at $IndexPath"
        exit 1
    }

    $lines = Get-Content $IndexPath

    $entries = @()
    $current = $null
    $mode = $null  # "file" | "status" | "dependencies" | "related" | $null

    foreach ($rawLine in $lines) {

        $line = $rawLine.Trim()

        if ($rawLine -match '^##\s+(SPEC-\d{4})(.*)$') {

            if ($null -ne $current) {
                $entries += $current
            }

            $specId = $matches[1].ToUpper()
            $remainder = $matches[2]
            # Strip everything up to the first plain ASCII letter. The
            # separator between ID and name is normally an em dash, but
            # generate_feature_index.ps1 has a pre-existing encoding bug
            # that can emit it as mojibake instead - stripping on
            # "not A-Za-z" sidesteps needing to match that literally.
            $remainder = $remainder -replace '^[^A-Za-z]*', ''

            $current = [PSCustomObject]@{
                SpecId       = $specId
                Name         = $remainder.Trim()
                File         = $null
                Status       = $null
                Dependencies = @()
                Related      = @()
            }
            $mode = $null
            continue
        }

        if ($null -eq $current) {
            continue
        }

        if ($line -eq "File:") { $mode = "file"; continue }
        if ($line -eq "Status:") { $mode = "status"; continue }
        if ($line -eq "Dependencies:") { $mode = "dependencies"; continue }
        if ($line -match '^Related( Specifications)?:$') { $mode = "related"; continue }
        if ($line -eq "---") { $mode = $null; continue }

        $backtick = [char]0x60
        if ($line -eq "" -or $line -eq "$backtick" -or $line -eq "$backtick$backtick$backtick") {
            # blank line or a lone/triple backtick fence marker - skip, keep current mode
            continue
        }

        switch ($mode) {
            "file" {
                if (!$current.File) { $current.File = $line }
                $mode = $null
            }
            "status" {
                if (!$current.Status) { $current.Status = $line }
                $mode = $null
            }
            "dependencies" {
                if ($line -match '(SPEC-\d{4})') {
                    $current.Dependencies += $matches[1]
                }
            }
            "related" {
                if ($line -match '(SPEC-\d{4})') {
                    $current.Related += $matches[1]
                }
            }
            default {
                # unrecognized content between sections - ignore
            }
        }
    }

    if ($null -ne $current) {
        $entries += $current
    }

    return $entries
}

function Find-FeatureIndexEntry {
    param(
        [Parameter(Mandatory = $true)]
        [array]$Entries,

        [Parameter(Mandatory = $true)]
        [string]$SpecNameOrId
    )

    # [regex]::Match is case-sensitive by .NET default (unlike PowerShell's
    # -match operator), so without IgnoreCase a lowercase "spec-0007..."
    # argument would fail to match and this would wrongly report "not found".
    $idMatch = [regex]::Match($SpecNameOrId, 'SPEC-\d{4}', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
    if (!$idMatch.Success) {
        return $null
    }
    $specId = $idMatch.Value.ToUpper()

    return $Entries | Where-Object { $_.SpecId -eq $specId } | Select-Object -First 1
}

function Test-SpecFileExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FeaturesPath,

        [Parameter(Mandatory = $true)]
        [string]$SpecId
    )

    $match = Get-ChildItem $FeaturesPath -Filter "$SpecId-*.md" -ErrorAction SilentlyContinue |
        Select-Object -First 1

    return $match
}
