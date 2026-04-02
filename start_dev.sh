#!/bin/bash

echo "Starting Enterprise RAT Development Environment..."

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Starting Backend Server...${NC}"
(cd backend && go run cmd/server/main.go) &
BACKEND_PID=$!

sleep 2

echo -e "${YELLOW}Starting Frontend Dev Server...${NC}"
(cd frontend && npm install && npm run dev) &
FRONTEND_PID=$!

sleep 5

echo -e "${YELLOW}Starting Agent (Demo)...${NC}"
(cd agent && go run cmd/agent/main.go) &
AGENT_PID=$!

echo ""
echo -e "${GREEN}All services started!${NC}"
echo "  Backend:   http://localhost:8080"
echo "  Frontend:  http://localhost:5173"
echo ""
echo "PIDs: Backend=$BACKEND_PID | Frontend=$FRONTEND_PID | Agent=$AGENT_PID"
echo ""
echo "Press Ctrl+C to stop all services"

# Wait for any process to exit
wait
