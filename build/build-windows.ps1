param(
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [string]$Version = '0.0.1'
)

$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$distPath = Join-Path $projectRoot 'dist'
$executablePath = Join-Path $distPath 'deck.exe'
$archivePath = Join-Path $distPath "deck-$Version-windows-amd64.zip"
$checksumPath = Join-Path $distPath "deck-$Version-windows-amd64.sha256"
$resourceBase = 'rsrc_deck_' + [Guid]::NewGuid().ToString('N')
$resourcePath = Join-Path $projectRoot ($resourceBase + '_windows_amd64.syso')
$previousModuleMode = $env:GO111MODULE
$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH

Push-Location $projectRoot
try {
    $env:GO111MODULE = 'on'
    New-Item -ItemType Directory -Path $distPath -Force | Out-Null

    # 资源生成器在当前主机运行，应用固定构建为 Windows amd64。
    & go run github.com/tc-hib/go-winres@v0.3.3 make `
        --in build/windows/winres.json --arch amd64 --out $resourceBase `
        --file-version $Version --product-version $Version
    if ($LASTEXITCODE -ne 0) {
        throw '生成 Windows 图标和版本资源失败'
    }

    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    & go build -tags production -trimpath `
        -ldflags "-s -w -X main.version=$Version -H windowsgui" `
        -o $executablePath .
    if ($LASTEXITCODE -ne 0) {
        throw '构建 Windows 可执行文件失败'
    }

    Compress-Archive -LiteralPath @($executablePath, (Join-Path $projectRoot 'README.md')) `
        -DestinationPath $archivePath -Force
    $checksums = foreach ($artifactPath in @($executablePath, $archivePath)) {
        $hash = (Get-FileHash -LiteralPath $artifactPath -Algorithm SHA256).Hash.ToLowerInvariant()
        $hash + '  ' + [System.IO.Path]::GetFileName($artifactPath)
    }
    [System.IO.File]::WriteAllText($checksumPath, ($checksums -join "`n") + "`n", [System.Text.UTF8Encoding]::new($false))

    Write-Output "可执行文件：$executablePath"
    Write-Output "发布包：$archivePath"
    Write-Output "校验文件：$checksumPath"
}
finally {
    if (Test-Path -LiteralPath $resourcePath) {
        Remove-Item -LiteralPath $resourcePath -Force
    }
    $env:GO111MODULE = $previousModuleMode
    $env:GOOS = $previousGoOS
    $env:GOARCH = $previousGoArch
    Pop-Location
}
