#!/bin/bash

# TongStock CLI 测试脚本
# 用于验证所有 CLI 命令的运行结果

CLI="tongstock-cli"
SERVER="tongstock-server"

# 检查 CLI 是否存在（先检查当前目录，再检查 ~/.local/bin）
if [ ! -f "$CLI" ] && [ ! -f "$HOME/.local/bin/$CLI" ]; then
    echo "错误: 未找到 tongstock-cli，请先编译"
    echo "运行: bash setup.sh"
    exit 1
fi

# 确定 CLI 路径
if [ -f "$CLI" ]; then
    CLI_PATH="./$CLI"
else
    CLI_PATH="$HOME/.local/bin/$CLI"
fi

echo "========================================"
echo "TongStock CLI 测试脚本"
echo "========================================"
echo ""
echo "使用 CLI: $CLI_PATH"
echo ""

echo "1. 测试 quote 命令 - 查询股票行情"
echo "----------------------------------------"
$CLI_PATH quote 000001
echo ""

echo "2. 测试 quote 命令 - 查询多个股票"
echo "----------------------------------------"
$CLI_PATH quote 000001 600000
echo ""

echo "3. 测试 codes 命令 - 深圳市场"
echo "----------------------------------------"
$CLI_PATH codes | head -20
echo "... (仅显示前20条)"
echo ""

echo "4. 测试 codes 命令 - 上海市场"
echo "----------------------------------------"
$CLI_PATH codes --exchange sh | head -20
echo "... (仅显示前20条)"
echo ""

echo "5. 测试 codes 命令 - 北京市场"
echo "----------------------------------------"
$CLI codes --exchange bj | head -20
echo "... (仅显示前20条)"
echo ""

echo "6. 测试 kline 命令 - 日K"
echo "----------------------------------------"
$CLI kline --code 000001 --type day | head -10
echo "... (仅显示前10条)"
echo ""

echo "7. 测试 kline 命令 - 周K"
echo "----------------------------------------"
$CLI kline --code 000001 --type week | head -10
echo "... (仅显示前10条)"
echo ""

echo "8. 测试 kline 命令 - 月K"
echo "----------------------------------------"
$CLI kline --code 000001 --type month | head -10
echo "... (仅显示前10条)"
echo ""

echo "9. 测试 kline 命令 - 1分钟K"
echo "----------------------------------------"
$CLI kline --code 000001 --type 1m | head -10
echo "... (仅显示前10条)"
echo ""

echo "10. 测试 kline 命令 - 5分钟K"
echo "----------------------------------------"
$CLI kline --code 000001 --type 5m | head -10
echo "... (仅显示前10条)"
echo ""

echo "11. 测试 kline 命令 - 1分钟K"
echo "----------------------------------------"
$CLI kline --code 000001 --type 1m | head -10
echo "... (仅显示前10条)"
echo ""

echo "12. 测试 kline 命令 - 5分钟K"
echo "----------------------------------------"
$CLI kline --code 000001 --type 5m | head -10
echo "... (仅显示前10条)"
echo ""

echo "13. 测试 kline 命令 - 60分钟K"
echo "----------------------------------------"
$CLI kline --code 000001 --type 60m | head -10
echo "... (仅显示前10条)"
echo ""

echo "14. 测试 kline 命令 - 季K"
echo "----------------------------------------"
$CLI kline --code 000001 --type quarter | head -10
echo "... (仅显示前10条)"
echo ""

echo "15. 测试 kline 命令 - 年K"
echo "----------------------------------------"
$CLI kline --code 000001 --type year | head -10
echo "... (仅显示前10条)"
echo ""

echo "16. 测试 kline 命令 - 全部历史日K"
echo "----------------------------------------"
$CLI kline --code 000001 --type day --all | head -10
echo "... (仅显示前10条)"
echo ""

echo "17. 测试 minute 命令 - 当日分时"
echo "----------------------------------------"
$CLI minute 000001 | head -20
echo "... (仅显示前20条)"
echo ""

echo "18. 测试 trade 命令 - 当日分笔成交"
echo "----------------------------------------"
$CLI trade 000001 | head -20
echo "... (仅显示前20条)"
echo ""

echo "19. 测试 trade 命令 - 历史分笔成交"
echo "----------------------------------------"
$CLI trade 000001 --history --date 20250314 | head -20
echo "... (仅显示前20条)"
echo ""

echo "========================================"
echo "测试完成"
echo "========================================"
