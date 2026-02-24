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

# 1. Start infrastructure (Postgres + Qdrant)
echo -e "${BLUE}📦 Starting infrastructure (Postgres + Qdrant)...${NC}"
docker compose up -d db qdrant
echo -e "${GREEN}✅ Infrastructure started${NC}"
echo ""

# Wait for Postgres
echo "⏳ Waiting for Postgres..."
sleep 5

# 2. Start backend
echo -e "${BLUE}🦀 Starting backend (Rust)...${NC}"
docker compose up -d backend
echo -e "${GREEN}✅ Backend started${NC}"
echo ""

# Wait for backend
echo "⏳ Waiting for backend to compile..."
sleep 10

# 3. Start frontend-landing
echo -e "${BLUE}🌐 Starting landing page...${NC}"
docker compose up -d frontend-landing
echo -e "${GREEN}✅ Landing page started${NC}"
echo ""

# 4. Start frontend-developer
echo -e "${BLUE}⚡ Starting developer UI...${NC}"
docker compose up -d frontend-developer
echo -e "${GREEN}✅ Developer UI started${NC}"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo -e "${GREEN}🎉 All services are starting!${NC}"
echo ""
echo "📍 Service URLs:"
echo "   • Backend API:      http://localhost:8081"
echo "   • Landing Page:     http://localhost:3000"
echo "   • Developer UI:     http://localhost:5174"
echo ""
echo "📊 Infrastructure:"
echo "   • Postgres:         localhost:5432"
echo "   • Qdrant:           localhost:6343"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "💡 Useful commands:"
echo "   • View logs:        docker compose logs -f [service]"
echo "   • Stop all:         docker compose down"
echo "   • Restart service:  docker compose restart [service]"
echo ""
