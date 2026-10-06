# wide-pure 的构建脚本。所有路径相对本文件所在目录（go.mod 旁），
# 用 make -C 指到本目录即可，别的目录不会误碰。
# 版本取自 git 标签（没打标签时退回 dev），注入 main.version。
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null | tr -cd 'A-Za-z0-9._+-'),dev)
LDFLAGS  = -s -w -X main.version=$(VERSION)

# WINVER 去掉非数字前后缀，喂给 go-winres 的文件/产品版本。
WINVER = $(shell echo $(VERSION) | sed -E 's/^v//; s/[^0-9.].*//')

.PHONY: build serve release-windows clean test

# 默认构建：桌面 GUI，不弹出控制台窗口。
build:
	go build -tags production -trimpath -ldflags="$(LDFLAGS) -H windowsgui" -o wide-pure.exe .

# 浏览器预览形态（无 Wails 窗口），用 `wide-pure-serve serve` 起在 127.0.0.1:3420。
serve:
	go build -tags nogui -trimpath -ldflags="$(LDFLAGS)" -o wide-pure-serve.exe .

# Windows 发布版：GUI 程序（-H windowsgui 无控制台窗口），嵌入
# build/windows 里的图标、版本信息与 DPI 感知清单（go-winres）。
release-windows:
	mkdir -p dist
	go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --arch amd64 --out rsrc \
		--file-version "$(or $(WINVER),0.1.0)" --product-version "$(or $(WINVER),0.1.0)"
	go build -tags production -trimpath -ldflags="$(LDFLAGS) -H windowsgui" -o dist/wide-pure-windows-amd64.exe .
	rm -f rsrc_windows_amd64.syso

# 单元测试（jsonc / widecfg / settings）。
test:
	go test ./...

clean:
	rm -rf wide-pure.exe wide-pure-serve.exe dist rsrc_windows_*.syso
