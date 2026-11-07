#!/usr/bin/env bash
# deck 的 Windows 发布打包脚本，行为与 build/build-windows.ps1 一致。
# 适用于 Git Bash / WSL / Linux 下交叉编译；压缩优先用 zip，
# 没有 zip 时回退到 Windows 自带的 PowerShell Compress-Archive。
#
# 产物：dist/deck.exe、dist/deck-<版本>-windows-amd64.zip 及同名 .sha256

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

DEFAULT_VERSION="0.0.1"
VERSION="$DEFAULT_VERSION"

usage() {
    cat <<'EOF'
用法: ./build.sh [选项] [版本号]

选项:
  -v, --version <版本>  指定版本号（形如 0.1.0），默认 0.0.1
  -h, --help            显示本帮助

示例:
  ./build.sh
  ./build.sh 0.1.0
  ./build.sh --version 0.1.0

产物:
  dist/deck.exe
  dist/deck-<版本>-windows-amd64.zip
  dist/deck-<版本>-windows-amd64.sha256
EOF
}

fail() {
    echo "错误：$*" >&2
    exit 1
}

# 把 POSIX 路径转成 Windows 路径，供 PowerShell 使用；非 MSYS 环境原样返回。
to_win() {
    if command -v cygpath >/dev/null 2>&1; then
        cygpath -w "$1"
    else
        printf '%s' "$1"
    fi
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print tolower($1) }'
    else
        powershell.exe -NoProfile -ExecutionPolicy Bypass -Command \
            "(Get-FileHash -LiteralPath '$(to_win "$1")' -Algorithm SHA256).Hash.ToLowerInvariant()" \
            | tr -d '\r\n'
    fi
}

# make_zip <输出 zip> <文件...>，文件路径相对项目根目录，压缩后平铺存放。
make_zip() {
    local archive="$1"
    shift
    local -a files=("$@")
    rm -f "$archive"

    if command -v zip >/dev/null 2>&1; then
        ( cd "$ROOT_DIR" && zip -q -j "$archive" "${files[@]}" )
        return 0
    fi

    if command -v powershell.exe >/dev/null 2>&1; then
        local -a win_files=()
        local file
        for file in "${files[@]}"; do
            win_files+=("'$(to_win "$ROOT_DIR/$file")'")
        done
        local joined
        joined=$(IFS=,; printf '%s' "${win_files[*]}")
        powershell.exe -NoProfile -ExecutionPolicy Bypass -Command \
            "Compress-Archive -LiteralPath @(${joined}) -DestinationPath '$(to_win "$ROOT_DIR/$archive")' -Force"
        return 0
    fi

    fail "找不到可用的压缩工具（zip 或 powershell.exe）"
}

while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        -v|--version)
            [ $# -ge 2 ] || fail "选项 $1 缺少版本号"
            VERSION="$2"
            shift 2
            ;;
        -*)
            fail "未知选项 $1（使用 --help 查看用法）"
            ;;
        *)
            VERSION="$1"
            shift
            ;;
    esac
done

if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    fail "版本号必须形如 0.0.1，当前为 $VERSION"
fi

command -v go >/dev/null 2>&1 || fail "未找到 go 命令，请先安装 Go"

ARCHIVE_NAME="deck-${VERSION}-windows-amd64.zip"
CHECKSUM_NAME="deck-${VERSION}-windows-amd64.sha256"
EXE_PATH="$ROOT_DIR/dist/deck.exe"
ARCHIVE_PATH="$ROOT_DIR/dist/$ARCHIVE_NAME"
CHECKSUM_PATH="$ROOT_DIR/dist/$CHECKSUM_NAME"

# go-winres 会生成 <前缀>_windows_amd64.syso，构建结束后清理。
RESOURCE_BASE="rsrc_deck_$$_${RANDOM}"
RESOURCE_PATH="$ROOT_DIR/${RESOURCE_BASE}_windows_amd64.syso"
cleanup() {
    rm -f "$RESOURCE_PATH"
}
trap cleanup EXIT

mkdir -p "$ROOT_DIR/dist"

echo "==> 生成 Windows 资源（版本 $VERSION）"
GO111MODULE=on go run github.com/tc-hib/go-winres@v0.3.3 make \
    --in build/windows/winres.json \
    --arch amd64 \
    --out "$RESOURCE_BASE" \
    --file-version "$VERSION" \
    --product-version "$VERSION"

echo "==> 构建 deck.exe"
GO111MODULE=on GOOS=windows GOARCH=amd64 go build \
    -tags production \
    -trimpath \
    -ldflags "-s -w -X main.version=$VERSION -H windowsgui" \
    -o dist/deck.exe .

echo "==> 打包发布包"
make_zip "dist/$ARCHIVE_NAME" "dist/deck.exe" "README.md"

echo "==> 计算校验值"
{
    printf '%s  %s\n' "$(sha256_of "$EXE_PATH")" "deck.exe"
    printf '%s  %s\n' "$(sha256_of "$ARCHIVE_PATH")" "$ARCHIVE_NAME"
} > "$CHECKSUM_PATH"

echo
echo "打包完成："
ls -lh "$EXE_PATH" "$ARCHIVE_PATH" "$CHECKSUM_PATH"
