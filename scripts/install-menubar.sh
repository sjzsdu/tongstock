#!/bin/bash
set -e

echo "=== TongStock Menu Bar 安装 ==="

# 1. Build
echo "编译中..."
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/tongstock-menubar" ./cmd/menubar
go build -o "$HOME/.local/bin/tongstock-server" ./cmd/server
echo "编译完成:"
echo "  ~/.local/bin/tongstock-menubar"
echo "  ~/.local/bin/tongstock-server"

# 2. Create plist
mkdir -p "$HOME/Library/LaunchAgents"
cat > "$HOME/Library/LaunchAgents/com.tongstock.menubar.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.tongstock.menubar</string>
    <key>ProgramArguments</key>
    <array>
        <string>${HOME}/.local/bin/tongstock-menubar</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>${HOME}/.tongstock/menubar.log</string>
    <key>StandardErrorPath</key>
    <string>${HOME}/.tongstock/menubar.log</string>
</dict>
</plist>
EOF
echo "plist 已写入: ~/Library/LaunchAgents/com.tongstock.menubar.plist"

# 3. Create log dir
mkdir -p "$HOME/.tongstock"

# 4. Load service
launchctl unload "$HOME/Library/LaunchAgents/com.tongstock.menubar.plist" 2>/dev/null || true
launchctl load "$HOME/Library/LaunchAgents/com.tongstock.menubar.plist"
echo "服务已启动"

echo ""
echo "=== 安装完成 ==="
echo "菜单栏应用将在每次登录时自动启动"
echo "手动管理: launchctl start/stop com.tongstock.menubar"
