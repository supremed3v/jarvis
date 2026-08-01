# ============================================
# JARVIS DEVELOPMENT ENVIRONMENT VERIFIER
# ============================================
#
# Checks that the local toolchain required for JARVIS development
# (SPEC-0002) is present: Go, Node.js/npm, and Ollama. Reports
# detected versions against the repo's pinned versions (.go-version,
# .nvmrc) and confirms .env.example exists and is well-formed.
#
# This is a diagnostic/reporting script, not an installer. No app
# code exists yet (go.mod/package.json arrive in SPEC-0007/SPEC-0063),
# so this only smoke-tests toolchain presence.
#
# Run from inside scripts/ (uses relative ../ paths).
#
# ============================================

$root = ".."

$warnings = 0
$failures = 0

Write-Host ""
Write-Host "================================"
Write-Host "JARVIS Dev Environment Check"
Write-Host "================================"
Write-Host ""

# --- Go ---
$goVersionPin = (Get-Content (Join-Path $root ".go-version") -ErrorAction SilentlyContinue | Select-Object -First 1)

try {
    $goOutput = & go version 2>$null
    if ($LASTEXITCODE -eq 0 -and $goOutput) {
        Write-Host "$([char]0x2713) Go found      $goOutput"
        if ($goVersionPin -and ($goOutput -notmatch [regex]::Escape($goVersionPin))) {
            Write-Host "  NOTE: installed Go version does not match pinned .go-version ($goVersionPin)"
            $warnings++
        }
    } else {
        Write-Host "$([char]0x2717) Go missing    'go version' did not succeed"
        $failures++
    }
} catch {
    Write-Host "$([char]0x2717) Go missing    'go' not found on PATH"
    $failures++
}

# --- Node / npm ---
$nvmrcPin = (Get-Content (Join-Path $root ".nvmrc") -ErrorAction SilentlyContinue | Select-Object -First 1)

try {
    $nodeOutput = & node --version 2>$null
    if ($LASTEXITCODE -eq 0 -and $nodeOutput) {
        Write-Host "$([char]0x2713) Node found    $nodeOutput"
        if ($nvmrcPin -and ($nodeOutput -notmatch [regex]::Escape($nvmrcPin))) {
            Write-Host "  NOTE: installed Node version does not match pinned .nvmrc ($nvmrcPin)"
            $warnings++
        }
    } else {
        Write-Host "$([char]0x2717) Node missing  'node --version' did not succeed"
        $failures++
    }
} catch {
    Write-Host "$([char]0x2717) Node missing  'node' not found on PATH"
    $failures++
}

try {
    $npmOutput = & npm --version 2>$null
    if ($LASTEXITCODE -eq 0 -and $npmOutput) {
        Write-Host "$([char]0x2713) npm found     $npmOutput"
    } else {
        Write-Host "$([char]0x2717) npm missing   'npm --version' did not succeed"
        $failures++
    }
} catch {
    Write-Host "$([char]0x2717) npm missing   'npm' not found on PATH"
    $failures++
}

# --- Ollama (manual local prerequisite, not hard-required) ---
try {
    $ollamaOutput = & ollama --version 2>$null
    if ($LASTEXITCODE -eq 0 -and $ollamaOutput) {
        Write-Host "$([char]0x2713) Ollama found  $ollamaOutput"
    } else {
        Write-Host "$([char]0x2717) Ollama missing (optional today; required before running any LLM-backed feature)"
        $warnings++
    }
} catch {
    Write-Host "$([char]0x2717) Ollama missing (optional today; required before running any LLM-backed feature)"
    $warnings++
}

# --- .env.example ---
$envExamplePath = Join-Path $root ".env.example"
if (Test-Path $envExamplePath) {
    $lines = Get-Content $envExamplePath
    $malformed = $lines | Where-Object {
        $_.Trim() -ne "" -and $_.Trim() -notmatch "^#" -and $_ -notmatch "^[A-Za-z_][A-Za-z0-9_]*="
    }
    if ($malformed.Count -eq 0) {
        Write-Host "$([char]0x2713) .env.example  present and well-formed"
    } else {
        Write-Host "$([char]0x2717) .env.example  present but contains malformed lines"
        $failures++
    }
} else {
    Write-Host "$([char]0x2717) .env.example  missing"
    $failures++
}

Write-Host ""
Write-Host "================================"
Write-Host "Summary"
Write-Host "================================"
Write-Host "Warnings: $warnings"
Write-Host "Failures: $failures"
Write-Host ""

if ($failures -gt 0) {
    exit 1
} else {
    exit 0
}
