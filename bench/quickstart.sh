#!/bin/bash
# Quick Start: Complete Memro Benchmark Setup
# Sets up all dependencies and runs Supermemory-scale benchmarks

set -e  # Exit on error

echo "=================================="
echo "🚀 MEMRO BENCHMARK QUICK START"
echo "=================================="

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# Check prerequisites
echo ""
echo "1️⃣  Checking prerequisites..."
echo "=================================="

# Check Docker
if ! command -v docker &> /dev/null; then
    print_error "Docker not found. Please install Docker."
    exit 1
fi
print_status "Docker installed"

# Check Rust
if ! command -v cargo &> /dev/null; then
    print_warning "Rust/cargo not found. Will need Rust to build backend."
fi

# Check Python
if ! command -v python3 &> /dev/null; then
    print_error "Python3 not found. Please install Python3."
    exit 1
fi
print_status "Python3 installed"

# Check Python dependencies
echo ""
echo "2️⃣  Checking Python dependencies..."
echo "=================================="

python3 -c "import memro" 2>/dev/null && print_status "memro SDK installed" || print_warning "memro SDK not installed - will try to install"

# Install Python dependencies if needed
if [ -f "$SCRIPT_DIR/requirements.txt" ]; then
    python3 -m pip install -q -r "$SCRIPT_DIR/requirements.txt" 2>/dev/null && \
        print_status "Python dependencies installed" || \
        print_warning "Some Python dependencies may not be installed"
fi

# Check if services are running
echo ""
echo "3️⃣  Checking services..."
echo "=================================="

# Function to check if port is open
check_port() {
    nc -z 127.0.0.1 $1 2>/dev/null
    return $?
}

MEMRO_RUNNING=false
QDRANT_RUNNING=false
POSTGRES_RUNNING=false

if check_port 8081; then
    print_status "Memro API running on :8081"
    MEMRO_RUNNING=true
else
    print_warning "Memro API not running on :8081"
fi

if check_port 6334; then
    print_status "Qdrant running on :6334"
    QDRANT_RUNNING=true
else
    print_warning "Qdrant not running on :6334"
fi

if check_port 5432; then
    print_status "PostgreSQL running on :5432"
    POSTGRES_RUNNING=true
else
    print_warning "PostgreSQL not running on :5432"
fi

# Start services if needed
echo ""
echo "4️⃣  Starting services..."
echo "=================================="

if [ "$QDRANT_RUNNING" = false ]; then
    print_warning "Starting Qdrant..."
    cd "$PROJECT_ROOT/memro-core"
    
    # Check if docker-compose.yml exists
    if [ -f "docker-compose.yml" ]; then
        docker-compose up -d qdrant 2>/dev/null || true
        sleep 3
        if check_port 6334; then
            print_status "Qdrant started"
        else
            print_error "Failed to start Qdrant"
        fi
    else
        print_warning "docker-compose.yml not found, Qdrant may need manual startup"
    fi
fi

if [ "$MEMRO_RUNNING" = false ]; then
    print_warning "Memro API not running"
    echo ""
    echo "To start Memro backend:"
    echo "  cd $PROJECT_ROOT/memro-core/backend"
    echo "  cargo run --release"
    echo ""
    echo "Or if running test environment:"
    echo "  cd $PROJECT_ROOT/memro-core"
    echo "  docker-compose up memro-backend"
    echo ""
    print_error "Please start Memro backend and run this script again"
    exit 1
fi

# Run benchmarks
echo ""
echo "5️⃣  Running benchmarks..."
echo "=================================="

cd "$SCRIPT_DIR"

# Create output directory
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_DIR="results_$TIMESTAMP"
mkdir -p "$OUTPUT_DIR"

print_status "Running Supermemory-scale benchmark (500 memories)..."
python3 supermemory_scale_benchmark.py \
    --base_url http://localhost:8081 \
    --size 500 \
    --output "$OUTPUT_DIR/benchmark_500_memories.json"

print_status "Running scaling analysis (100-1000 memories)..."
python3 supermemory_scale_benchmark.py \
    --base_url http://localhost:8081 \
    --scaling \
    --output "$OUTPUT_DIR/scaling_analysis.json"

print_status "Comparing with Supermemory baseline (500 memories)..."
python3 supermemory_scale_benchmark.py \
    --base_url http://localhost:8081 \
    --compare \
    --output "$OUTPUT_DIR/memro_vs_supermemory.json"

# Generate summary
echo ""
echo "6️⃣  Generating report..."
echo "=================================="

cat > "$OUTPUT_DIR/summary.txt" << EOF
MEMRO BENCHMARK RESULTS
Generated: $(date)

Tests Run:
✓ 500-memory benchmark (LongMemEval style)
✓ Scaling analysis (100-1000 memories)  
✓ Supermemory comparison

Output Files:
- benchmark_500_memories.json
- scaling_analysis.json
- memro_vs_supermemory.json

Next Steps:
1. Review results in $OUTPUT_DIR/
2. Compare against Supermemory baseline metrics
3. Apply optimizations from doc/OPTIMIZATION_COMPLETE.md
4. Re-run benchmarks to measure improvement

For detailed analysis:
  python3 benchmark_report_generator.py \
    --input $OUTPUT_DIR/benchmark_500_memories.json \
    --output_md $OUTPUT_DIR/BENCHMARK_RESULTS.md
EOF

print_status "Report saved to $OUTPUT_DIR/summary.txt"

echo ""
echo "=================================="
echo "✅ BENCHMARK COMPLETE!"
echo "=================================="
echo ""
echo "📁 Results saved in: $OUTPUT_DIR"
echo ""
echo "To generate detailed markdown report:"
echo "  python3 benchmark_report_generator.py --input $OUTPUT_DIR/benchmark_500_memories.json"
echo ""
echo "To view results:"
echo "  cat $OUTPUT_DIR/summary.txt"
echo ""
