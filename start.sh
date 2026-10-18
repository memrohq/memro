#!/bin/bash

# memro.co - Startup Script
# Spin up all services separately

set -e

echo "🚀 Starting memro.co services..."
echo ""

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 1. Start infrastructure (Postgres + pgvector)
echo -e "${BLUE}📦 Starting infrastructure (Postgres + pgvector)...${NC}"
docker compose up -d db
echo -e "${GREEN}✅ Infrastructure started${NC}"
echo ""

# Wait for Postgres
echo "⏳ Waiting for Postgres..."
sleep 5

# 2. Start backend
echo -e "${BLUE}🐹 Starting backend (Go)...${NC}"
docker compose up -d --build backend
echo -e "${GREEN}✅ Backend started${NC}"
echo ""

# Wait for backend
echo "⏳ Waiting for backend..."
sleep 3

# 3. Start frontend
echo -e "${BLUE}⚡ Starting developer UI...${NC}"
docker compose up -d frontend
echo -e "${GREEN}✅ Developer UI started${NC}"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo -e "${GREEN}🎉 All services are starting!${NC}"
echo ""
echo "📍 Service URLs:"
echo "   • Backend API:      http://localhost:8081"
echo "   • Developer UI:     http://localhost:5173"
echo ""
echo "📊 Infrastructure:"
echo "   • Postgres:         localhost:5432 (pgvector)"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "💡 Useful commands:"
echo "   • View logs:        docker compose logs -f [service]"
echo "   • Stop all:         docker compose down"
echo "   • Restart service:  docker compose restart [service]"
echo ""
