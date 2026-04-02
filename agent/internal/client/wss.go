package client

import (
	"encoding/json"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/enterprise-rat/agent/internal/config"
	"github.com/enterprise-rat/agent/internal/executor"
	"github.com/enterprise-rat/agent/internal/models"
	"github.com/gorilla/websocket"
)

const (
	maxRetries        = 10
	pingInterval      = 25 * time.Second
	pongWait          = 40 * time.Second
	maxMessageSize    = 4 * 1024 * 1024
	reconnectDelay    = 5 * time.Second
	maxReconnectDelay = 60 * time.Second
)

type WSSClient struct {
	serverURL        string
	enrollmentSecret string
	identity         *config.Identity
	token            string
	conn             *websocket.Conn
	done             chan struct{}
	shouldStop       bool
	mu               sync.Mutex
	ptyHandler       *executor.PtyHandler
	fileManager      *executor.FileManager
}

func NewWSSClient(serverURL, token, enrollmentSecret string, identity *config.Identity) *WSSClient {
	return &WSSClient{
		serverURL:        serverURL,
		enrollmentSecret: enrollmentSecret,
		token:            token,
		identity:         identity,
		done:             make(chan struct{}),
		ptyHandler:       executor.NewPtyHandler(),
		fileManager:      executor.NewFileManager(),
	}
}

func (c *WSSClient) Connect() error {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return err
	}

	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	header := make(map[string][]string)
	if c.token != "" {
		header["Authorization"] = []string{"Bearer " + c.token}
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return err
	}

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait + 5*time.Second))
	conn.SetPongHandler(func(string) error {
		log.Printf("[WSS] Pong received, resetting deadline")
		conn.SetReadDeadline(time.Now().Add(pongWait + 5*time.Second))
		return nil
	})

	c.conn = conn
	c.done = make(chan struct{})

	log.Printf("[WSS] Connected to %s", c.serverURL)

	if err := c.sendHandshake(); err != nil {
		conn.Close()
		return err
	}

	go c.readPump()
	go c.writePump()

	return nil
}

func (c *WSSClient) sendHandshake() error {
	header := make(map[string][]string)
	if c.token != "" {
		header["Authorization"] = []string{"Bearer " + c.token}
	}
	if c.enrollmentSecret != "" {
		header["X-Agent-Enrollment-Secret"] = []string{c.enrollmentSecret}
	}

	return c.conn.WriteJSON(map[string]interface{}{
		"type": "agent_register",
		"payload": map[string]interface{}{
			"agent_id":   c.identity.AgentID,
			"hostname":   c.identity.Hostname,
			"os_family":  c.identity.OSFamily,
			"os_version": c.identity.OSVersion,
		},
	})
}

func (c *WSSClient) readPump() {
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			log.Printf("[WSS] readPump closing connection")
			c.conn.Close()
			c.conn = nil
		}
		close(c.done)
	}()

	for {
		if c.shouldStop {
			log.Println("[WSS] readPump stopping (shouldStop=true)")
			return
		}

		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WSS] WS Read Error (unexpected): %v", err)
			} else if err.Error() == "websocket: close 1000 (normal)" {
				log.Printf("[WSS] Connection closed normally by server")
			} else {
				log.Printf("[WSS] WS Connection Closed: %v", err)
			}
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[WSS] Invalid JSON message: %v, raw: %s", err, string(message))
			continue
		}

		log.Printf("[WSS] Raw message received: %s", string(message))
		c.handleMessage(msg)
	}
}

func (c *WSSClient) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
	}()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("[WSS] Ping failed: %v", err)
				} else {
					log.Printf("[WSS] Ping sent")
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *WSSClient) handleMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok || msgType == "" {
		log.Printf("[WSS] Message missing or empty 'type' field: %+v", msg)
		return
	}

	log.Printf("[WSS] Handling message type: '%s'", msgType)

	switch msgType {
	case "command":
		go c.executeCommand(msg)
	case "pty":
		go c.handlePty(msg)
	case "file_manager":
		go c.handleFileManager(msg)
	default:
		log.Printf("[WSS] Unknown message type: '%s'", msgType)
	}
}

func (c *WSSClient) handlePty(msg map[string]interface{}) {
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		log.Printf("[PTY] Invalid payload")
		return
	}

	resp, shouldSend := c.ptyHandler.HandlePtyCommand(payload)
	if resp != nil && shouldSend {
		c.sendPtyResponse(resp, payload)
	}
}

