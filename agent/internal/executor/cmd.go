package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/enterprise-rat/agent/internal/models"
)

const (
	maxOutputSize = 5 * 1024 * 1024
	maxArgLength  = 1024
)

func ExecuteCommand(req *models.CommandRequest) *models.CommandResponse {
	resp := &models.CommandResponse{
		CommandID: req.CommandID,
	}

	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 60
	}

	if req.TimeoutSeconds > 3600 {
		req.TimeoutSeconds = 3600
	}

	executable := sanitizePath(req.Executable)
	sanitizedArgs := make([]string, 0, len(req.Args))
	for _, arg := range req.Args {
		arg = sanitizePath(arg)
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

	cmd := exec.CommandContext(ctx, executable, sanitizedArgs...)
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

func sanitizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\x00", "")

	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == "" {
		return ""
	}

	if strings.HasPrefix(cleaned, "..") {
		return ""
	}

	return cleaned
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
