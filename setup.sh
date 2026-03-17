#!/bin/bash

# TongStock 构建脚本
# 用于编译 CLI 和 Server 程序

set -e

echo "========================================"
echo "TongStock 构建脚本"
echo "========================================"
echo ""

# 检查 Go 版本
echo "检查 Go 版本..."
if ! command -v go &> /dev/null; then
    echo "错误: 未找到 Go，请先安装 Go 1.25+"
    echo "下载地址: https://go.dev/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo "当前 Go 版本: $GO_VERSION"
echo ""

# 下载依赖
echo "下载依赖..."
go mod download
echo "依赖下载完成"
echo ""

# 编译 CLI
echo "编译 CLI..."
go build -o tongstock-cli ./cmd/cli
if [ $? -eq 0 ]; then
    echo "✓ CLI 编译成功: tongstock-cli"
else
    echo "✗ CLI 编译失败"
    exit 1
fi
echo ""

# 编译 Server
echo "编译 Server..."
go build -o tongstock-server ./cmd/server
if [ $? -eq 0 ]; then
    echo "✓ Server 编译成功: tongstock-server"
else
    echo "✗ Server 编译失败"
    exit 1
fi
echo ""

# 检查并创建 ~/.local/bin 目录
echo "检查 ~/.local/bin 目录..."
if [ ! -d "$HOME/.local/bin" ]; then
    mkdir -p "$HOME/.local/bin"
    echo "已创建 ~/.local/bin 目录"
else
    echo "~/.local/bin 目录已存在"
fi
echo ""

# 检查 ~/.local/bin 是否在 PATH 中
if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    echo "添加 ~/.local/bin 到 PATH..."
    export PATH="$HOME/.local/bin:$PATH"
    echo "已添加 PATH"
else
    echo "~/.local/bin 已在 PATH 中"
fi
echo ""

# 检查编译结果
if [ -f "tongstock-cli" ] && [ -f "tongstock-server" ]; then
    echo "移动文件到 ~/.local/bin 目录..."
    mv tongstock-cli "$HOME/.local/bin/"
    mv tongstock-server "$HOME/.local/bin/"
    chmod +x "$HOME/.local/bin/tongstock-cli" "$HOME/.local/bin/tongstock-server"
    echo "文件已移动到 ~/.local/bin 目录"
    echo ""
    echo "========================================"
    echo "构建完成！"
    echo "========================================"
    echo ""
    echo "生成的文件:"
    ls -lh "$HOME/.local/bin/tongstock-cli" "$HOME/.local/bin/tongstock-server"
    echo ""
    echo "使用方法:"
    echo "  tongstock-cli quote 000001"
    echo "  tongstock-server"
    echo ""
    echo "提示: 如需永久生效，可将以下内容添加到 ~/.zshrc:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
else
    echo "========================================"
    echo "构建失败"
    echo "========================================"
    exit 1
fi
