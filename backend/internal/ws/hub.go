package ws

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

const (
	BufferSize = 256
)

type ClientType string

const (
	ClientTypeAgent ClientType = "agent"
	ClientTypeAdmin ClientType = "admin"
)

type Client struct {
	ID   string
	Type ClientType
	Conn *websocket.Conn
	Send chan []byte
	Hub  *Hub
}

type Hub struct {
	clients    map[*Client]bool
	agents     map[string]*Client
	admins     map[string]*Client
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	ptyManager *PtyManager
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewHub() *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		clients:    make(map[*Client]bool),
		agents:     make(map[string]*Client),
		admins:     make(map[string]*Client),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		ptyManager: NewPtyManager(nil),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (h *Hub) Run() {
	defer func() {
		h.cancel()
		h.cleanupAllSessions()
		log.Printf("[WS] Hub shutdown complete")
	}()

	for {
		select {
		case <-h.ctx.Done():
			log.Printf("[WS] Hub received shutdown signal")
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			if client.Type == ClientTypeAgent {
				h.agents[client.ID] = client
				log.Printf("[WS] Agent registered in hub: %s (total: %d)", client.ID, len(h.agents))
			} else {
				h.admins[client.ID] = client
				log.Printf("[WS] Admin connected: %s (total: %d)", client.ID, len(h.admins))
			}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.Send != nil {
					close(client.Send)
				}
				if client.Type == ClientTypeAgent {
					delete(h.agents, client.ID)
					log.Printf("[WS] Agent disconnected: %s (remaining: %d)", client.ID, len(h.agents))
				} else {
					delete(h.admins, client.ID)
					log.Printf("[WS] Admin disconnected: %s (remaining: %d)", client.ID, len(h.admins))
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) RunWithContext(ctx context.Context) {
	go func() {
		<-ctx.Done()
		log.Printf("[WS] Hub context cancelled, initiating shutdown")
		h.Shutdown()
	}()
	h.Run()
}

func (h *Hub) Shutdown() {
	h.cancel()
}

func (h *Hub) cleanupAllSessions() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients {
		if client.Send != nil {
			close(client.Send)
		}
	}
	h.clients = make(map[*Client]bool)
	h.agents = make(map[string]*Client)
	h.admins = make(map[string]*Client)

	if h.ptyManager != nil {
		h.ptyManager.Cleanup()
	}
	log.Printf("[WS] All client connections and PTY sessions cleaned up")
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) SendToAgent(agentID string, message []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if client, ok := h.agents[agentID]; ok {
		select {
		case client.Send <- message:
			return true
		default:
			return false
		}
	}
	return false
}

func (h *Hub) SendToAdmin(adminID string, message []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if client, ok := h.admins[adminID]; ok {
		select {
		case client.Send <- message:
			return true
		default:
			return false
		}
	}
	return false
}

func (h *Hub) BroadcastToAgents(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.agents {
		select {
		case client.Send <- message:
		default:
		}
	}
}

func (h *Hub) GetAgentCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.agents)
}

func (h *Hub) GetAdminCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.admins)
}
