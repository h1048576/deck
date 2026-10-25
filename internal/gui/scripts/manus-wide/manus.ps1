param(
    [switch]$DetectOnly,
    [string]$ExecutablePath,
    [ValidatePattern('^(?:auto|fit-content|(?:0|[1-9][0-9]{0,3})(?:\.[0-9]+)?(?:px|rem|em|vw|vh|%))$')]
    [string]$Width = '70vw',
    [ValidatePattern('^(?:none|(?:0|[1-9][0-9]{0,3})(?:\.[0-9]+)?(?:px|rem|em|vw|vh|%))$')]
    [string]$MaxWidth = '90rem',
    [ValidateRange(8, 72)]
    [int]$FontSize = 17,
    [ValidateRange(100, 1000)]
    [int]$FontWeight = 200,
    [ValidateScript({ $_ -and $_ -notmatch '[;{}<>\r\n]' })]
    [string]$FontFamily = 'Maple Mono NF CN',
    [ValidateRange(1024, 65535)]
    [int]$Port = 9338,
    [switch]$Normal
)

$ErrorActionPreference = 'Stop'

function Write-Step([string]$Message) {
    if ($env:WIDE_PROGRESS_STREAM -eq '1') { [Console]::WriteLine("[Manus Wide] $Message"); return }
    Write-Host "[Manus Wide] $Message" -ForegroundColor Cyan
}

function Get-ManusApplication {
    if ($ExecutablePath) {
        $file = Get-Item -LiteralPath $ExecutablePath -ErrorAction Stop
        if ($file.PSIsContainer -or $file.Name -ine 'Manus.exe') { throw '请选择 Manus Studio 的 Manus.exe。' }
        return [pscustomobject]@{ Executable = $file.FullName; InstallRoot = $file.DirectoryName; Version = "$($file.VersionInfo.ProductVersion)" }
    }

    $candidates = New-Object 'System.Collections.Generic.List[string]'
    # 精确匹配 Studio 包名，不能将 Cue by Manus 当成 Manus Studio。
    foreach ($package in @(Get-AppxPackage -Name 'ManusAI.Manus' -ErrorAction SilentlyContinue | Sort-Object { [version]$_.Version } -Descending)) {
        if ($package.InstallLocation) { $candidates.Add((Join-Path $package.InstallLocation 'Manus.exe')) }
    }
    foreach ($processInfo in @(Get-CimInstance Win32_Process -Filter "Name = 'Manus.exe'" -ErrorAction SilentlyContinue)) {
        if ($processInfo.ExecutablePath) { $candidates.Add($processInfo.ExecutablePath) }
    }
    $uninstallRoots = @(
        'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*',
        'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*'
    )
    foreach ($entry in @(Get-ItemProperty -Path $uninstallRoots -ErrorAction SilentlyContinue | Where-Object { "$($_.DisplayName)" -match '^(?i:Manus(?: Studio)?)(?:\s+[0-9].*)?$' })) {
        $displayIcon = "$($entry.DisplayIcon)".Trim()
        if ($displayIcon -match '^"([^"]+\.exe)"') { $candidates.Add($matches[1]) }
        elseif ($displayIcon -match '^(.+?\.exe)(?:,\d+)?$') { $candidates.Add($matches[1]) }
        if ($entry.InstallLocation) { $candidates.Add((Join-Path "$($entry.InstallLocation)".Trim().Trim('"') 'Manus.exe')) }
    }
    foreach ($root in @($env:LOCALAPPDATA, $env:ProgramFiles, ${env:ProgramFiles(x86)})) {
        if (-not $root) { continue }
        foreach ($relative in @('Manus\Manus.exe', 'Manus Studio\Manus.exe', 'Programs\Manus\Manus.exe', 'Programs\Manus Studio\Manus.exe')) {
            $candidates.Add((Join-Path $root $relative))
        }
    }
    $executable = $candidates | Where-Object {
        $_ -and [IO.Path]::GetFileName($_) -ieq 'Manus.exe' -and (Test-Path -LiteralPath $_ -PathType Leaf)
    } | Select-Object -Unique -First 1
    if (-not $executable) { throw '没有找到 Windows 版 Manus Studio，请先安装桌面应用。' }
    $file = Get-Item -LiteralPath $executable -ErrorAction Stop
    $version = "$($file.VersionInfo.ProductVersion)".Trim()
    if (-not $version) { $version = "$($file.VersionInfo.FileVersion)".Trim() }
    if (-not $version) { $version = '未知版本' }
    [pscustomobject]@{ Executable = $file.FullName; InstallRoot = $file.DirectoryName; Version = $version }
}

function Select-Port([int]$Preferred) {
    for ($candidate = $Preferred; $candidate -le [Math]::Min(65535, $Preferred + 50); $candidate++) {
        $listener = $null
        try {
            $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $candidate)
            $listener.Start()
            return $candidate
        } catch {
        } finally {
            if ($listener) { $listener.Stop() }
        }
    }
    throw "从 $Preferred 开始的 51 个本地端口都不可用。"
}

try {
    $application = Get-ManusApplication
    if ($DetectOnly) {
        [pscustomobject]@{ ApplicationName = 'Manus Studio'; Executable = $application.Executable; Version = $application.Version } | ConvertTo-Json -Compress
        exit 0
    }
    Write-Step "检测到 Manus Studio $($application.Version)"
    $scriptsRoot = Split-Path -Parent $PSScriptRoot
    Write-Step '正在关闭已运行的 Manus Studio…'
    & (Join-Path $scriptsRoot 'quit-application.ps1') -ExecutablePath $application.Executable -ApplicationId 'manus' -ProtectedProcessId $PID
    $options = @{
        FilePath = $application.Executable
        WorkingDirectory = $application.InstallRoot
        ErrorAction = 'Stop'
    }
    if ($Normal) {
        Write-Step '正在正常启动 Manus Studio（恢复默认界面，不开放 CDP）…'
    } else {
        $Port = Select-Port $Port
        $options.ArgumentList = @('--remote-debugging-address=127.0.0.1', "--remote-debugging-port=$Port")
        Write-Step "正在启动 Manus Studio（CDP 端口 $Port）…"
    }
    # 用户通过 deck 启动的 Studio 主窗口需要正常显示。
    Start-Process @options | Out-Null
    if ($Normal) { Write-Step '完成。' }
    else { Write-Step '已启动 Manus Studio，等待 deck 连接主页面并应用界面设置。' }
    exit 0
} catch {
    Write-Host "[Manus Wide] 失败：$($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
