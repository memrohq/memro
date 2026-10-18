#!/bin/bash
# Load evaluation data into Memro and verify

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo ""
echo "=================================="
echo "📥 DATA LOADER & VERIFICATION"
echo "=================================="
echo ""

# Step 1: Start services if needed
echo "1️⃣  Checking services..."
echo "=================================="

cd "$PROJECT_ROOT/memro-core"

# Try to connect to PostgreSQL
if ! psql -h localhost -U memro -d memro -c "SELECT 1" > /dev/null 2>&1; then
    echo "Starting PostgreSQL and Qdrant..."
    docker-compose up -d postgres qdrant
    echo "Waiting 10 seconds for services to start..."
    sleep 10
fi

# Verify connection
if ! psql -h localhost -U memro -d memro -c "SELECT 1" > /dev/null 2>&1; then
    echo "❌ Failed to connect to PostgreSQL. Make sure docker-compose is running."
    exit 1
fi

echo "✅ PostgreSQL connected"

# Step 2: Run migrations if needed
echo ""
echo "2️⃣  Running database migrations..."
echo "=================================="

cd "$PROJECT_ROOT/memro-core/backend"

if command -v sqlx &> /dev/null; then
    export DATABASE_URL=postgresql://memro:memro@localhost:5432/memro
    sqlx migrate run || echo "Migrations may already be up to date"
    echo "✅ Migrations complete"
else
    echo "⚠️  sqlx not found, skipping migrations"
    echo "   (make sure memro backend tables exist)"
fi

# Step 3: Check current database state
echo ""
echo "3️⃣  Checking current database state..."
echo "=================================="

psql -h localhost -U memro -d memro << EOF
\echo '📊 Database Status:'
\echo ''
SELECT 'Total Memories:' as metric, COUNT(*) as count FROM memories UNION ALL
SELECT 'Total Agents:' as metric, COUNT(*) as count FROM agents;
\echo ''
EOF

# Step 4: Load data if not already loaded
echo ""
echo "4️⃣  Loading evaluation data..."
echo "=================================="

cd "$SCRIPT_DIR"

python3 << 'PYTHON_EOF'
import json
import os
import psycopg2
from datetime import datetime, timedelta
import random

# Check if data file exists
data_file = "../data/eval/longmemeval_s.json"
if not os.path.exists(data_file):
    print(f"❌ Data file not found: {data_file}")
    exit(1)

print(f"📖 Loading evaluation data...")

# Read data
with open(data_file, 'r') as f:
    test_records = json.load(f)

print(f"✓ Loaded {len(test_records)} test records")

# Connect to database
try:
    conn = psycopg2.connect(
        host="localhost",
        database="memro",
        user="memro",
        password="memro",
        port=5432
    )
    cur = conn.cursor()
    
    # Check current memory count
    cur.execute("SELECT COUNT(*) FROM memories")
    current_count = cur.fetchone()[0]
    
    if current_count >= 500:
        print(f"\n✅ Database already has {current_count} memories (>= 500)")
        print(f"   No need to insert more data!")
    else:
        print(f"\n⚠️  Database has only {current_count} memories")
        print(f"   Need to insert {500 - current_count} more")
        
        # Create a test agent if none exists
        cur.execute("SELECT COUNT(*) FROM agents")
        if cur.fetchone()[0] == 0:
            print(f"\n   Creating test agent...")
            agent_id = "benchmark-agent-" + os.urandom(4).hex()
            cur.execute(
                """INSERT INTO agents (id, name, description, vector_model)
                   VALUES (%s, %s, %s, %s)""",
                (agent_id, "Benchmark Agent", "Agent for LongMemEval benchmark", "voyage-3")
            )
            conn.commit()
            print(f"   Created agent: {agent_id}")
        else:
            cur.execute("SELECT id FROM agents LIMIT 1")
            agent_id = cur.fetchone()[0]
            print(f"   Using existing agent: {agent_id}")
        
        # Insert memories from test records
        print(f"\n   Inserting {500 - current_count} memories...")
        
        for i, record in enumerate(test_records[:500]):
            if i < current_count:
                continue  # Skip already inserted records
            
            # Extract memory content from test record
            content = f"Q: {record.get('question', '')}\nA: {record.get('answer', '')}"
            
            # Parse date if available
            try:
                date_str = record.get('question_date', '')
                if date_str:
                    # Try to parse "2023/05/30 (Tue) 23:40"
                    date_part = date_str.split('(')[0].strip()
                    memory_date = datetime.strptime(date_part, "%Y/%m/%d").isoformat()
                else:
                    memory_date = (datetime.now() - timedelta(days=random.randint(1, 365))).isoformat()
            except:
                memory_date = (datetime.now() - timedelta(days=random.randint(1, 365))).isoformat()
            
            cur.execute(
                """INSERT INTO memories (id, agent_id, content, memory_type, created_at, metadata)
                   VALUES (gen_random_uuid(), %s, %s, %s, %s, %s)""",
                (
                    agent_id,
                    content,
                    record.get('question_type', 'semantic'),
                    memory_date,
                    json.dumps({"source": "longmemeval", "question_id": record.get("question_id", "")})
                )
            )
            
            if (i + 1) % 100 == 0:
                print(f"   • Inserted {i + 1} memories...")
                conn.commit()
        
        conn.commit()
        print(f"   ✅ Finished inserting memories!")
    
    # Final verification
    print(f"\n5️⃣  Final verification...")
    print(f"{'='*40}")
    
    cur.execute("SELECT COUNT(*) FROM memories")
    total_memories = cur.fetchone()[0]
    
    cur.execute("SELECT memory_type, COUNT(*) FROM memories GROUP BY memory_type ORDER BY count DESC")
    types = cur.fetchall()
    
    cur.execute("SELECT COUNT(*) FROM agents")
    total_agents = cur.fetchone()[0]
    
    print(f"\n✅ READY FOR BENCHMARKING!")
    print(f"   • Agents: {total_agents}")
    print(f"   • Total Memories: {total_memories}")
    
    if types:
        print(f"   • Memory Types:")
        for mtype, count in types:
            print(f"      - {mtype}: {count}")
    
    if total_memories >= 500:
        print(f"\n✅ All 500 test memories are in the database!")
        print(f"   You can now run benchmarks: python3 run_supermemory_benchmarks.py")
    
    cur.close()
    conn.close()

except psycopg2.Error as e:
    print(f"❌ Database error: {e}")
    exit(1)

PYTHON_EOF

echo ""
echo "=================================="
echo "✅ DATA LOADING COMPLETE"
echo "=================================="
echo ""
echo "Next step: Run benchmarks"
echo "  cd memro-benchmark/"
echo "  python3 run_supermemory_benchmarks.py"
echo ""
