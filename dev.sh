#!/bin/bash

# Development script with hot reload
# Usage: ./dev.sh

set -e

cleanup() {
    echo ""
    echo "Stopping all processes..."
    kill $(jobs -p) 2>/dev/null || true
    exit 0
}

trap cleanup SIGINT SIGTERM

kill_port() {
    local pids
    pids=$(lsof -ti:"$1" 2>/dev/null)
    if [ -n "$pids" ]; then
        echo "Port $1 occupied (PID: $pids), killing..."
        echo "$pids" | xargs kill -9 2>/dev/null || true
        sleep 0.5
    fi
}

kill_port 8080
kill_port 5173
if ! command -v air &> /dev/null; then
    echo "Installing air for Go hot reload..."
    go install github.com/air-verse/air@latest
fi

# Check if node modules exist
if [ ! -d "web/node_modules" ]; then
    echo "Installing frontend dependencies..."
    cd web && pnpm install && cd ..
fi

echo "Starting development servers..."
echo ""
echo "  Go API Server:  http://localhost:8080"
echo "  Vite Dev Server: http://localhost:5173"
echo ""
echo "  Open http://localhost:5173 for frontend development"
echo "  Frontend changes: instant hot reload"
echo "  Go changes: automatic rebuild via air"
echo ""
echo "Press Ctrl+C to stop"
echo ""

# Start Go server with air (hot reload)
cd "$(dirname "$0")"
air -c .air.toml &
GO_PID=$!

# Start Vite dev server
cd web
pnpm dev &
VITE_PID=$!

cd ..

# Wait for any process to exit
wait -n

# If either exits, kill the other and exit
kill $GO_PID $VITE_PID 2>/dev/null || true
exit 1
