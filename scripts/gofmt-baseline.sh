#!/bin/sh
set -eu

gofmt_bin="${1:-gofmt}"
actual_file="$(mktemp)"
allowed_file="$(mktemp)"
trap 'rm -f "$actual_file" "$allowed_file"' EXIT

"$gofmt_bin" -l $(find cmd internal pkg -name '*.go' -type f) | sort >"$actual_file"
cat >"$allowed_file" <<'EOF'
internal/picoclaw/constants.go
pkg/newsfeed/cluster.go
pkg/newsfeed/sources/cailianshe.go
pkg/newsfeed/sources/juchao.go
pkg/newsfeed/sources/xueqiu.go
pkg/signal/cycle.go
pkg/signal/interpreter.go
pkg/signal/signal.go
pkg/tdx/protocol/kline.go
pkg/tdx/pull_test.go
EOF

unexpected="$(comm -23 "$actual_file" "$allowed_file")"
if [ -n "$unexpected" ]; then
	echo "New gofmt regressions:"
	echo "$unexpected"
	exit 1
fi

echo "gofmt baseline passed; no new formatting regressions"
