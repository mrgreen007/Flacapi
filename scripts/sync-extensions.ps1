Write-Host "Syncing extensions submodule from upstream..." -ForegroundColor Cyan

# Update the submodule to the latest commit on the remote branch
git submodule update --remote --merge extensions

Write-Host "Extensions sync completed." -ForegroundColor Green
