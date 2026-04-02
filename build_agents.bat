@echo off
REM =============================================================================
REM Enterprise RAT Agent Build Script
REM Cross-compiles the agent for Windows and Linux (amd64)
REM =============================================================================

setlocal enabledelayedexpansion

set "PROJECT_ROOT=%~dp0"
set "AGENT_DIR=%PROJECT_ROOT%agent"
set "DIST_DIR=%AGENT_DIR%\dist"
set "LD_FLAGS=-s -w"

echo ==============================================================================
echo Enterprise RAT Agent Build Script
echo ==============================================================================
echo.

REM Create output directory
if not exist "%DIST_DIR%" (
    echo [+] Creating output directory: %DIST_DIR%
    mkdir "%DIST_DIR%"
)

cd /d "%AGENT_DIR%"
if errorlevel 1 (
    echo [!] ERROR: Failed to change to agent directory
    exit /b 1
)

echo.
echo [+] Checking Go installation...
go version
if errorlevel 1 (
    echo [!] ERROR: Go is not installed or not in PATH
    exit /b 1
)

echo.
echo ==============================================================================
echo Building Windows AMD64 Agent...
echo ==============================================================================
set "OUT_NAME=agent-windows-amd64.exe"
echo [*] Output: %DIST_DIR%\%OUT_NAME%
go build -ldflags="%LD_FLAGS%" -o "%DIST_DIR%\%OUT_NAME%" ./cmd/agent
if errorlevel 1 (
    echo [!] ERROR: Windows build failed
    exit /b 1
)
echo [+] Windows AMD64 build complete

REM Get file size
for %%A in ("%DIST_DIR%\%OUT_NAME%") do echo [*] Size: %%~zA bytes

echo.
echo ==============================================================================
echo Building Linux AMD64 Agent...
echo ==============================================================================
set "OUT_NAME=agent-linux-amd64"
echo [*] Output: %DIST_DIR%\%OUT_NAME%
set GOOS=linux
set GOARCH=amd64
go build -ldflags="%LD_FLAGS%" -o "%DIST_DIR%\%OUT_NAME%" ./cmd/agent
if errorlevel 1 (
    echo [!] ERROR: Linux build failed
    exit /b 1
)
echo [+] Linux AMD64 build complete

REM Get file size
for %%A in ("%DIST_DIR%\%OUT_NAME%") do echo [*] Size: %%~zA bytes

echo.
echo ==============================================================================
echo Build Summary
echo ==============================================================================
echo.
dir /b "%DIST_DIR%"

echo.
echo ==============================================================================
echo Build Complete!
echo ==============================================================================
echo.
echo Binaries are located in: %DIST_DIR%
echo - agent-windows-amd64.exe  : Windows AMD64
echo - agent-linux-amd64        : Linux AMD64
echo.
echo To run the agent, set these environment variables:
echo   RAT_SERVER_URL=wss://your-server.com/api/v1/ws
echo   AGENT_ENROLLMENT_SECRET=your-secret-here
echo   RAT_AGENT_TOKEN=optional-token
echo.

endlocal
exit /b 0