func (c *WSSClient) sendPtyResponse(resp *models.CommandResponse, payload map[string]interface{}) {
	sessionID, _ := payload["session_id"].(string)

	output, _ := c.ptyHandler.PollSessionOutput(sessionID)

	msg := map[string]interface{}{
		"type": "pty_output",
		"payload": map[string]interface{}{
			"session_id": sessionID,
			"data":       output,
		},
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.WriteJSON(msg); err != nil {
			log.Printf("[PTY] Failed to send PTY output: %v", err)
		}
	}
}

func (c *WSSClient) handleFileManager(msg map[string]interface{}) {
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		log.Printf("[FILE] Invalid payload")
		return
	}

	result := c.fileManager.HandleFileOperation(payload)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		response := map[string]interface{}{
			"type":    "file_result",
			"payload": result,
		}
		if err := c.conn.WriteJSON(response); err != nil {
			log.Printf("[FILE] Failed to send result: %v", err)
		} else {
			log.Printf("[FILE] Operation completed: %s", result.RequestID)
		}
	}
}

func (c *WSSClient) executeCommand(msg map[string]interface{}) {
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		log.Println("[EXEC] Invalid command payload")
		c.sendErrorResult("", "invalid command payload")
		return
	}

	executable, _ := payload["executable"].(string)
	argsRaw, _ := payload["args"].([]interface{})
	commandID, _ := payload["command_id"].(string)

	if executable == "" {
		log.Println("[EXEC] Missing executable in command")
		c.sendErrorResult(commandID, "missing executable")
		return
	}

	var args []string
	if argsRaw != nil {
		for _, a := range argsRaw {
			if s, ok := a.(string); ok {
				args = append(args, s)
			}
		}
	}

	timeout := 300
	if t, ok := payload["timeout_seconds"].(float64); ok {
		timeout = int(t)
	}

	req := &models.CommandRequest{
		CommandID:      commandID,
		Executable:     executable,
		Args:           args,
		TimeoutSeconds: timeout,
	}

	log.Printf("[EXEC] Executing command_id=%s: %s %v (timeout=%ds)", commandID, req.Executable, req.Args, timeout)

	resp := executor.ExecuteCommand(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		response := map[string]interface{}{
			"type":    "command_result",
			"payload": resp,
		}
		if err := c.conn.WriteJSON(response); err != nil {
			log.Printf("[EXEC] Failed to send response: %v", err)
		} else {
			log.Printf("[EXEC] Command %s completed with exit code %d", req.CommandID, resp.ExitCode)
		}
	}
}

func (c *WSSClient) sendErrorResult(commandID, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		resp := &models.CommandResponse{
			CommandID: commandID,
			ExitCode:  -1,
			ErrorMsg:  errMsg,
		}
		response := map[string]interface{}{
			"type":    "command_result",
			"payload": resp,
		}
		if err := c.conn.WriteJSON(response); err != nil {
			log.Printf("[EXEC] Failed to send error result: %v", err)
		} else {
			log.Printf("[EXEC] Error result sent for command %s: %s", commandID, errMsg)
		}
	}
}

func (c *WSSClient) Run() {
	retryCount := 0
	reconnectDelay := reconnectDelay

	for {
		if c.shouldStop {
			log.Println("[WSS] Client stopping (shouldStop=true)")
			return
		}

		if err := c.Connect(); err != nil {
			retryCount++
			if retryCount > maxRetries {
				log.Printf("[WSS] Max retries (%d) reached, continuing with unlimited retries", maxRetries)
			}
			log.Printf("[WSS] Connection failed (attempt %d): %v. Retrying in %v...", retryCount, err, reconnectDelay)
			time.Sleep(reconnectDelay)
			reconnectDelay = reconnectDelay * 2
			if reconnectDelay > maxReconnectDelay {
				reconnectDelay = maxReconnectDelay
			}
		} else {
			log.Println("[WSS] Connected successfully, keeping connection alive...")
			retryCount = 0
			reconnectDelay = reconnectDelay
			<-c.done
			log.Println("[WSS] Connection ended, will reconnect...")
			time.Sleep(1 * time.Second)
		}
	}
}

func (c *WSSClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.shouldStop = true

	if c.conn != nil {
		c.conn.Close()
	}
}
