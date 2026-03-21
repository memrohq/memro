# Pre-Benchmark Setup Checklist

## What You Need Running Before Benchmarks

Benchmarks **require** these services to be operational:

| Service | Port | Purpose | Status Check |
|---------|------|---------|--------------|
| **PostgreSQL** | 5432 | Store memories, agents, metadata | `psql -h localhost -U memro -d memro -c "SELECT 1"` |
| **Qdrant** | 6334 | Vector database for embeddings | `curl http://localhost:6334/health` |
| **Memro API** | 8081 | Your backend service | `curl http://localhost:8081/health` |

---

## Quick Setup Guide

### Option 1: Using Docker Compose (Recommended)

```bash
cd memro-core/

# Start all services
docker-compose up -d

# Verify services are running
docker-compose ps

# Check logs
docker-compose logs -f
```

**Wait 10-15 seconds** for services to initialize and connect.

### Option 2: Manual Startup

#### 1. Start PostgreSQL
```bash
# If using Docker
docker run -d \
  --name memro-postgres \
  -e POSTGRES_USER=memro \
  -e POSTGRES_PASSWORD=memro \
  -e POSTGRES_DB=memro \
  -p 5432:5432 \
  postgres:15

# Wait for connection
sleep 5
```

#### 2. Start Qdrant
```bash
# If using Docker
docker run -d \
  --name memro-qdrant \
  -p 6334:6334 \
  -p 6335:6335 \
  qdrant/qdrant:latest

# Wait for startup
sleep 5
```

#### 3. Run Database Migrations
```bash
cd memro-core/backend/

# Set environment
export DATABASE_URL=postgresql://memro:memro@localhost:5432/memro

# Run migrations
sqlx migrate run
```

#### 4. Start Memro Backend
```bash
cd memro-core/backend/

# Development mode (faster to build)
cargo run

# Or production mode (slower to build, faster runtime)
cargo run --release
```

Wait for output: `Server running on 0.0.0.0:8081`

---

## Verify Setup

Before running benchmarks, verify all services:

```bash
#!/bin/bash

echo "Checking PostgreSQL..."
psql -h localhost -U memro -d memro -c "SELECT 1" > /dev/null 2>&1 && echo "✓ PostgreSQL OK" || echo "✗ PostgreSQL FAILED"

echo "Checking Qdrant..."
curl -s http://localhost:6334/health | grep -q "version" && echo "✓ Qdrant OK" || echo "✗ Qdrant FAILED"

echo "Checking Memro API..."
curl -s http://localhost:8081/health && echo "✓ Memro API OK" || echo "✗ Memro API FAILED"
```

---

## Run Benchmarks

Once all services are running:

### Option 1: Quick Start Script (Automated)
```bash
cd memro-benchmark/
chmod +x quickstart.sh
./quickstart.sh
```

### Option 2: Manual Benchmark Execution

```bash
cd memro-benchmark/

# Set Python environment
source ../.venv/bin/activate

# Run Supermemory-scale benchmark (500 memories)
python3 supermemory_scale_benchmark.py \
  --size 500 \
  --output results/benchmark_500.json

# Run scaling analysis
python3 supermemory_scale_benchmark.py \
  --scaling \
  --output results/scaling_analysis.json

# Run comparison with Supermemory baseline
python3 supermemory_scale_benchmark.py \
  --compare \
  --output results/memro_vs_supermemory.json
```

### Option 3: Comprehensive Benchmark Suite
```bash
cd memro-benchmark/
python3 run_supermemory_benchmarks.py
```

---

## Troubleshooting

### "Connection refused" on 8081
- Memro backend not running
- Solution: `cd memro-core/backend && cargo run --release`
- Wait 30-60 seconds for Rust compilation and startup

### "Connection refused" on 6334 (Qdrant)
- Qdrant not running
- Solution: `docker run -d -p 6334:6334 qdrant/qdrant:latest`

### "Connection refused" on 5432 (PostgreSQL)
- PostgreSQL not running
- Solution: Start PostgreSQL or use `docker-compose up postgres`

### Migrations failed
- Database not initialized
- Solution: `sqlx migrate run` in `memro-core/backend/`

### "Table memro does not exist"
- Migrations haven't run
- Solution: Ensure `sqlx migrate run` completes successfully

### Benchmarks timing out
- Services running but slow to respond
- Solution: 
  - Check service logs: `docker-compose logs -f [service]`
  - Increase timeout in benchmark script
  - Verify CPU/memory available

---

## What Benchmarks Do

1. **Create Agent**: Registers a test agent in Memro
2. **Insert Memories**: Adds 100-1000 test memories (episodic, semantic, procedural, profile)
3. **Query**: Tests 4 query patterns (factual, temporal, semantic, reasoning)
4. **Measure**: Records latency distribution (mean, P95, P99, P99.9)
5. **Analyze**: Compares results against Supermemory baselines

---

## Expected Results

After optimizations (P0 + P1):
- **Query Latency P95**: < 100ms (vs Supermemory ~250ms)
- **Throughput**: > 50 QPS
- **Scaling**: Sublinear growth (100-1000 memories)

---

## Next Steps

1. ✅ Services running
2. ✅ Run benchmarks
3. 👉 Analyze results
4. 👉 Compare vs Supermemory publish metrics
5. 👉 Calculate optimization impact
6. 👉 Generate markdown report

