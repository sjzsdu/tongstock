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
$CLI_PATH codes --exchange bj | head -20
echo "... (仅显示前20条)"
echo ""

echo "6. 测试 kline 命令 - 日K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type day | head -10
echo "... (仅显示前10条)"
echo ""

echo "7. 测试 kline 命令 - 周K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type week | head -10
echo "... (仅显示前10条)"
echo ""

echo "8. 测试 kline 命令 - 月K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type month | head -10
echo "... (仅显示前10条)"
echo ""

echo "9. 测试 kline 命令 - 1分钟K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type 1m | head -10
echo "... (仅显示前10条)"
echo ""

echo "10. 测试 kline 命令 - 5分钟K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type 5m | head -10
echo "... (仅显示前10条)"
echo ""

echo "11. 测试 kline 命令 - 1分钟K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type 1m | head -10
echo "... (仅显示前10条)"
echo ""

echo "12. 测试 kline 命令 - 5分钟K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type 5m | head -10
echo "... (仅显示前10条)"
echo ""

echo "13. 测试 kline 命令 - 60分钟K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type 60m | head -10
echo "... (仅显示前10条)"
echo ""

echo "14. 测试 kline 命令 - 季K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type quarter | head -10
echo "... (仅显示前10条)"
echo ""

echo "15. 测试 kline 命令 - 年K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type year | head -10
echo "... (仅显示前10条)"
echo ""

echo "16. 测试 kline 命令 - 全部历史日K"
echo "----------------------------------------"
$CLI_PATH kline --code 000001 --type day --all | head -10
echo "... (仅显示前10条)"
echo ""

echo "17. 测试 minute 命令 - 当日分时"
echo "----------------------------------------"
$CLI_PATH minute 000001 | head -20
echo "... (仅显示前20条)"
echo ""

echo "18. 测试 minute 命令 - 历史分时"
echo "----------------------------------------"
$CLI_PATH minute 000001 --history --date 20250314 | head -20
echo "... (仅显示前20条)"
echo ""

echo "19. 测试 count 命令 - 深圳市场证券数量"
echo "----------------------------------------"
$CLI_PATH count
echo ""

echo "20. 测试 count 命令 - 上海市场证券数量"
echo "----------------------------------------"
$CLI_PATH count -e sh
echo ""

echo "21. 测试 count 命令 - 北京市场证券数量"
echo "----------------------------------------"
$CLI_PATH count -e bj
echo ""

echo "22. 测试 auction 命令 - 集合竞价"
echo "----------------------------------------"
$CLI_PATH auction 000001 | head -20
echo "... (仅显示前20条)"
echo ""

echo "23. 测试 trade 命令 - 当日分笔成交"
echo "----------------------------------------"
$CLI_PATH trade 000001 | head -20
echo "... (仅显示前20条)"
echo ""

echo "24. 测试 trade 命令 - 历史分笔成交"
echo "----------------------------------------"
$CLI_PATH trade 000001 --history --date 20250314 | head -20
echo "... (仅显示前20条)"
echo ""

echo "25. 测试 xdxr 命令 - 除权除息"
echo "----------------------------------------"
$CLI_PATH xdxr 000001 | head -20
echo "... (仅显示前20条)"
echo ""

echo "26. 测试 finance 命令 - 财务数据"
echo "----------------------------------------"
$CLI_PATH finance 000001
echo ""

echo "27. 测试 index 命令 - 指数日K"
echo "----------------------------------------"
$CLI_PATH index --code 999999 --type day | head -10
echo "... (仅显示前10条)"
echo ""

echo "28. 测试 index 命令 - 指数5分钟K"
echo "----------------------------------------"
$CLI_PATH index --code 399300 --type 5m | head -10
echo "... (仅显示前10条)"
echo ""

echo "29. 测试 company 命令 - 公司信息目录"
echo "----------------------------------------"
$CLI_PATH company 000001
echo ""

echo "30. 测试 block 命令 - 指数板块"
echo "----------------------------------------"
$CLI_PATH block --file block_zs.dat | head -20
echo "... (仅显示前20条)"
echo ""

echo "31. 测试 block 命令 - 行业板块"
echo "----------------------------------------"
$CLI_PATH block --file block_fg.dat | head -20
echo "... (仅显示前20条)"
echo ""

echo "32. 测试 block 命令 - 概念板块"
echo "----------------------------------------"
$CLI_PATH block --file block_gn.dat | head -20
echo "... (仅显示前20条)"
echo ""

echo "33. 测试 company-content 命令 - 基本用法"
echo "----------------------------------------"
$CLI_PATH company-content 000001 | head -50
echo "... (仅显示前50条)"
echo ""

echo "34. 测试 company-content 命令 - 通过块名称查询"
echo "----------------------------------------"
$CLI_PATH company-content 000001 --block "公司概况" | head -50
echo "... (仅显示前50条)"
echo ""

echo "35. 测试 company-content 命令 - 指定范围"
echo "----------------------------------------"
$CLI_PATH company-content 000001 --start 30744 --length 1000 | head -50
echo "... (仅显示前50条)"
echo ""

echo "36. 测试 indicator 命令 - 技术指标分析"
echo "----------------------------------------"
$CLI_PATH indicator --code 000001 --type day
echo ""

echo "37. 测试 indicator 命令 - 全部历史K线"
echo "----------------------------------------"
$CLI_PATH indicator --code 000001 --type day --all | head -30
echo "... (仅显示前30条)"
echo ""

echo "38. 测试 indicator 命令 - 60分钟K线"
echo "----------------------------------------"
$CLI_PATH indicator --code 000001 --type 60m | head -30
echo "... (仅显示前30条)"
echo ""

echo "39. 测试 screen 命令 - 批量筛选"
echo "----------------------------------------"
$CLI_PATH screen --codes "000001,600519,000858" --type day
echo ""

echo "40. 测试 screen 命令 - 筛选金叉信号"
echo "----------------------------------------"
$CLI_PATH screen --codes "000001,600519,000858" --type day --signal golden_cross
echo ""

echo "41. 测试 screen 命令 - 筛选超卖信号"
echo "----------------------------------------"
$CLI_PATH screen --codes "000001,600519,000858" --type day --signal oversold
echo ""

echo "========================================"
echo "测试完成"
echo "========================================"
