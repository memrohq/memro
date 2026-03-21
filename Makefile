# memro.co - Build & Execution Makefile

.PHONY: setup local infra backend frontend down logs ps clean

# Default: Start all services in the background (Local Dev)
local: setup
	@echo "🚀 Starting all Memro services..."
	./start.sh

# Start only infrastructure (Postgres + Qdrant)
infra: setup
	@echo "📦 Starting infrastructure (Postgres + Qdrant)..."
	docker compose up -d db qdrant

# Run only the backend API (and infra)
backend: setup
	@echo "🦀 Starting Rust backend & infrastructure..."
	docker compose up -d db qdrant backend

# Run backend with hot-reloading (cargo-watch)
# This will start infra if not already running
watch: setup
	@echo "🔍 Starting infrastructure and Rust backend in watch mode..."
	docker compose up backend

# Run only the frontend components
frontend: setup
	@echo "🌐 Starting landing page & developer UI..."
	docker compose up -d frontend-landing frontend-developer

# Stop all services
down:
	@echo "🛑 Stopping all services..."
	docker compose down

# View logs for all services
logs:
	docker compose logs -f

# Check service status
ps:
	docker compose ps

# Setup project (initialize .env)
setup:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "✅ Created .env from .env.example"; \
	fi

# Clean up docker images and volumes (Advanced)
clean:
	@echo "🧹 Cleaning up docker resources..."
	docker compose down -v --rmi local --remove-orphans
