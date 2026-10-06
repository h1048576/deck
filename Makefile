# deck 的构建脚本。所有路径相对本文件所在目录（go.mod 旁），
# 用 make -C 指到本目录即可，别的目录不会误碰。
# 默认版本为 0.0.1，可用 make VERSION=x.y.z 覆盖。
VERSION ?= 0.0.1
LDFLAGS  = -s -w -X main.version=$(VERSION)

export GO111MODULE := on

.PHONY: build serve release-windows clean test

# 默认构建：桌面 GUI，不弹出控制台窗口。
build:
	go build -tags production -trimpath -ldflags="$(LDFLAGS) -H windowsgui" -o deck.exe .

# 浏览器预览形态（无 Wails 窗口），用 `deck-serve serve` 起在 127.0.0.1:3420。
serve:
	go build -tags nogui -trimpath -ldflags="$(LDFLAGS)" -o deck-serve.exe .

# Windows 发布版：GUI 程序（-H windowsgui 无控制台窗口），嵌入
# build/windows 里的图标、版本信息与 DPI 感知清单（go-winres）。
release-windows:
	powershell.exe -NoProfile -ExecutionPolicy Bypass -File build/build-windows.ps1 -Version "$(VERSION)"

# 单元测试（jsonc / harnesscfg / settings）。
test:
	go test ./...

clean:
	rm -rf deck.exe deck-serve.exe dist rsrc_windows_*.syso
