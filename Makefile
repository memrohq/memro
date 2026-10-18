# memro.co - Build & Execution Makefile

.PHONY: setup local infra backend dev test frontend down logs ps clean

# Default: Start all services in the background (Local Dev)
local: setup
	@echo "🚀 Starting all Memro services..."
	./start.sh

# Start only infrastructure (Postgres + pgvector)
infra: setup
	@echo "📦 Starting infrastructure (Postgres + pgvector)..."
	docker compose up -d db

# Run only the backend API (and infra)
backend: setup
	@echo "🐹 Starting Go backend & infrastructure..."
	docker compose up -d --build db backend

# Run the Go backend natively against the dockerized database
dev: setup
	@echo "🐹 Starting database, then the Go backend natively..."
	docker compose up -d db
	cd server && DATABASE_URL=postgres://memro:memro@localhost:5432/memro go run ./cmd/memro-api

# Run backend tests (integration tests need the db service running)
test:
	cd server && MEMRO_TEST_DATABASE_URL=postgres://memro:memro@localhost:5432/memro go test -race ./...

# Run only the frontend components
frontend: setup
	@echo "🌐 Starting landing page & developer UI..."
	docker compose up -d frontend

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
