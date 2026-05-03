#Requires -Version 5.1
<#
.SYNOPSIS
    Ding Network Scanner — Windows installer.

.DESCRIPTION
    Installs Ding on Windows. Supports two modes:
      1. Docker Desktop  — recommended; isolated, easy to update.
      2. Native binary   — runs as a Windows Service; requires Npcap.

    Must be run as Administrator.

.PARAMETER Mode
    "docker" or "native". Omit to be prompted.

.PARAMETER InstallDir
    Installation directory. Default: C:\Program Files\Ding

.EXAMPLE
    # Interactive (recommended):
    irm https://raw.githubusercontent.com/hamed0406/ding/main/scripts/install.ps1 | iex

    # Non-interactive Docker install:
    .\install.ps1 -Mode docker

    # Non-interactive native install to a custom path:
    .\install.ps1 -Mode native -InstallDir D:\Ding
#>
param(
    [ValidateSet('docker','native','')]
    [string]$Mode = '',

    [string]$InstallDir = 'C:\Program Files\Ding'
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── Colours ────────────────────────────────────────────────────────────────────
function Write-Header  { param([string]$Msg) Write-Host "`n$Msg" -ForegroundColor White }
function Write-Info    { param([string]$Msg) Write-Host "  > $Msg" -ForegroundColor Cyan }
function Write-OK      { param([string]$Msg) Write-Host "  [OK] $Msg" -ForegroundColor Green }
function Write-Warn    { param([string]$Msg) Write-Host "  [!]  $Msg" -ForegroundColor Yellow }
function Write-Fail    { param([string]$Msg) Write-Host "  [X]  $Msg" -ForegroundColor Red; exit 1 }

# ── Config ─────────────────────────────────────────────────────────────────────
$DockerImage    = 'hamed0406/ding:latest'
$RepoRaw        = 'https://raw.githubusercontent.com/hamed0406/ding/main'
$GhReleases     = 'https://github.com/hamed0406/ding/releases/latest/download'
$NpcapUrl       = 'https://npcap.com/dist/npcap-1.80.exe'  # update as new versions ship
$ServiceName    = 'Ding'
$ServiceDesc    = 'Ding Network Scanner'

# ── Banner ─────────────────────────────────────────────────────────────────────
Write-Host ''
Write-Host '  ██████╗ ██╗███╗   ██╗ ██████╗ ' -ForegroundColor Cyan
Write-Host '  ██╔══██╗██║████╗  ██║██╔════╝ ' -ForegroundColor Cyan
Write-Host '  ██║  ██║██║██╔██╗ ██║██║  ███╗' -ForegroundColor Cyan
Write-Host '  ██║  ██║██║██║╚██╗██║██║   ██║' -ForegroundColor Cyan
Write-Host '  ██████╔╝██║██║ ╚████║╚██████╔╝' -ForegroundColor Cyan
Write-Host '  ╚═════╝ ╚═╝╚═╝  ╚═══╝ ╚═════╝ ' -ForegroundColor Cyan
Write-Host ''
Write-Host '  Network Scanner — Windows Installer' -ForegroundColor White
Write-Host ''

# ── Pre-flight: Administrator ──────────────────────────────────────────────────
$IsAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $IsAdmin) {
    Write-Fail 'This installer must be run as Administrator.
    Right-click PowerShell → "Run as administrator", then re-run the script.'
}
Write-OK 'Running as Administrator'

# ── Windows version ────────────────────────────────────────────────────────────
$WinVer = [System.Environment]::OSVersion.Version
if ($WinVer.Major -lt 10) {
    Write-Fail "Windows 10 or Server 2016 or later is required (detected: $($WinVer.ToString()))."
}
Write-OK "Windows $($WinVer.ToString())"

