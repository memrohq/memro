#!/bin/bash
# Helper script to mimic 'air' functionality for Rust
# Provides hot-reloading using cargo-watch (local or via Docker)

# Exit on error
set -e

# Navigate to backend directory if not already there
CDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$CDIR"

echo "🚀 Starting Rust backend in watch mode..."

# Check if cargo-watch is installed locally
if command -v cargo-watch &> /dev/null; then
    echo "🦀 Found local cargo-watch. Running natively..."
    # Ensure infra is running in the background via the parent Makefile if possible
    # (Assuming we are in memrohq/memro-core/backend)
    if [ -f "../Makefile" ]; then
        echo "📦 Ensuring infrastructure is running..."
        (cd .. && docker compose up -d db qdrant)
    fi
    cargo watch -x run
else
    echo "🐳 cargo-watch not found locally. Running via Docker Compose..."
    # Run the backend service from the parent docker-compose.yml
    if [ -f "../docker-compose.yml" ]; then
        (cd .. && docker compose up backend)
    else
        echo "❌ Could not find docker-compose.yml in parent directory."
        exit 1
    fi
fi
