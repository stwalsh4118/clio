#!/bin/bash
# Wrapper script for Air hot reload
# This script handles graceful stop/start of the daemon during development

set -e

BINARY="./tmp/clio"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Function to cleanup on exit
cleanup() {
    if [ -f "$BINARY" ]; then
        echo "Stopping daemon..."
        "$BINARY" stop 2>/dev/null || true
    fi
}

# Set up signal handlers to cleanup on exit
trap cleanup EXIT INT TERM

# Stop daemon if running (in case it wasn't stopped properly)
if [ -f "$BINARY" ]; then
    "$BINARY" stop 2>/dev/null || true
    # Give it a moment to fully stop
    sleep 0.5
fi

# Build is handled by Air, so we just need to start
if [ -f "$BINARY" ]; then
    echo "Starting daemon in dev mode (console logging enabled)..."
    # Run in foreground - Air will send signals to this process
    CLIO_DEV=true "$BINARY" start
else
    echo "Error: Binary not found at $BINARY"
    exit 1
fi

