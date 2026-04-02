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

## Security Improvements (v2.0)

### Cryptography & Key Derivation
- **Argon2id**: Upgraded from SHA-256 to Argon2id for password-based key derivation. Uses memory-hard parameters (64MB, 2 iterations) to resist GPU/ASIC attacks.
- **JWT Algorithm Validation**: Strict validation ensures only HS256 signing method is accepted, preventing "none" algorithm attacks.

### WebSocket & HTTP Security
- **Dynamic CORS**: Origins are strictly parsed from `CORS_ALLOWED_ORIGINS` environment variable. No hardcoded localhost addresses in production.
- **Rate Limiting**: Login and enrollment endpoints have IP-based rate limiting (5 requests/minute) to prevent brute-force attacks.
- **WebSocket Message Limits**: 4MB message size limits on both client and server to handle large command outputs.

### Agent Security
- **Path Traversal Prevention**: File manager uses `filepath.Clean()` and validates against allowed directories to block `../` attacks.
- **Command Execution Timeout**: All commands wrapped in `context.WithTimeout` (default 60s) to prevent zombie processes.
- **Agent Enrollment Secret**: Agents must provide `AGENT_ENROLLMENT_SECRET` header during registration.

### Database Security
- **Parameterized Queries**: All database queries use `$1, $2` parameterization - no string concatenation.
- **Input Sanitization**: Command payloads and arguments sanitized before storage.

### Container Security
- **Non-root Users**: All Docker containers run as non-root (`USER nonroot` / `USER appuser`).
- **Minimal Base Images**: Alpine-based images with minimal attack surface.

### Frontend Security
- **XSS Protection**: Terminal output sanitized with DOMPurify before rendering in xterm.js.
- **Secure Cookies**: JWT tokens use `HttpOnly`, `Secure`, and `SameSite=Strict` attributes.

---

## Deployment Guide

### 1. Generate Production Environment File

Create `.env.prod` in the project root:

```env
# Domain Configuration
DOMAIN=your-domain.com
CORS_ALLOWED_ORIGINS=https://your-domain.com

# Database (generate strong password!)
POSTGRES_USER=ratadmin
POSTGRES_PASSWORD=$(openssl rand -base64 32)
POSTGRES_DB=ratdb

# JWT Security (generate: openssl rand -base64 32)
JWT_SECRET=$(openssl rand -base64 32)
AGENT_ENROLLMENT_SECRET=$(openssl rand -base64 32)

# Let's Encrypt
LETSENCRYPT_EMAIL=admin@your-domain.com
```

### 2. Generate mTLS Certificates

```bash
cd backend/certs
./generate_certs.sh your-domain.com

# Or for IP-based deployment:
./generate_certs.sh 192.168.1.100
```

This creates:
- `ca.crt` / `ca.key` - Root CA
- `server.crt` / `server.key` - Backend server certificate
- `agent.crt` / `agent.key` - Agent certificate
- `agent.bundle.crt` - Agent cert + CA for agent distribution

### 3. Build Agent Binaries

```bash
# Windows
./build_agents.bat

# Output in agent/dist/
# - agent-windows-amd64.exe
# - agent-linux-amd64
```

### 4. Deploy with Docker Compose

```bash
# Production deployment (non-dev)
docker-compose -f docker-compose.prod.yml up -d

# Verify services
docker-compose -f docker-compose.prod.yml ps

# View logs
docker-compose -f docker-compose.prod.yml logs -f
```

### 5. Obtain SSL Certificate (Let's Encrypt)

```bash
# After initial deployment, run certbot
docker-compose -f docker-compose.prod.yml up -d certbot
```

### 6. Running Agents

Set environment variables on target machines:

```bash
# Linux
export RAT_SERVER_URL=wss://your-domain.com/api/v1/ws
export AGENT_ENROLLMENT_SECRET=your-secret-from-env
export RAT_AGENT_TOKEN=optional-token
./agent-linux-amd64

# Windows (PowerShell)
$env:RAT_SERVER_URL="wss://your-domain.com/api/v1/ws"
$env:AGENT_ENROLLMENT_SECRET="your-secret"
.\agent-windows-amd64.exe
```

---

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