# ── TLS 1.2 for downloads ──────────────────────────────────────────────────────
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# ── Helper: download file ──────────────────────────────────────────────────────
function Get-RemoteFile {
    param([string]$Url, [string]$Dest)
    Write-Info "Downloading $(Split-Path $Url -Leaf)..."
    try {
        $wc = New-Object System.Net.WebClient
        $wc.DownloadFile($Url, $Dest)
    } catch {
        Write-Fail "Download failed: $Url`n  $_"
    }
}

# ── Helper: generate secret key ───────────────────────────────────────────────
function New-SecretKey {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return [Convert]::ToBase64String($bytes)
}

# ── Installation mode ──────────────────────────────────────────────────────────
Write-Header 'Installation Method'
if ($Mode -eq '') {
    Write-Host '  How would you like to install Ding?'
    Write-Host ''
    Write-Host '    1) Docker Desktop  — recommended; isolated, easy to update' -ForegroundColor White
    Write-Host '    2) Native binary   — runs as a Windows Service; requires Npcap' -ForegroundColor White
    Write-Host ''
    $choice = Read-Host '  Choose [1/2] (default: 1)'
    $Mode = if ($choice -eq '2') { 'native' } else { 'docker' }
}

# ══════════════════════════════════════════════════════════════════════════════
# DOCKER INSTALLATION
# ══════════════════════════════════════════════════════════════════════════════
if ($Mode -eq 'docker') {

    Write-Header 'Checking prerequisites (Docker)'

    # Docker Desktop / Docker Engine for Windows
    $dockerCmd = Get-Command docker -ErrorAction SilentlyContinue
    if ($null -eq $dockerCmd) {
        Write-Warn 'Docker not found.'
        Write-Host ''
        Write-Host '  Docker Desktop for Windows is required. Download it from:'
        Write-Host '  https://www.docker.com/products/docker-desktop/' -ForegroundColor Cyan
        Write-Host ''
        Write-Host '  After installing Docker Desktop:'
        Write-Host '    1. Launch Docker Desktop and wait for it to finish starting.'
        Write-Host '    2. Re-run this installer.'
        Write-Host ''
        Write-Fail 'Docker is not installed. Install Docker Desktop first.'
    }
    $dockerVer = (docker --version 2>&1) -replace '[^0-9.]','' | Select-Object -First 1
    Write-OK "Docker $dockerVer"

    # Docker daemon running?
    $dockerInfo = docker info 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Fail 'Docker daemon is not running.
    Start Docker Desktop from the Start Menu and wait for it to become ready, then re-run.'
    }
    Write-OK 'Docker daemon running'

    # WSL2 / Hyper-V warning — containers use Linux VM, so host network scanning won't work
    Write-Warn 'Important limitation: Docker on Windows uses a Linux VM.'
    Write-Warn 'network_mode: host attaches the VM network — not your physical LAN.'
    Write-Warn 'ARP scanning will discover the VM network, not your real devices.'
    Write-Host ''
    Write-Host '  For full LAN scanning use the Native binary option instead (choice 2).' -ForegroundColor Yellow
    Write-Host ''
    $cont = Read-Host '  Continue with Docker anyway? [y/N]'
    if ($cont.ToLower() -ne 'y') {
        Write-Host 'Exiting. Run the installer again and choose option 2 for native binary.'
        exit 0
    }

    # Docker Compose
    $composeVer = docker compose version 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Fail 'Docker Compose plugin not found. Update Docker Desktop to get it bundled.'
    }
    Write-OK 'Docker Compose available'

    # ── Create install directory ─────────────────────────────────────────────────
    Write-Header "Setting up $InstallDir"
    New-Item -ItemType Directory -Force -Path "$InstallDir\data" | Out-Null

    # docker-compose.yml
    $composeFile = Join-Path $InstallDir 'docker-compose.yml'
    if (-not (Test-Path $composeFile)) {
        Get-RemoteFile "$RepoRaw/docker-compose.yml" $composeFile
        Write-OK 'docker-compose.yml downloaded'
    } else {
        Write-Info 'docker-compose.yml already exists — keeping yours'
    }

    # ── .env ─────────────────────────────────────────────────────────────────────
    Write-Header 'Configuration'
    $envFile = Join-Path $InstallDir '.env'
    if (Test-Path $envFile) {
        Write-Info '.env already exists — keeping your settings'
    } else {
        $exampleFile = Join-Path $InstallDir '.env.example'
        Get-RemoteFile "$RepoRaw/.env.example" $exampleFile
        $SecretKey = New-SecretKey
        $envContent = (Get-Content $exampleFile -Raw) -replace '(?m)^DING_SECRET_KEY=.*', "DING_SECRET_KEY=$SecretKey"
        Set-Content -Path $envFile -Value $envContent -Encoding UTF8
        Write-OK 'Secret key generated and saved to .env'
        Write-Host ''
        Write-Warn 'Back up this key — you need it if you ever move the database:'
        Write-Host "  DING_SECRET_KEY=$SecretKey" -ForegroundColor White
        Write-Host ''
    }

    # ── Optional Telegram ─────────────────────────────────────────────────────────
    Write-Host ''
    $setupTg = Read-Host 'Set up Telegram alerts now? [y/N]'
    if ($setupTg.ToLower() -eq 'y') {
        Write-Host ''
        Write-Host '  1. Message @BotFather on Telegram → /newbot'
        Write-Host '  2. Copy your bot token  (e.g. 123456:ABCdef...)'
        Write-Host '  3. Start a chat with your bot, then open:'
        Write-Host '     https://api.telegram.org/bot<TOKEN>/getUpdates'
        Write-Host '     to find your chat_id'
        Write-Host ''
        $tgToken  = Read-Host '  Bot token'
        $tgChatId = Read-Host '  Chat ID'
        if ($tgToken -and $tgChatId) {
            $envContent = Get-Content $envFile -Raw
            $envContent = $envContent -replace '(?m)^DING_TELEGRAM_TOKEN=.*',  "DING_TELEGRAM_TOKEN=$tgToken"
            $envContent = $envContent -replace '(?m)^DING_TELEGRAM_CHAT_ID=.*',"DING_TELEGRAM_CHAT_ID=$tgChatId"
            Set-Content -Path $envFile -Value $envContent -Encoding UTF8
            Write-OK 'Telegram configured'
        } else {
            Write-Warn 'Skipped — configure it later in Settings → Telegram'
        }
    }

    # ── Pull and start ───────────────────────────────────────────────────────────
    Write-Header 'Starting Ding'
    Write-Info "Pulling Docker image ($DockerImage)..."
    docker pull $DockerImage
    if ($LASTEXITCODE -ne 0) { Write-Fail 'docker pull failed.' }

    Set-Location $InstallDir
    Write-Info 'Starting service...'
    docker compose up -d
    if ($LASTEXITCODE -ne 0) { Write-Fail 'docker compose up failed.' }

    Write-Info 'Waiting for server to start...'
    $ready = $false
    for ($i = 1; $i -le 20; $i++) {
        try {
            $r = Invoke-WebRequest -Uri 'http://localhost:8081/api/auth/providers' -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch {}
        Start-Sleep -Seconds 1
    }

    Write-Host ''
    Write-Host '  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━' -ForegroundColor Green
    Write-Host '  Ding is running!' -ForegroundColor Green
    Write-Host '  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━' -ForegroundColor Green
    Write-Host ''
    Write-Host '  Open in your browser: http://localhost:8081' -ForegroundColor White
    Write-Host ''
    Write-Host '  First visit: create your account (email + password).'
    Write-Host '  Then Ding will scan your network automatically.'
    Write-Host ''
    Write-Host '  Useful commands (run from ' + $InstallDir + '):'
    Write-Host '    View logs:  docker compose logs -f'
    Write-Host '    Stop:       docker compose down'
    Write-Host "    Update:     docker pull $DockerImage && docker compose up -d"
    Write-Host ''
    Write-Warn "Keep these safe:"
    Write-Host "    $InstallDir\.env          <- config and secret key"
    Write-Host "    $InstallDir\data\ding.db  <- scan history database"
    Write-Host ''
    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# NATIVE BINARY INSTALLATION
# ══════════════════════════════════════════════════════════════════════════════

Write-Header 'Checking prerequisites (native binary)'

# ── Npcap ─────────────────────────────────────────────────────────────────────
Write-Info 'Checking for Npcap (required for ARP/ICMP packet capture)...'
$npcapInstalled = $false
$npcapKey = 'HKLM:\SOFTWARE\WOW6432Node\Npcap'
if (Test-Path $npcapKey) {
    $npcapVer = (Get-ItemProperty $npcapKey -ErrorAction SilentlyContinue).ProductVersion
    Write-OK "Npcap $npcapVer"
    $npcapInstalled = $true
}
if (-not $npcapInstalled) {
    Write-Warn 'Npcap is not installed.'
    Write-Host ''
    Write-Host '  Npcap is required for ARP and ICMP packet capture on Windows.' -ForegroundColor Yellow
    Write-Host '  License: free for personal / non-commercial use.' -ForegroundColor Yellow
    Write-Host ''
    $installNpcap = Read-Host '  Download and install Npcap now? [y/N]'
    if ($installNpcap.ToLower() -eq 'y') {
        $npcapTmp = Join-Path $env:TEMP 'npcap-installer.exe'
        Get-RemoteFile $NpcapUrl $npcapTmp
        Write-Info 'Launching Npcap installer (follow the on-screen prompts)...'
        Start-Process -FilePath $npcapTmp -Wait
        # Re-check
        if (Test-Path $npcapKey) {
            $npcapVer = (Get-ItemProperty $npcapKey -ErrorAction SilentlyContinue).ProductVersion
            Write-OK "Npcap $npcapVer installed"
        } else {
            Write-Fail 'Npcap installation did not complete. Re-run the installer after installing Npcap manually from https://npcap.com'
        }
    } else {
        Write-Fail 'Npcap is required. Download it from https://npcap.com and re-run this installer.'
    }
}

# ── Detect architecture ────────────────────────────────────────────────────────
$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -eq 'AMD64') {
    $ArchSuffix = 'windows-x86_64'
} elseif ($Arch -eq 'ARM64') {
    $ArchSuffix = 'windows-aarch64'
} else {
    Write-Fail "Unsupported architecture: $Arch. Only x86_64 and ARM64 are supported."
}
Write-OK "Architecture: $Arch"

