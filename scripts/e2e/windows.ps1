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
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Start at sign-in: trackd install writes the Run key and starts trackd,
# trackd uninstall removes the key, tock stop ends it.
$bin = Join-Path $env:RUNNER_TEMP 'tockbin'
go build -o "$bin\tock.exe" ./cmd/tock
go build -o "$bin\trackd.exe" ./cmd/trackd
$env:TOCK_DATA_DIR = Join-Path $env:RUNNER_TEMP 'tockdata'
$env:TOCK_STATE_DIR = Join-Path $env:RUNNER_TEMP 'tockstate'
$env:TOCK_CONFIG_DIR = Join-Path $env:RUNNER_TEMP 'tockconfig'
& "$bin\tock.exe" init
& "$bin\trackd.exe" install
if ($LASTEXITCODE -ne 0) { Write-Error 'trackd install failed' }
$run = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$v = (Get-ItemProperty -Path $run -Name 'tock trackd').'tock trackd'
if ($v -ne "`"$bin\tock.exe`" start") { Write-Error "Run key is '$v'" }
Start-Sleep -Seconds 3
if (-not (Get-Process trackd -ErrorAction SilentlyContinue)) { Write-Error 'trackd not running after install' }
& "$bin\trackd.exe" uninstall
if (Get-ItemProperty -Path $run -Name 'tock trackd' -ErrorAction SilentlyContinue) { Write-Error 'Run key left behind' }
& "$bin\tock.exe" stop
if ($LASTEXITCODE -ne 0) { Write-Error 'tock stop failed' }
Write-Output 'autostart: ok'
exit 0
