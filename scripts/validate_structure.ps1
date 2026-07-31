# ============================================
# JARVIS REPOSITORY STRUCTURE VALIDATOR
# ============================================
#
# Ensures the top-level repository structure required by the
# feature workflow (and target architecture) is intact.
#
# Run from inside scripts/ (uses relative ../ paths).
#
# ============================================

$root = ".."

$requiredPaths = @(
    ".claude",
    "context",
    "context/features",
    "docs",
    "docs/architecture",
    "docs/execution",
    "docs/decisions",
    "docs/agents",
    "scripts",
    "apps",
    "services",
    "agents",
    "packages",
    "tests"
)

Write-Host ""
Write-Host "================================"
Write-Host "JARVIS Repository Structure Check"
Write-Host "================================"
Write-Host ""

$existing = 0
$missing = 0

foreach ($path in $requiredPaths) {

    $fullPath = Join-Path $root $path

    if (Test-Path $fullPath) {
        Write-Host "$([char]0x2713) Existing   $path"
        $existing++
    } else {
        Write-Host "$([char]0x2717) Missing    $path"
        $missing++
    }
}

Write-Host ""
Write-Host "================================"
Write-Host "Summary"
Write-Host "================================"
Write-Host "Checked:  $($requiredPaths.Count)"
Write-Host "Existing: $existing"
Write-Host "Missing:  $missing"
Write-Host ""

if ($missing -gt 0) {
    exit 1
} else {
    exit 0
}
