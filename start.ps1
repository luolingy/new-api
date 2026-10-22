#Requires -Version 5.1
# One-click launcher for new-api: starts the built new-api.exe (no build step).
# Double-click start.bat, or run:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\start.ps1
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

$exe = Join-Path $PSScriptRoot 'new-api.exe'
if (-not (Test-Path -LiteralPath $exe)) {
    Write-Host 'new-api.exe was not found.' -ForegroundColor Red
    Write-Host 'Build it once first (needs web/dist):  go build -o new-api.exe .' -ForegroundColor Yellow
    exit 1
}

# The server loads ./.env itself (godotenv). We read PORT and SQL_DSN only to
# know where to open the browser and which database to report.
$port = 3000
$sqlDsn = ''
$sqlitePath = ''
$envFile = Join-Path $PSScriptRoot '.env'
if (Test-Path -LiteralPath $envFile) {
    foreach ($line in (Get-Content -LiteralPath $envFile)) {
        if ($line -match '^\s*PORT\s*=\s*(\d+)\s*$') { $port = [int]$Matches[1] }
        elseif ($line -match '^\s*SQL_DSN\s*=\s*(\S.*)$') { $sqlDsn = $Matches[1] }
        elseif ($line -match '^\s*SQLITE_PATH\s*=\s*(\S.*)$') { $sqlitePath = $Matches[1] }
    }
}

$url = "http://127.0.0.1:$port"
$dbLabel = 'SQLite (one-api.db in the working directory)'
if ($sqlitePath) { $dbLabel = "SQLite ($sqlitePath)" }
$dbHost = ''
$dbPort = 0
if ($sqlDsn) {
    if ($sqlDsn -match '@([^/:@\[\]]+):(\d+)/([^?/]+)') {
        $dbHost = $Matches[1]
        $dbPort = [int]$Matches[2]
        $dbLabel = "PostgreSQL $dbHost`:$dbPort/$($Matches[3])"
    } else {
        $dbLabel = 'external database (see SQL_DSN in .env)'
    }
}

# Already running? Then only open the UI.
if (netstat -ano | Select-String -Pattern ":$port\s.*LISTENING") {
    Write-Host "Port $port is already in use - assuming new-api is already running." -ForegroundColor Yellow
    Start-Process $url
    exit 0
}

# Only a local PostgreSQL can be started from here; a remote one just gets a warning.
if ($dbPort -gt 0 -and ($dbHost -eq '127.0.0.1' -or $dbHost -eq 'localhost')) {
    $dbReady = $false
    foreach ($attempt in 1..3) {
        try {
            $client = New-Object System.Net.Sockets.TcpClient
            if ($client.ConnectAsync($dbHost, $dbPort).Wait(2000) -and $client.Connected) { $dbReady = $true }
            $client.Close()
        } catch {
            # not reachable yet
        }
        if ($dbReady) { break }
        if ($attempt -eq 1) {
            $service = Get-Service -Name 'postgres*' -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($service -and $service.Status -ne 'Running') {
                Write-Host "PostgreSQL is down, starting service $($service.Name) ..." -ForegroundColor Yellow
                Start-Service -Name $service.Name -ErrorAction SilentlyContinue
            }
        }
        Start-Sleep -Seconds 2
    }
    if (-not $dbReady) {
        Write-Host "WARNING: PostgreSQL $dbHost`:$dbPort is not reachable. Start it first, or fix SQL_DSN in .env." -ForegroundColor Red
    }
}

Write-Host "Database: $dbLabel"
Write-Host "Starting new-api on $url ..." -ForegroundColor Cyan
$proc = Start-Process -FilePath $exe -WorkingDirectory $PSScriptRoot -PassThru -NoNewWindow

# Wait for readiness, then open the browser.
# The first start runs database migrations on a slow disk and can take several minutes.
Write-Host 'Waiting for the server (migrations on first start can take minutes) ...'
$ready = $false
$start = Get-Date
$deadline = $start.AddSeconds(600)
$tick = 0
while ((Get-Date) -lt $deadline) {
    if ($proc.HasExited) { break }
    try {
        $resp = Invoke-WebRequest -Uri "$url/api/status" -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -eq 200) { $ready = $true; break }
    } catch {
        # not up yet
    }
    $tick++
    if ($tick % 20 -eq 0) { Write-Host '.' -NoNewline }
    Start-Sleep -Milliseconds 1500
}
Write-Host ''

if ($ready) {
    Write-Host ''
    Write-Host "new-api is ready: $url" -ForegroundColor Green
    Start-Process $url
} elseif ($proc.HasExited) {
    Write-Host "new-api exited with code $($proc.ExitCode)." -ForegroundColor Red
    Write-Host 'Check the log output above, or the newest file under logs\.' -ForegroundColor Yellow
    exit $proc.ExitCode
} else {
    Write-Host 'new-api did not answer in time; it is still starting in the foreground.' -ForegroundColor Yellow
}

$proc.WaitForExit()
