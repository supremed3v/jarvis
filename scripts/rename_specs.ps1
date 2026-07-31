# ============================================
# JARVIS SPEC FILE RENAMER
# ============================================
#
# Converts:
# SPEC-0001_Repository_Foundation.md
#
# Into:
# SPEC-0001-repository-foundation.md
#
# ============================================


$featuresPath = "../context/features"


if (!(Test-Path $featuresPath)) {
    Write-Host "ERROR: Features folder not found"
    exit 1
}


Get-ChildItem $featuresPath -Filter "*.md" | ForEach-Object {

    $oldName = $_.Name


    # Skip index files
    if ($oldName -eq "FEATURE_INDEX.md") {
        return
    }


    if ($oldName -match "^SPEC-(\d{4})[_-](.+)\.md$") {

        # -match is case-insensitive, so this also catches "spec-0001..."
        # etc. Rebuild the prefix explicitly rather than reusing the
        # matched text, so wrongly-cased prefixes get normalized instead
        # of silently passing through unrenamed. Digits are pinned to
        # exactly 4 to match the canonical SPEC-NNNN format enforced by
        # validate_specs.ps1.
        $specNumber = "SPEC-" + $matches[1]

        $title = $matches[2]


        $newTitle = $title `
            -replace "_","-" `
            -replace "\s+","-" `
            -replace "-+","-"


        $newTitle = $newTitle.ToLower()


        $newName = "$specNumber-$newTitle.md"


        if ($oldName -ne $newName) {


            Write-Host ""
            Write-Host "Renaming:"
            Write-Host "$oldName"
            Write-Host "   ->"
            Write-Host "$newName"


            Rename-Item `
                -Path $_.FullName `
                -NewName $newName

        }

    }

}


Write-Host ""
Write-Host "================================"
Write-Host "Spec rename completed"
Write-Host "================================"