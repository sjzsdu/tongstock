#!/bin/bash

# TongStock 安装脚本
# 编译 CLI + Server（含 Web UI），安装到 ~/.local/bin

set -e

echo "========================================"
echo "TongStock 安装脚本"
echo "========================================"
echo ""

for cmd in go pnpm; do
    if ! command -v $cmd &> /dev/null; then
        echo "错误: 未找到 $cmd"
        case $cmd in
            go) echo "下载地址: https://go.dev/dl/" ;;
            pnpm) echo "安装: npm install -g pnpm" ;;
        esac
        exit 1
    fi
done

GO_VERSION=$(go version | awk '{print $3}')
echo "Go 版本: $GO_VERSION"
echo "pnpm 版本: $(pnpm -v)"
echo ""

echo "构建中..."
make all
echo ""

mkdir -p "$HOME/.local/bin"
cp tongstock-cli tongstock-server "$HOME/.local/bin/"
chmod +x "$HOME/.local/bin/tongstock-cli" "$HOME/.local/bin/tongstock-server"

echo "========================================"
echo "安装完成！"
echo "========================================"
echo ""
ls -lh "$HOME/.local/bin/tongstock-cli" "$HOME/.local/bin/tongstock-server"
echo ""
echo "使用方法:"
echo "  tongstock-cli quote 000001"
echo "  tongstock-server"
echo "  浏览器访问 http://localhost:8080"
echo ""
echo "提示: 如需永久生效，将以下内容添加到 ~/.zshrc:"
echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