# ── Check if Windows Service already exists ────────────────────────────────────
$existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($null -ne $existingService) {
    Write-Warn "Service '$ServiceName' already exists (status: $($existingService.Status))."
    $overwrite = Read-Host '  Stop existing service and reinstall? [y/N]'
    if ($overwrite.ToLower() -ne 'y') {
        Write-Host 'Exiting — existing install preserved.'
        exit 0
    }
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
    Write-Info 'Existing service removed'
}

# ── Download binary ────────────────────────────────────────────────────────────
Write-Header "Downloading Ding ($ArchSuffix)"

$ZipName = "ding-$ArchSuffix.zip"
$ZipUrl  = "$GhReleases/$ZipName"
$TmpDir  = Join-Path $env:TEMP "ding-install-$([System.IO.Path]::GetRandomFileName())"
New-Item -ItemType Directory -Force -Path $TmpDir | Out-Null

$ZipPath = Join-Path $TmpDir $ZipName
Get-RemoteFile $ZipUrl $ZipPath

# Checksum
$sha256Url = "$GhReleases/$ZipName.sha256"
try {
    $expectedHash = (Invoke-WebRequest -Uri $sha256Url -UseBasicParsing -ErrorAction Stop).Content.Trim().Split()[0]
    $actualHash   = (Get-FileHash $ZipPath -Algorithm SHA256).Hash.ToLower()
    if ($expectedHash.ToLower() -ne $actualHash) {
        Write-Fail "Checksum mismatch!`n  Expected: $expectedHash`n  Got:      $actualHash`nDownload may be corrupt — try again."
    }
    Write-OK 'Checksum verified'
} catch {
    Write-Warn 'No .sha256 file found — skipping checksum verification'
}

