$ErrorActionPreference = "Stop"

Write-Host "Syncing go_backend from upstream..." -ForegroundColor Cyan

# Path to the vendored go_backend inside the repo
$VENDORED_DIR = "internal/go_backend"

# Temporary directory for cloning upstream
$Guid = [Guid]::NewGuid().Guid
$TMP_DIR = Join-Path $env:TEMP "flacapi-sync-$Guid"

# Upstream repository (SpotiFLAC-Mobile)
$UPSTREAM_REPO = "https://github.com/zarzet/SpotiFLAC-Mobile.git"

# Clone upstream (depth 1 for speed)
git clone --depth 1 $UPSTREAM_REPO $TMP_DIR

# Ensure vendored directory exists and is clean
if (Test-Path $VENDORED_DIR) {
    Remove-Item -Recurse -Force "$VENDORED_DIR/*"
} else {
    New-Item -ItemType Directory -Path $VENDORED_DIR -Force | Out-Null
}

# Copy the go_backend directory from the clone to our vendored location, overwriting existing
Copy-Item -Path "$TMP_DIR/go_backend/*" -Destination $VENDORED_DIR -Recurse -Force

# Clean up the temporary folder
Remove-Item -Recurse -Force $TMP_DIR

# Run go mod tidy from the repository root
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location "$ScriptDir\.."
try {
    go mod tidy
} finally {
    Pop-Location
}

Write-Host "go_backend sync completed." -ForegroundColor Green
