# Enterprise RAT (Remote Administration Tool)

A production-ready, enterprise-grade remote administration tool built with Go, React/TypeScript, PostgreSQL, and WebSockets. Designed for secure, real-time monitoring and management of thousands of endpoints.

## Architecture Overview

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Web UI (React)│     │  Backend (Go)   │     │  Agent (Go)     │
│   Port: 5173    │◄───►│  Port: 8080     │◄───►│  Cross-Platform│
└─────────────────┘     └────────┬────────┘     └─────────────────┘
                                 │
                        ┌────────┴────────┐
                        │   PostgreSQL   │
                        │   Port: 5432   │
                        └────────────────┘
```

## Tech Stack

| Layer | Technology |
|-------|------------|
| **Frontend** | React 18, TypeScript, Vite, Tailwind CSS, xterm.js, Zustand |
| **Backend** | Go 1.22, Chi Router, gorilla/websocket, pgx/v5 |
| **Database** | PostgreSQL 15+ |
| **Agent** | Go 1.22 (cross-platform: Windows, Linux, macOS) |

## Project Structure

```
enterprise-rat/
├── backend/                    # Go API Server
│   ├── cmd/server/            # Entry point
│   ├── internal/
│   │   ├── api/               # HTTP handlers & routes
│   │   ├── auth/              # JWT authentication
│   │   ├── ws/                # WebSocket hub
│   │   ├── commands/          # Command dispatch
│   │   └── config/            # Configuration
│   ├── pkg/db/                # Database utilities
│   └── migrations/            # SQL migrations
├── frontend/                   # React SPA
│   ├── src/
│   │   ├── components/        # UI components
│   │   ├── pages/             # Route pages
│   │   ├── services/          # API clients
│   │   └── store/             # State management
├── agent/                     # Cross-platform agent
│   ├── cmd/agent/             # Entry point
│   └── internal/
│       ├── client/            # WSS client
│       ├── executor/         # Command execution
│       └── config/            # Identity & telemetry
├── start_dev.bat              # Windows startup script
├── start_dev.sh              # Unix startup script
└── README.md
```

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 18+
- PostgreSQL 15+
- Git

### 1. Database Setup

Create the database and run migrations:

```bash
# Connect to PostgreSQL
psql -U postgres

# Create database
CREATE DATABASE ratdb;
\q

# Run migrations
psql -U postgres -d ratdb -f backend/migrations/001_initial_schema.sql
psql -U postgres -d ratdb -f backend/migrations/002_seed_data.sql
```

### 2. Environment Variables

Create a `.env` file in the `backend/` directory:

```env
# Database
DB_URL=postgres://postgres:postgres@localhost:5432/ratdb?sslmode=disable

# Application
PORT=8080

# Security (change this in production!)
JWT_SECRET=your-super-secret-key-change-in-production
```

### 3. Start Development Environment

**Windows:**
```bash
.\start_dev.bat
```

**Unix/Linux/macOS:**
```bash
chmod +x start_dev.sh
./start_dev.sh
```

This will start:
- **Backend**: http://localhost:8080
- **Frontend**: http://localhost:5173
- **Agent**: Connects to backend automatically

### 4. Access the Application

1. Open http://localhost:5173
2. Login with default credentials (seeded in `002_seed_data.sql`):
   - **Username**: `admin`
   - **Password**: `admin123` *(update hash in production)*

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/auth/login` | User authentication |
| `GET` | `/api/v1/agents` | List all agents |
| `GET` | `/api/v1/agents/{id}` | Get agent details |
| `POST` | `/api/v1/commands` | Execute command |
| `GET` | `/api/v1/commands/{id}` | Get command status |
| `GET` | `/api/v1/commands/{id}/result` | Get command output |
| `GET` | `/api/v1/ws` | WebSocket endpoint |

## Development

### Backend

```bash
cd backend
go mod download
go run cmd/server/main.go
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

### Agent

```bash
cd agent
go run cmd/agent/main.go
```

### Environment Variables for Agent

```env
RAT_SERVER_URL=ws://localhost:8080/api/v1/ws
RAT_AGENT_TOKEN=<optional-jwt-token>
```

## Security Features

- **JWT Authentication**: Short-lived access tokens with secure refresh
- **RBAC**: Role-based access control (admin, viewer)
- **mTLS Ready**: X.509 certificate authentication support
- **Audit Logging**: All actions logged with user context

## Database Schema

### Core Tables

- `users` - User accounts and authentication
- `agents` - Endpoint inventory and status
- `commands` - Command queue and dispatch
- `command_results` - Command outputs and exit codes
- `audit_logs` - Security audit trail

## License

MIT License - See LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit changes
4. Push to the branch
5. Open a Pull Request
