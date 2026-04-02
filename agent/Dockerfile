#==============================================
# Enterprise RAT Agent - Multi-stage Dockerfile
# Supports Windows and Linux builds
#==============================================

#----------------------------------
# Stage 1: Linux Agent (default)
#----------------------------------
FROM golang:1.22-alpine AS linux-builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build for Linux
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o agent-linux-amd64 ./cmd/agent

#----------------------------------
# Stage 2: Linux ARM64 Agent
#----------------------------------
FROM golang:1.22-alpine AS linux-arm64-builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build for Linux ARM64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o agent-linux-arm64 ./cmd/agent

#----------------------------------
# Stage 3: Windows Agent (using Docker buildx)
#----------------------------------
FROM golang:1.22-windowsservercore AS windows-builder

WORKDIR C:/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build for Windows AMD64
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o agent-windows-amd64.exe ./cmd/agent

#----------------------------------
# Stage 4: Linux Runtime (default)
#----------------------------------
FROM alpine:3.19 AS linux-runtime

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

# Copy binary
COPY --from=linux-builder /app/agent-linux-amd64 ./agent
COPY --from=linux-arm64-builder /app/agent-linux-arm64 ./agent-arm64

RUN chmod +x ./agent

# Create config directory
RUN mkdir -p /etc/rat-agent

# Create non-root user
RUN adduser -D -g '' ratagent

USER ratagent

EXPOSE 8080

ENV RAT_SERVER_URL=wss://localhost:8080/api/v1/ws
ENV RAT_AGENT_TOKEN=

ENTRYPOINT ["./agent"]

#----------------------------------
# Stage 5: Windows Runtime (Nano Server)
#----------------------------------
FROM mcr.microsoft.com/windows/nanoserver:ltsc2022 AS windows-runtime

WORKDIR C:/agent

# Copy binary
COPY --from=windows-builder /app/agent-windows-amd64.exe ./agent.exe

# Copy PowerShell for health check
COPY --from=mcr.microsoft.com/powershell:ltsc2022-nanoserver /pwsh/pwsh.exe C:/Windows/System32/

EXPOSE 8080

CMD ["C:/agent/agent.exe"]

#==============================================
# Build Instructions:
#
# Linux (default):
#   docker build -t rat-agent:linux --target linux-runtime .
#
# Windows:
#   docker buildx build --platform windows/amd64 -t rat-agent:windows --target windows-runtime .
#
# All platforms:
#   docker buildx bake --platform linux/amd64,linux/arm64,windows/amd64
#
#==============================================
