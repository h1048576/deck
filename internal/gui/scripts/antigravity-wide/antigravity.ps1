param(
    [switch]$DetectOnly,
    [switch]$DebugPortOnly,
    [string]$ExecutablePath,
    [ValidateRange(1024, 65535)]
    [int]$Port = 9341,
    [switch]$Normal
)

$ErrorActionPreference = 'Stop'

function Get-AntigravityApplication {
    if ($ExecutablePath) {
        $candidates = @($ExecutablePath)
    } else {
        $candidates = @(
            foreach ($root in @("$env:LOCALAPPDATA/Programs", $env:ProgramFiles, ${env:ProgramFiles(x86)})) {
                if ($root) { Join-Path $root 'antigravity/Antigravity.exe' }
            }
            Get-CimInstance Win32_Process -Filter "Name = 'Antigravity.exe'" -ErrorAction SilentlyContinue |
                Select-Object -ExpandProperty ExecutablePath
            foreach ($key in @('HKCU:/Software/Microsoft/Windows/CurrentVersion/Uninstall/*', 'HKLM:/Software/Microsoft/Windows/CurrentVersion/Uninstall/*', 'HKLM:/Software/WOW6432Node/Microsoft/Windows/CurrentVersion/Uninstall/*')) {
                Get-ItemProperty -Path $key -ErrorAction SilentlyContinue |
                    Where-Object { $_.DisplayName -eq 'Antigravity' -and $_.InstallLocation } |
                    ForEach-Object { Join-Path $_.InstallLocation 'Antigravity.exe' }
            }
        )
    }
    foreach ($candidate in $candidates) {
        if (-not $candidate) { continue }
        $file = Get-Item -LiteralPath $candidate -ErrorAction SilentlyContinue
        if (-not $file -or $file.PSIsContainer -or $file.Name -ne 'Antigravity.exe') { continue }
        # IDE 与桌面应用可能同名。独立桌面版同时提供 Electron 归档和独立语言服务。
        if ($file.VersionInfo.ProductName -ne 'Antigravity' -or $file.VersionInfo.CompanyName -ne 'Google' -or
            -not (Test-Path -LiteralPath (Join-Path $file.DirectoryName 'resources/app.asar') -PathType Leaf) -or
            -not (Test-Path -LiteralPath (Join-Path $file.DirectoryName 'resources/bin/language_server.exe') -PathType Leaf)) { continue }
        return [pscustomobject]@{
            ApplicationName = 'Antigravity'
            Executable = $file.FullName
            Version = $file.VersionInfo.FileVersion
        }
    }
    throw '没有找到 Antigravity Desktop，请选择独立桌面版的 Antigravity.exe。'
}

function Select-Port([int]$Preferred) {
    for ($candidate = $Preferred; $candidate -le [Math]::Min(65535, $Preferred + 50); $candidate++) {
        $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $candidate)
        try {
            $listener.Start()
            return $candidate
        } catch [Net.Sockets.SocketException] {
        } finally {
            $listener.Stop()
        }
    }
    throw "从 $Preferred 开始的本机调试端口均不可用。"
}

function Get-AntigravityDebugPort([string]$Executable) {
    foreach ($info in (Get-CimInstance Win32_Process -Filter "Name = 'Antigravity.exe'" -ErrorAction Stop)) {
        if ($info.ExecutablePath -ne $Executable -or $info.CommandLine -notmatch '(?:^|\s)--remote-debugging-port=(\d{1,5})(?:\s|$)') { continue }
        $candidate = [int]$Matches[1]
        if ($candidate -lt 1024 -or $candidate -gt 65535) { continue }
        if (Get-NetTCPConnection -LocalAddress '127.0.0.1' -LocalPort $candidate -State Listen -OwningProcess $info.ProcessId -ErrorAction SilentlyContinue) {
            return $candidate
        }
    }
}

try {
    $app = Get-AntigravityApplication
    if ($DetectOnly) {
        $app | ConvertTo-Json -Compress
        exit 0
    }
    if ($DebugPortOnly) {
        Get-AntigravityDebugPort $app.Executable
        exit 0
    }
    if (-not $Normal) { $Port = Select-Port $Port }
    $quit = Join-Path (Split-Path -Parent $PSScriptRoot) 'quit-application.ps1'
    & $quit -ExecutablePath $app.Executable -ApplicationId 'antigravity' -ProtectedProcessId $PID
    $options = @{
        FilePath = $app.Executable
        WorkingDirectory = Split-Path -Parent $app.Executable
        ErrorAction = 'Stop'
    }
    if ($Normal) {
        Start-Process @options | Out-Null
        exit 0
    }

    $options.ArgumentList = @('--remote-debugging-address=127.0.0.1', "--remote-debugging-port=$Port")
    [Console]::WriteLine("正在启动 Antigravity（CDP 端口 $Port）…")
    $process = Start-Process @options -PassThru
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        if ($process.HasExited) { throw 'Antigravity 启动进程已退出，请确认没有其他方式启动的实例。' }
        $endpoint = Get-NetTCPConnection -LocalAddress '127.0.0.1' -LocalPort $Port -State Listen -OwningProcess $process.Id -ErrorAction SilentlyContinue
        if ($endpoint) { exit 0 }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Antigravity 未在本机开启调试端口 $Port。"
} catch {
    [Console]::Error.WriteLine("Antigravity 失败：$($_.Exception.Message)")
    exit 1
}
