package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type PtyManager struct {
	sessions    map[string]*PtySession
	hub         *Hub
	mu          sync.RWMutex
	maxSessions int
}

type PtySession struct {
	ID      string
	AgentID string
	UserID  string
	Cols    int
	Rows    int
	closed  bool
	mu      sync.Mutex
}

func NewPtyManager(hub *Hub) *PtyManager {
	return &PtyManager{
		sessions:    make(map[string]*PtySession),
		hub:         hub,
		maxSessions: 100,
	}
}

func (pm *PtyManager) CreateSession(agentID, userID, sessionID string, cols, rows int) (*PtySession, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.sessions) >= pm.maxSessions {
		return nil, fmt.Errorf("maximum session limit reached (%d)", pm.maxSessions)
	}

	session := &PtySession{
		ID:      sessionID,
		AgentID: agentID,
		UserID:  userID,
		Cols:    cols,
		Rows:    rows,
	}

	pm.sessions[sessionID] = session
	return session, nil
}

func (pm *PtyManager) GetSession(sessionID string) *PtySession {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.sessions[sessionID]
}

func (pm *PtyManager) RemoveSession(sessionID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.sessions, sessionID)
}

func (pm *PtyManager) SessionCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.sessions)
}

func (pm *PtyManager) CloseSession(sessionID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	session, ok := pm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}

	session.mu.Lock()
	session.closed = true
	session.mu.Unlock()

	delete(pm.sessions, sessionID)
	return nil
}

func (pm *PtyManager) Cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for sessionID := range pm.sessions {
		delete(pm.sessions, sessionID)
	}
	log.Printf("[PTY] All sessions cleaned up")
}

type PtyPayload struct {
	PtyType   string `json:"pty_type"`
	SessionID string `json:"session_id,omitempty"`
	Cols      int    `json:"cols,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Data      string `json:"data,omitempty"`
	Shell     string `json:"shell,omitempty"`
}

type PtyMessage struct {
	Type    string     `json:"type"`
	Payload PtyPayload `json:"payload"`
}

func (h *Handler) handlePtyStart(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	agentID := getString(payload, "agent_id")
	if sessionID == "" {
		sessionID = generateSecureID()
	}

	if agentID == "" {
		log.Printf("[PTY] Missing agent_id in start request from %s", client.ID)
		h.sendPtyError(client, sessionID, "agent_id is required")
		return
	}

	cols := int(getFloat(payload, "cols", 80))
	rows := int(getFloat(payload, "rows", 24))
	shell := getString(payload, "shell")

	// Store UserID so we know where to send output back to
	session, err := h.hub.ptyManager.CreateSession(agentID, client.ID, sessionID, cols, rows)
	if err != nil {
		log.Printf("[PTY] Failed to create session: %v", err)
		h.sendPtyError(client, sessionID, err.Error())
		return
	}
	_ = session

	agentMsg := map[string]interface{}{
		"type": "pty",
		"payload": PtyPayload{
			PtyType:   "start",
			SessionID: sessionID,
			Cols:      cols,
			Rows:      rows,
			Shell:     shell,
		},
	}

	msgBytes, _ := json.Marshal(agentMsg)
	if !h.hub.SendToAgent(agentID, msgBytes) {
		h.sendPtyError(client, sessionID, "agent not connected")
		h.hub.ptyManager.RemoveSession(sessionID)
		return
	}

	h.sendPtyStarted(client, sessionID)
	log.Printf("[PTY] Session %s started for agent %s by admin %s", sessionID, agentID, client.ID)
}

func (h *Handler) handlePtyResize(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	if sessionID == "" {
		return
	}

	session := h.hub.ptyManager.GetSession(sessionID)
	if session == nil || session.UserID != client.ID {
		return
	}

	cols := int(getFloat(payload, "cols", 80))
	rows := int(getFloat(payload, "rows", 24))

	session.Cols = cols
	session.Rows = rows

	agentMsg := map[string]interface{}{
		"type": "pty",
		"payload": PtyPayload{
			PtyType:   "resize",
			SessionID: sessionID,
			Cols:      cols,
			Rows:      rows,
		},
	}

	msgBytes, _ := json.Marshal(agentMsg)
	h.hub.SendToAgent(session.AgentID, msgBytes)
}

const maxPtyInputSize = 64 * 1024

func (h *Handler) handlePtyInput(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	data := getString(payload, "data")

	if sessionID == "" || data == "" {
		return
	}

	session := h.hub.ptyManager.GetSession(sessionID)
	if session == nil || session.UserID != client.ID {
		return
	}

	if len(data) > maxPtyInputSize {
		log.Printf("[PTY] SECURITY: Dropped oversized PTY input from %s: %d bytes (max: %d)", client.ID, len(data), maxPtyInputSize)
		h.sendPtyError(client, sessionID, "input payload too large (max 64KB)")
		return
	}

	agentMsg := map[string]interface{}{
		"type": "pty",
		"payload": PtyPayload{
			PtyType:   "input",
			SessionID: sessionID,
			Data:      data,
		},
	}

	msgBytes, _ := json.Marshal(agentMsg)
	h.hub.SendToAgent(session.AgentID, msgBytes)
}

func (h *Handler) handlePtyStop(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	if sessionID == "" {
		return
	}

	session := h.hub.ptyManager.GetSession(sessionID)
	if session != nil && session.UserID == client.ID {
		agentMsg := map[string]interface{}{
			"type": "pty",
			"payload": PtyPayload{
				PtyType:   "stop",
				SessionID: sessionID,
			},
		}

		msgBytes, _ := json.Marshal(agentMsg)
		h.hub.SendToAgent(session.AgentID, msgBytes)
	}

	h.hub.ptyManager.RemoveSession(sessionID)
	h.sendPtyStopped(client, sessionID)
}

func (h *Handler) sendPtyError(client *Client, sessionID, errMsg string) {
	msg := map[string]interface{}{
		"type": "pty_error",
		"payload": map[string]interface{}{
			"session_id": sessionID,
			"error":      errMsg,
		},
	}
	client.WriteJSON(msg)
}

func (h *Handler) sendPtyStarted(client *Client, sessionID string) {
	msg := map[string]interface{}{
		"type": "pty_started",
		"payload": map[string]interface{}{
			"session_id": sessionID,
		},
	}
	client.WriteJSON(msg)
}

func (h *Handler) sendPtyStopped(client *Client, sessionID string) {
	msg := map[string]interface{}{
		"type": "pty_stopped",
		"payload": map[string]interface{}{
			"session_id": sessionID,
		},
	}
	client.WriteJSON(msg)
}

func (h *Handler) handlePtyOutput(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	data := getString(payload, "data")

	if sessionID == "" {
		return
	}

	session := h.hub.ptyManager.GetSession(sessionID)
	if session == nil {
		log.Printf("[PTY] Session not found: %s", sessionID)
		return
	}

	// Output comes from Agent, send it to Admin
	msg := map[string]interface{}{
		"type": "pty_output",
		"payload": map[string]interface{}{
			"session_id": sessionID,
			"data":       data,
		},
	}
	msgBytes, _ := json.Marshal(msg)
	h.hub.SendToAdmin(session.UserID, msgBytes)
}

func generateSecureID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b) + fmt.Sprintf("%d", time.Now().UnixNano())
}

func getFloat(m map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return defaultVal
}
