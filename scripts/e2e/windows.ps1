# Real Windows session for the e2e test: two console windows with known
# titles, focused through WScript.Shell. Runs on windows-latest.
$ErrorActionPreference = 'Stop'
foreach ($t in 'tock-e2e-alpha', 'tock-e2e-beta') {
    Start-Process powershell -ArgumentList '-NoExit', '-Command', "`$host.UI.RawUI.WindowTitle = '$t'"
}
Start-Sleep -Seconds 4
$env:E2E_TITLES = 'tock-e2e-alpha,tock-e2e-beta'
$env:E2E_FOCUS = "if (-not (New-Object -ComObject WScript.Shell).AppActivate('{title}')) { exit 1 }"
go test -tags e2e ./e2e -v -count=1
exit $LASTEXITCODE