Write-Info 'Extracting archive...'
Expand-Archive -Path $ZipPath -DestinationPath $TmpDir -Force

$dingBin    = Join-Path $TmpDir 'ding.exe'
$scannerBin = Join-Path $TmpDir 'scanner.exe'
foreach ($bin in @($dingBin, $scannerBin)) {
    if (-not (Test-Path $bin)) {
        Write-Fail "Expected '$bin' in archive — archive layout may have changed."
    }
}
Write-OK 'Binaries extracted'

# ── Install to InstallDir ──────────────────────────────────────────────────────
Write-Header "Installing to $InstallDir"
New-Item -ItemType Directory -Force -Path $InstallDir          | Out-Null
New-Item -ItemType Directory -Force -Path "$InstallDir\data"   | Out-Null

Copy-Item $dingBin    -Destination "$InstallDir\ding.exe"    -Force
Copy-Item $scannerBin -Destination "$InstallDir\scanner.exe" -Force
Write-OK "ding.exe    -> $InstallDir\ding.exe"
Write-OK "scanner.exe -> $InstallDir\scanner.exe"

# Cleanup temp
Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue

# ── Generate .env ──────────────────────────────────────────────────────────────
Write-Header 'Configuration'
$envFile = Join-Path $InstallDir '.env'
if (Test-Path $envFile) {
    Write-Info '.env already exists — keeping your settings'
} else {
    $exampleFile = Join-Path $InstallDir '.env.example'
    Get-RemoteFile "$RepoRaw/.env.example" $exampleFile
    $SecretKey = New-SecretKey
    $dataPath  = "$InstallDir\data\ding.db" -replace '\\','\\'
    $envContent = (Get-Content $exampleFile -Raw)
    $envContent = $envContent -replace '(?m)^DING_SECRET_KEY=.*', "DING_SECRET_KEY=$SecretKey"
    $envContent = $envContent -replace '(?m)^DING_DATA_PATH=.*',  "DING_DATA_PATH=$($InstallDir -replace '\\','/')/data/ding.db"
    $envContent = $envContent -replace '(?m)^DING_HTTP_ADDR=.*',  'DING_HTTP_ADDR=:8081'
    Set-Content -Path $envFile -Value $envContent -Encoding UTF8
    Write-OK 'Secret key generated and saved to .env'
    Write-Host ''
    Write-Warn 'Back up this key — you need it if you ever move the database:'
    Write-Host "  DING_SECRET_KEY=$SecretKey" -ForegroundColor White
    Write-Host ''
}

