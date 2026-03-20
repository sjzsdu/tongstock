#!/bin/bash

# TongStock Server API 测试脚本
# 用于验证所有 HTTP API 是否正常工作

SERVER="tongstock-server"
URL="http://localhost:8080"

if [ ! -f "$SERVER" ] && [ ! -f "$HOME/.local/bin/$SERVER" ]; then
    echo "错误: 未找到 tongstock-server，请先编译"
    exit 1
fi

if [ -f "$SERVER" ]; then
    SERVER_PATH="./$SERVER"
else
    SERVER_PATH="$HOME/.local/bin/$SERVER"
fi

echo "========================================"
echo "TongStock Server API 测试"
echo "========================================"
echo ""

$SERVER_PATH &
SERVER_PID=$!
sleep 2

trap "kill $SERVER_PID 2>/dev/null" EXIT

test_api() {
    local name="$1"
    local url="$2"
    local result=$(curl -s -o /dev/null -w "%{http_code}" "$url")
    if [ "$result" = "200" ]; then
        echo "✓ $name"
    else
        echo "✗ $name (HTTP $result)"
    fi
}

echo "测试接口..."
test_api "health" "$URL/health"
test_api "quote" "$URL/api/quote?code=000001"
test_api "codes:sz" "$URL/api/codes?exchange=sz"
test_api "codes:sh" "$URL/api/codes?exchange=sh"
test_api "codes:bj" "$URL/api/codes?exchange=bj"
test_api "kline:day" "$URL/api/kline?code=000001&type=day"
test_api "kline:week" "$URL/api/kline?code=000001&type=week"
test_api "kline:month" "$URL/api/kline?code=000001&type=month"
test_api "kline:quarter" "$URL/api/kline?code=000001&type=quarter"
test_api "kline:year" "$URL/api/kline?code=000001&type=year"
test_api "kline:1m" "$URL/api/kline?code=000001&type=1m"
test_api "kline:5m" "$URL/api/kline?code=000001&type=5m"
test_api "kline:60m" "$URL/api/kline?code=000001&type=60m"
test_api "minute" "$URL/api/minute?code=000001"
test_api "minute:history" "$URL/api/minute?code=000001&history=true&date=20250314"
test_api "count:sz" "$URL/api/count?exchange=sz"
test_api "count:sh" "$URL/api/count?exchange=sh"
test_api "count:bj" "$URL/api/count?exchange=bj"
test_api "auction" "$URL/api/auction?code=000001"
test_api "trade" "$URL/api/trade?code=000001"
test_api "trade:history" "$URL/api/trade?code=000001&history=true&date=20250314"
test_api "xdxr" "$URL/api/xdxr?code=000001"
test_api "finance" "$URL/api/finance?code=000001"
test_api "index:day" "$URL/api/index?code=999999&type=day"
test_api "index:5m" "$URL/api/index?code=399300&type=5m"
test_api "company" "$URL/api/company?code=000001"
test_api "company:content" "$URL/api/company/content?code=000001&filename=000001.txt"
test_api "block:zs" "$URL/api/block?file=block_zs.dat"
test_api "block:fg" "$URL/api/block?file=block_fg.dat"
test_api "block:gn" "$URL/api/block?file=block_gn.dat"

echo ""
echo "测试完成"
