#!/bin/bash

# Development script with hot reload
# Usage: ./dev.sh       - start development servers
#        ./dev.sh stop  - stop development servers

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/.dev.pids"

cleanup() {
    echo ""
    echo "Stopping all processes..."
    kill $(jobs -p) 2>/dev/null || true
    if [ -f "$PID_FILE" ]; then
        kill $(cat "$PID_FILE") 2>/dev/null || true
        rm -f "$PID_FILE"
    fi
    exit 0
}

do_stop() {
    echo "Stopping development servers..."
    
    if [ -f "$PID_FILE" ]; then
        local pids
        pids=$(cat "$PID_FILE")
        echo "Stopping PIDs: $pids"
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                kill -TERM "$pid" 2>/dev/null || true
                sleep 1
                kill -9 "$pid" 2>/dev/null || true
            fi
        done
        rm -f "$PID_FILE"
    fi
    
    pids=$(lsof -ti:"8080" 2>/dev/null)
    if [ -n "$pids" ]; then
        echo "Killing port 8080: $pids"
        echo "$pids" | xargs kill -9 2>/dev/null || true
    fi
    
    pids=$(lsof -ti:"5173" 2>/dev/null)
    if [ -n "$pids" ]; then
        echo "Killing port 5173: $pids"
        echo "$pids" | xargs kill -9 2>/dev/null || true
    fi
    
    echo "All development servers stopped."
    exit 0
}

if [ "$1" = "stop" ]; then
    do_stop
fi

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

# Create a symlink from pkg/web/dist to web/dist for hot reload
mkdir -p pkg/web
if [ -L pkg/web/dist ]; then
    rm pkg/web/dist
fi
ln -s ../../web/dist pkg/web/dist
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

# Save PIDs to file
echo "$GO_PID $VITE_PID" > "$PID_FILE"

# Wait for any process to exit
wait -n

# If either exits, kill the other and exit
kill $GO_PID $VITE_PID 2>/dev/null || true
rm -f "$PID_FILE"
exit 1
