# memro.co - Quick Start Guide

## 🚀 Start All Services

### Option 1: One Command (Recommended)
```bash
./start.sh
```

### Option 2: Manually (Step by Step)

#### 1. Start Infrastructure (Postgres + pgvector)
```bash
docker compose up -d db
```

#### 2. Start Backend (Go)
```bash
docker compose up -d --build backend
```

#### 3. Start Developer UI
```bash
docker compose up -d frontend
```

---

## 📍 Service URLs

| Service | URL | Purpose |
|---------|-----|---------|
| **Backend API** | http://localhost:8081 | REST API |
| **Developer UI** | http://localhost:5173 | Infrastructure management |
| **Postgres** | localhost:5432 | Database + vectors (pgvector) |

---

## 🛠️ Useful Commands

### View Logs
```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f backend
docker compose logs -f frontend-developer
docker compose logs -f frontend-landing
```

### Restart Services
```bash
# Restart backend
docker compose restart backend

# Restart frontend
docker compose restart frontend-developer
docker compose restart frontend-landing
```

### Stop Everything
```bash
docker compose down
```

### Rebuild After Changes
```bash
# Rebuild specific service
docker compose up -d --build backend

# Rebuild all
docker compose up -d --build
```

---

## 🔧 Development Mode

### Backend (Hot Reload)
Run the Go backend natively for fast iteration (restart to pick up changes):
```bash
make dev        # starts Postgres in Docker, then `go run ./cmd/memro-api`
make test       # unit + integration tests against the same database
```

### Frontend Developer UI (Hot Reload)
Currently runs in production mode. For development:
```bash
cd frontend
npm run dev
```

### Frontend Landing (Static)
Edit `frontend-landing/index.html` and rebuild:
```bash
docker compose up -d --build frontend-landing
```

---

## ✅ Health Checks

```bash
# Backend
curl http://localhost:8081/health

# Create identity
curl -X POST http://localhost:8081/identity

# Create memory
curl -X POST http://localhost:8081/memory \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"xxx","content":"test","memory_type":"episodic","visibility":"private"}'
```

---

## 🐛 Troubleshooting

### Backend won't start
```bash
# Check logs
docker compose logs backend

# Rebuild
docker compose up -d --build backend
```

### Frontend build fails
```bash
# Check if dependencies are installed
cd frontend-developer
npm install

# Rebuild
docker compose up -d --build frontend-developer
```

### Semantic search returns nothing
Semantic search needs `OPENAI_API_KEY` (or `VOYAGE_API_KEY`) in `.env`;
without it the API falls back to keyword search. Check:
```bash
curl localhost:8081/health   # "embeddings": "enabled" when a key is set
```

---

## 📦 Clean Start

```bash
# Stop everything
docker compose down

# Remove volumes (WARNING: deletes data)
docker compose down -v

# Start fresh
./start.sh
```
