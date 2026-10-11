param(
    [switch]$DetectOnly,
    [string]$ExecutablePath
)

$ErrorActionPreference = 'Stop'

function Write-Step([string]$Message) {
    if ($env:WIDE_PROGRESS_STREAM -eq '1') { [Console]::WriteLine("[Claude] $Message"); return }
    Write-Host "[Claude] $Message" -ForegroundColor Cyan
}

function Get-ClaudePackages {
    @(Get-AppxPackage -Name 'Claude' -ErrorAction SilentlyContinue | Where-Object { $_.InstallLocation } | Sort-Object { [version]$_.Version } -Descending)
}

function Test-ClaudeExecutable([IO.FileInfo]$File) {
    # Claude Code 命令行同样名为 claude.exe，只接受桌面应用。
    $File.Name -ieq 'Claude.exe' -and "$($File.VersionInfo.ProductName)".Trim() -eq 'Claude'
}

function Get-ClaudeApplication {
    if ($ExecutablePath) {
        $file = Get-Item -LiteralPath $ExecutablePath -ErrorAction Stop
        if ($file.PSIsContainer -or -not (Test-ClaudeExecutable $file)) { throw '请选择 Claude 桌面应用的 Claude.exe。' }
    } else {
        $file = $null
        foreach ($package in Get-ClaudePackages) {
            $candidate = Join-Path $package.InstallLocation 'app\Claude.exe'
            if (Test-Path -LiteralPath $candidate -PathType Leaf) { $file = Get-Item -LiteralPath $candidate; break }
        }
        if (-not $file) { throw '没有找到 Windows 版 Claude 桌面应用，请先安装。' }
    }
    $version = "$($file.VersionInfo.ProductVersion)".Trim()
    if (-not $version) { $version = '未知版本' }
    [pscustomobject]@{ Executable = $file.FullName; InstallRoot = $file.DirectoryName; Version = $version }
}

function Get-ClaudeAppUserModelId([string]$Executable) {
    foreach ($package in Get-ClaudePackages) {
        $root = [IO.Path]::GetFullPath($package.InstallLocation).TrimEnd('\') + '\'
        if (-not $Executable.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) { continue }
        $relative = $Executable.Substring($root.Length)
        $application = @((Get-AppxPackageManifest -Package $package.PackageFullName -ErrorAction Stop).Package.Applications.Application) |
            Where-Object { "$($_.Executable)" -ieq $relative } | Select-Object -First 1
        if ($application -and $application.Id) { return "$($package.PackageFamilyName)!$($application.Id)" }
    }
    ''
}

try {
    $application = Get-ClaudeApplication
    if ($DetectOnly) {
        [pscustomobject]@{ ApplicationName = 'Claude'; Executable = $application.Executable; Version = $application.Version } | ConvertTo-Json -Compress
        exit 0
    }
    Write-Step "检测到 Claude $($application.Version)"
    $scriptsRoot = Split-Path -Parent $PSScriptRoot
    Write-Step '正在关闭已运行的 Claude…'
    & (Join-Path $scriptsRoot 'quit-application.ps1') -ExecutablePath $application.Executable -ApplicationId 'claude' -ProtectedProcessId $PID
    # Claude 检测到 --remote-debugging-port 会拒绝启动，因此只做普通启动。
    Write-Step '正在启动 Claude…'
    $appUserModelId = Get-ClaudeAppUserModelId $application.Executable
    if ($appUserModelId) {
        # 商店版通过应用包入口启动，与开始菜单启动方式一致。
        Start-Process -FilePath "shell:AppsFolder\$appUserModelId" -ErrorAction Stop | Out-Null
    } else {
        Start-Process -FilePath $application.Executable -WorkingDirectory $application.InstallRoot -ErrorAction Stop | Out-Null
    }
    Write-Step '已启动 Claude，字体需在开发者工具中执行 deck 复制的脚本。'
    exit 0
} catch {
    Write-Host "[Claude] 失败：$($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
