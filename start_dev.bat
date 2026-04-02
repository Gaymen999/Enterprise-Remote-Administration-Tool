@echo off
title Enterprise RAT - Development Environment
echo Starting Enterprise RAT Development Environment...
echo.

start "Backend Server" cmd /k "cd backend && go run cmd/server/main.go"

timeout /t 2 /nobreak >nul

start "Frontend Dev" cmd /k "cd frontend && npm install && npm run dev"

timeout /t 5 /nobreak >nul

start "Agent (Demo)" cmd /k "cd agent && go run cmd/agent/main.go"

echo.
echo All services started!
echo - Backend: http://localhost:8080
echo - Frontend: http://localhost:5173
echo.
echo Press any key to exit this window (services will keep running)...
pause >nul
