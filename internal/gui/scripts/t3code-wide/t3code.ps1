param(
    [switch]$DetectOnly,
    [string]$ExecutablePath,
    [ValidatePattern('^(?:auto|fit-content|(?:0|[1-9][0-9]{0,3})(?:\.[0-9]+)?(?:px|rem|em|vw|vh|%))$')]
    [string]$Width = '70vw',
    [ValidatePattern('^(?:none|(?:0|[1-9][0-9]{0,3})(?:\.[0-9]+)?(?:px|rem|em|vw|vh|%))$')]
    [string]$MaxWidth = '90rem',
    [ValidateRange(8, 72)]
    [int]$FontSize = 18,
    [ValidateRange(100, 1000)]
    [int]$FontWeight = 300,
    [ValidateScript({ $_ -and $_ -notmatch '[;{}<>\r\n]' })]
    [string]$FontFamily = 'Maple Mono NF CN',
    [ValidateRange(1024, 65535)]
    [int]$Port = 9340,
    [switch]$Normal
)

$ErrorActionPreference = 'Stop'

function Write-Step([string]$Message) {
    if ($env:WIDE_PROGRESS_STREAM -eq '1') { [Console]::WriteLine("[T3Code Wide] $Message"); return }
    Write-Host "[T3Code Wide] $Message" -ForegroundColor Cyan
}

function Get-T3CodeApplication {
    if ($ExecutablePath) {
        $file = Get-Item -LiteralPath $ExecutablePath -ErrorAction Stop
        if ($file.PSIsContainer -or $file.Name -notin @('T3 Code (Beta).exe', 'T3 Code.exe', 'T3Code.exe')) { throw '请选择 T3Code 的可执行文件。' }
        return [pscustomobject]@{ Executable = $file.FullName; InstallRoot = $file.DirectoryName; Version = "$($file.VersionInfo.ProductVersion)" }
    }

    $names = @('T3 Code (Beta).exe', 'T3 Code.exe', 'T3Code.exe')
    $candidates = @(
        foreach ($root in @("$env:LOCALAPPDATA/Programs", $env:ProgramFiles)) {
            foreach ($name in $names) { Join-Path $root "t3code/$name" }
        }
        foreach ($name in $names) {
            Get-CimInstance Win32_Process -Filter "Name = '$name'" -ErrorAction SilentlyContinue | Select-Object -ExpandProperty ExecutablePath
        }
    )
    $executable = $candidates | Where-Object { $_ -and (Test-Path -LiteralPath $_ -PathType Leaf) } | Select-Object -First 1
    if (-not $executable) { throw '没有找到 Windows 版 T3Code，请先安装桌面应用。' }
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
    $application = Get-T3CodeApplication
    if ($DetectOnly) {
        [pscustomobject]@{ ApplicationName = 'T3Code'; Executable = $application.Executable; Version = $application.Version } | ConvertTo-Json -Compress
        exit 0
    }
    Write-Step "检测到 T3Code $($application.Version)"
    $scriptsRoot = Split-Path -Parent $PSScriptRoot
    Write-Step '正在关闭已运行的 T3Code…'
    & (Join-Path $scriptsRoot 'quit-application.ps1') -ExecutablePath $application.Executable -ApplicationId 't3code' -ProtectedProcessId $PID
    $options = @{
        FilePath = $application.Executable
        WorkingDirectory = $application.InstallRoot
        ErrorAction = 'Stop'
    }
    if ($Normal) {
        Write-Step '正在正常启动 T3Code（恢复默认界面，不开放 CDP）…'
    } else {
        $Port = Select-Port $Port
        $options.ArgumentList = @('--remote-debugging-address=127.0.0.1', "--remote-debugging-port=$Port")
        Write-Step "正在启动 T3Code（CDP 端口 $Port）…"
    }
    # 用户通过 deck 启动的 Studio 主窗口需要正常显示。
    Start-Process @options | Out-Null
    if ($Normal) { Write-Step '完成。' }
    else { Write-Step '已启动 T3Code，等待 deck 连接主页面并应用界面设置。' }
    exit 0
} catch {
    Write-Host "[T3Code Wide] 失败：$($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