# Optional Telegram
Write-Host ''
$setupTg = Read-Host 'Set up Telegram alerts now? [y/N]'
if ($setupTg.ToLower() -eq 'y') {
    Write-Host ''
    Write-Host '  1. Message @BotFather on Telegram → /newbot'
    Write-Host '  2. Copy your bot token  (e.g. 123456:ABCdef...)'
    Write-Host '  3. Start a chat with your bot, then open:'
    Write-Host '     https://api.telegram.org/bot<TOKEN>/getUpdates'
    Write-Host '     to find your chat_id'
    Write-Host ''
    $tgToken  = Read-Host '  Bot token'
    $tgChatId = Read-Host '  Chat ID'
    if ($tgToken -and $tgChatId) {
        $envContent = Get-Content $envFile -Raw
        $envContent = $envContent -replace '(?m)^DING_TELEGRAM_TOKEN=.*',   "DING_TELEGRAM_TOKEN=$tgToken"
        $envContent = $envContent -replace '(?m)^DING_TELEGRAM_CHAT_ID=.*', "DING_TELEGRAM_CHAT_ID=$tgChatId"
        Set-Content -Path $envFile -Value $envContent -Encoding UTF8
        Write-OK 'Telegram configured'
    } else {
        Write-Warn 'Skipped — configure it later in Settings → Telegram'
    }
}

