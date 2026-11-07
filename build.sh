#!/usr/bin/env bash
# deck 的 Windows 发布构建脚本，与 build/build-windows.ps1 的编译步骤一致，
# 但只产出可执行文件，不打包 zip、不生成校验值。
# 适用于 Git Bash / WSL / Linux 下交叉编译。
#
# 产物：dist/deck.exe

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
EOF
}

fail() {
    echo "错误：$*" >&2
    exit 1
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

EXE_PATH="$ROOT_DIR/dist/deck.exe"

# Windows 下正在运行的 deck.exe 会锁定输出文件，导致 go build 失败，
# 因此构建前先结束正在运行的实例。

# 成功输出命令名并返回 0；非 Windows 环境返回 1。
windows_process_cmd() {
    local name="$1"
    if command -v "$name.exe" >/dev/null 2>&1; then
        echo "$name.exe"
        return 0
    fi
    if command -v "$name" >/dev/null 2>&1; then
        echo "$name"
        return 0
    fi
    return 1
}

deck_is_running() {
    local tasklist_cmd output
    tasklist_cmd="$(windows_process_cmd tasklist)" || return 1

    output="$(MSYS_NO_PATHCONV=1 "$tasklist_cmd" /FI "IMAGENAME eq deck.exe" /NH 2>/dev/null || true)"
    # tasklist 的提示信息随系统语言变化，统一转小写后只匹配映像名。
    output="$(printf '%s' "$output" | tr '[:upper:]' '[:lower:]')"
    case "$output" in
        *deck.exe*) return 0 ;;
        *) return 1 ;;
    esac
}

kill_running_deck() {
    local taskkill_cmd i
    taskkill_cmd="$(windows_process_cmd taskkill)" || return 0

    deck_is_running || return 0

    echo "==> 检测到 deck.exe 正在运行，先结束进程"
    # taskkill 可能因进程刚好自行退出而返回非零，此时不算失败。
    if ! MSYS_NO_PATHCONV=1 "$taskkill_cmd" /F /IM deck.exe >/dev/null 2>&1; then
        deck_is_running || return 0
        fail "结束 deck.exe 失败，请手动关闭后重试"
    fi

    # 等待 Windows 释放可执行文件的句柄
    for ((i = 0; i < 20; i++)); do
        deck_is_running || return 0
        sleep 0.5
    done
    fail "deck.exe 仍在运行，请手动关闭后重试"
}

kill_running_deck

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

echo
echo "构建完成："
ls -lh "$EXE_PATH"
