package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/enterprise-rat/agent/internal/models"
)

const (
	maxOutputSize  = 5 * 1024 * 1024
	maxArgLength   = 1024
	defaultTimeout = 60
	maxTimeout     = 3600
)

var (
	windowsExecutables = map[string]bool{
		"ping":       true,
		"ipconfig":   true,
		"netstat":    true,
		"tracert":    true,
		"route":      true,
		"arp":        true,
		"nslookup":   true,
		"hostname":   true,
		"systeminfo": true,
		"tasklist":   true,
		"ver":        true,
	}
	linuxExecutables = map[string]bool{
		"ping":       true,
		"ifconfig":   true,
		"netstat":    true,
		"traceroute": true,
		"route":      true,
		"arp":        true,
		"nslookup":   true,
		"hostname":   true,
		"uname":      true,
		"ps":         true,
		"ver":        true,
		"ip":         true,
		"iwconfig":   true,
	}
)

func getAllowedExecutables() map[string]bool {
	if runtime.GOOS == "windows" {
		return windowsExecutables
	}
	return linuxExecutables
}

func ExecuteCommand(req *models.CommandRequest) *models.CommandResponse {
	resp := &models.CommandResponse{
		CommandID: req.CommandID,
	}

	if req.TimeoutSeconds <= 0 || req.TimeoutSeconds > maxTimeout {
		req.TimeoutSeconds = defaultTimeout
	}

	executable := strings.ToLower(filepath.Base(req.Executable))

	if strings.Contains(req.Executable, "/") || strings.Contains(req.Executable, "\\") {
		resp.ExitCode = -1
		resp.ErrorMsg = "execution denied: path traversal not allowed"
		return resp
	}

	allowedExecutables := getAllowedExecutables()
	if !allowedExecutables[executable] {
		resp.ExitCode = -1
		resp.ErrorMsg = "execution denied: executable not in allowlist"
		return resp
	}

	sanitizedArgs := make([]string, 0, len(req.Args))
	for _, arg := range req.Args {
		arg = strings.TrimSpace(strings.ReplaceAll(arg, "\x00", ""))
		if len(arg) > maxArgLength {
			arg = arg[:maxArgLength]
		}
		sanitizedArgs = append(sanitizedArgs, arg)
	}

	if len(sanitizedArgs) > 100 {
		sanitizedArgs = sanitizedArgs[:100]
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stdoutBuf := newLimitedBuffer(maxOutputSize)
	stderrBuf := newLimitedBuffer(maxOutputSize)

	cmd := exec.CommandContext(ctx, req.Executable, sanitizedArgs...)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()

	resp.StdOut = strings.TrimSpace(stdoutBuf.String())
	resp.StdErr = strings.TrimSpace(stderrBuf.String())

	if ctx.Err() == context.DeadlineExceeded {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
		resp.ExitCode = -1
		resp.ErrorMsg = "command timed out"
		if stdoutBuf.truncated {
			resp.StdOut = "(output truncated due to timeout)"
		}
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

type limitedBuffer struct {
	buf       *bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{
		buf:   new(bytes.Buffer),
		limit: limit,
	}
}

func (l *limitedBuffer) Write(p []byte) (n int, err error) {
	if l.truncated {
		return 0, io.EOF
	}

	remaining := l.limit - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return 0, io.EOF
	}

	if len(p) > remaining {
		p = p[:remaining]
		l.truncated = true
	}

	return l.buf.Write(p)
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}
