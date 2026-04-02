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

func (pm *PtyManager) CreateSession(agentID, userID, sessionID string, cols, rows int) *PtySession {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.sessions) >= pm.maxSessions {
		return nil
	}

	session := &PtySession{
		ID:      sessionID,
		AgentID: agentID,
		UserID:  userID,
		Cols:    cols,
		Rows:    rows,
	}

	pm.sessions[sessionID] = session
	return session
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

func (h *Handler) handlePtyMessage(client *Client, msg WSSMessage) {
	payload := msg.Payload

	ptyType := getString(payload, "pty_type")
	if ptyType == "" {
		log.Printf("[PTY] Missing pty_type in message")
		return
	}

	switch ptyType {
	case "start":
		h.handlePtyStart(client, payload)
	case "resize":
		h.handlePtyResize(client, payload)
	case "input":
		h.handlePtyInput(client, payload)
	case "stop":
		h.handlePtyStop(client, payload)
	default:
		log.Printf("[PTY] Unknown PTY type: %s", ptyType)
	}
}

func (h *Handler) handlePtyStart(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	if sessionID == "" {
		sessionID = generateSecureID()
	}

	cols := int(getFloat(payload, "cols", 80))
	rows := int(getFloat(payload, "rows", 24))
	shell := getString(payload, "shell")

	h.hub.ptyManager.CreateSession(client.ID, "", sessionID, cols, rows)

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
	if !h.hub.SendToAgent(client.ID, msgBytes) {
		h.sendPtyError(client, sessionID, "agent not connected")
		h.hub.ptyManager.RemoveSession(sessionID)
		return
	}

	h.sendPtyStarted(client, sessionID)
	log.Printf("[PTY] Session %s started for agent %s", sessionID, client.ID)
}

func (h *Handler) handlePtyResize(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	if sessionID == "" {
		return
	}

	cols := int(getFloat(payload, "cols", 80))
	rows := int(getFloat(payload, "rows", 24))

	session := h.hub.ptyManager.GetSession(sessionID)
	if session != nil && session.AgentID == client.ID {
		session.Cols = cols
		session.Rows = rows
	}

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
	h.hub.SendToAgent(client.ID, msgBytes)
}

func (h *Handler) handlePtyInput(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	data := getString(payload, "data")

	if sessionID == "" || data == "" {
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
	h.hub.SendToAgent(client.ID, msgBytes)
}

func (h *Handler) handlePtyStop(client *Client, payload map[string]interface{}) {
	sessionID := getString(payload, "session_id")
	if sessionID == "" {
		return
	}

	session := h.hub.ptyManager.GetSession(sessionID)
	if session != nil && session.AgentID == client.ID {
		agentMsg := map[string]interface{}{
			"type": "pty",
			"payload": PtyPayload{
				PtyType:   "stop",
				SessionID: sessionID,
			},
		}

		msgBytes, _ := json.Marshal(agentMsg)
		h.hub.SendToAgent(client.ID, msgBytes)
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

	msg := map[string]interface{}{
		"type": "pty_output",
		"payload": map[string]interface{}{
			"session_id": sessionID,
			"data":       data,
		},
	}
	client.WriteJSON(msg)
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