# ── Register Windows Service ───────────────────────────────────────────────────
Write-Header 'Registering Windows Service'

# Build the env var string for the service (read from .env at runtime via ding binary)
# ding reads env vars from the process environment; we pass the .env path via DING_ENV_FILE
# or set each variable in the service environment block.

# Load .env into a hashtable for the service environment
$envVars = @{}
Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
    $parts = $_ -split '=', 2
    if ($parts.Count -eq 2 -and $parts[0].Trim()) {
        $envVars[$parts[0].Trim()] = $parts[1].Trim()
    }
}
$envVars['DING_SCANNER_BIN'] = "$InstallDir\scanner.exe"

# Create service using sc.exe (works without NSSM)
$binPath = "`"$InstallDir\ding.exe`""
sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= $ServiceDesc | Out-Null
sc.exe description $ServiceName $ServiceDesc | Out-Null

# Set environment variables for the service via registry
$svcRegPath = "HKLM:\SYSTEM\CurrentControlSet\Services\$ServiceName"
$envStrings = ($envVars.GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" })
New-ItemProperty -Path $svcRegPath -Name 'Environment' -Value $envStrings -PropertyType MultiString -Force | Out-Null

Write-OK "Service '$ServiceName' registered"

# ── Windows Firewall ───────────────────────────────────────────────────────────
Write-Info 'Adding Windows Firewall rule for port 8081...'
$fwRule = Get-NetFirewallRule -DisplayName 'Ding Web UI' -ErrorAction SilentlyContinue
if ($null -eq $fwRule) {
    New-NetFirewallRule -DisplayName 'Ding Web UI' `
        -Direction Inbound -Protocol TCP -LocalPort 8081 `
        -Action Allow -Profile Any | Out-Null
    Write-OK 'Firewall rule added (TCP 8081 inbound)'
} else {
    Write-Info 'Firewall rule already exists'
}

# ── Start service ──────────────────────────────────────────────────────────────
Write-Info "Starting service '$ServiceName'..."
Start-Service -Name $ServiceName -ErrorAction Stop
Write-OK "Service started"

# Wait for server
Write-Info 'Waiting for Ding to start...'
$ready = $false
for ($i = 1; $i -le 20; $i++) {
    try {
        $r = Invoke-WebRequest -Uri 'http://localhost:8081/api/auth/providers' -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
        if ($r.StatusCode -eq 200) { $ready = $true; break }
    } catch {}
    Start-Sleep -Seconds 1
}

if (-not $ready) {
    Write-Warn 'Server did not respond within 20 seconds.'
    Write-Warn "Check the Windows Event Log: Get-EventLog -LogName Application -Source $ServiceName"
}

# ── Done ───────────────────────────────────────────────────────────────────────
Write-Host ''
Write-Host '  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━' -ForegroundColor Green
Write-Host '  Ding is running!' -ForegroundColor Green
Write-Host '  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━' -ForegroundColor Green
Write-Host ''
Write-Host '  Open in your browser: http://localhost:8081' -ForegroundColor White
Write-Host ''
Write-Host '  First visit: create your account (email + password).'
Write-Host '  Then Ding will scan your network automatically.'
Write-Host ''
Write-Host '  Useful commands (PowerShell as Administrator):'
Write-Host "    View logs:   Get-EventLog -LogName Application -Source $ServiceName -Newest 50"
Write-Host "    Stop:        Stop-Service $ServiceName"
Write-Host "    Start:       Start-Service $ServiceName"
Write-Host "    Update:      irm $RepoRaw/scripts/install.ps1 | iex"
Write-Host ''
Write-Host '  Keep these safe:' -ForegroundColor Yellow
Write-Host "    $InstallDir\.env          <- config and secret key"
Write-Host "    $InstallDir\data\ding.db  <- scan history database"
Write-Host ''
