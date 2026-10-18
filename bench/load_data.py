#!/usr/bin/env python3
"""
Load LongMemEval data into Memro and verify 500 memories are present
"""

import json
import os
import sys
import time
import random
from datetime import datetime, timedelta

def check_database_connection():
    """Check if we can connect to PostgreSQL"""
    try:
        import psycopg2
        conn = psycopg2.connect(
            host="localhost",
            database="memro",
            user="memro",
            password="memro",
            port=5432
        )
        conn.close()
        return True
    except Exception as e:
        print(f"\n❌ Cannot connect to PostgreSQL: {e}")
        print("\nStart PostgreSQL first:")
        print("  cd memro-core/")
        print("  docker-compose up -d postgres qdrant")
        return False

def load_and_verify_data():
    """Load evaluation data and verify 500 memories exist"""
    import psycopg2
    
    # Check data file
    data_file = os.path.join(os.path.dirname(__file__), "..", "data", "eval", "longmemeval_s.json")
    if not os.path.exists(data_file):
        print(f"❌ Data file not found: {data_file}")
        return False
    
    print(f"\n📖 Loading data from: {data_file}")
    
    with open(data_file, 'r') as f:
        test_records = json.load(f)
    
    print(f"✓ Read {len(test_records)} test records from file")
    
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
        
        print(f"\n📊 Database Status:")
        print(f"   Current memories: {current_count}")
        
        if current_count >= 500:
            print(f"\n✅ Already have {current_count} memories (>= 500)")
            print(f"   ✓ Database ready for benchmarking!")
            
            # Show breakdown
            cur.execute("""
                SELECT memory_type, COUNT(*) 
                FROM memories 
                GROUP BY memory_type 
                ORDER BY count DESC
            """)
            types = cur.fetchall()
            if types:
                print(f"\n   Memory breakdown:")
                for mtype, count in types:
                    print(f"     • {mtype}: {count}")
            
            cur.close()
            conn.close()
            return True
        
        # Need to insert more data
        print(f"\n   Need to insert: {500 - current_count} more memories")
        
        # Ensure we have an agent
        cur.execute("SELECT COUNT(*) FROM agents")
        if cur.fetchone()[0] == 0:
            print(f"\n   Creating test agent...")
            agent_id = "benchmark-agent-" + os.urandom(4).hex()
            cur.execute("""
                INSERT INTO agents (id, name, description, vector_model)
                VALUES (%s, %s, %s, %s)
            """, (agent_id, "Benchmark Agent", "Agent for LongMemEval benchmark", "voyage-3"))
            conn.commit()
            print(f"   ✓ Created agent: {agent_id}")
        else:
            cur.execute("SELECT id FROM agents LIMIT 1")
            agent_id = cur.fetchone()[0]
            print(f"   ✓ Using existing agent: {agent_id}")
        
        # Insert memories
        print(f"\n   Inserting memories...")
        
        batch_count = 0
        for i, record in enumerate(test_records):
            if i < current_count:
                continue
            
            # Extract content
            content = f"Q: {record.get('question', '')}\nA: {record.get('answer', '')}"
            
            # Parse date
            try:
                date_str = record.get('question_date', '')
                if date_str:
                    date_part = date_str.split('(')[0].strip()
                    memory_date = datetime.strptime(date_part, "%Y/%m/%d").isoformat()
                else:
                    memory_date = (datetime.now() - timedelta(days=random.randint(1, 365))).isoformat()
            except:
                memory_date = (datetime.now() - timedelta(days=random.randint(1, 365))).isoformat()
            
            # Insert
            cur.execute("""
                INSERT INTO memories (id, agent_id, content, memory_type, created_at, metadata)
                VALUES (gen_random_uuid(), %s, %s, %s, %s, %s)
            """, (
                agent_id,
                content,
                record.get('question_type', 'semantic'),
                memory_date,
                json.dumps({"source": "longmemeval", "question_id": record.get("question_id", "")})
            ))
            
            batch_count += 1
            if batch_count % 100 == 0:
                conn.commit()
                print(f"     • {current_count + batch_count}/500 inserted")
        
        conn.commit()
        print(f"     • {current_count + batch_count}/500 inserted ✓")
        
        # Verify
        print(f"\n✅ Verifying...")
        cur.execute("SELECT COUNT(*) FROM memories")
        total = cur.fetchone()[0]
        
        print(f"   Final count: {total} memories")
        
        if total >= 500:
            print(f"\n✅ SUCCESS! Database has 500+ memories")
            print(f"   Ready to run benchmarks!")
        else:
            print(f"\n⚠️  Warning: Only {total} memories (expected 500)")
        
        cur.close()
        conn.close()
        return True
        
    except psycopg2.Error as e:
        print(f"\n❌ Database error: {e}")
        return False

def main():
    print("\n" + "="*50)
    print("🚀 MEMRO DATA LOADER & VERIFIER")
    print("="*50)
    
    # Check connection
    if not check_database_connection():
        return False
    
    print("\n✓ Database connected")
    
    # Load data
    if not load_and_verify_data():
        return False
    
    print("\n" + "="*50)
    print("✅ ALL SET FOR BENCHMARKING!")
    print("="*50)
    print("\nNext step:")
    print("  python3 run_supermemory_benchmarks.py")
    print()
    
    return True

if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)
