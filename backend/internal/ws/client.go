package ws

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 4 * 1024 * 1024
)

func NewClient(id string, clientType ClientType, conn *websocket.Conn, hub *Hub) *Client {
	return &Client{
		ID:   id,
		Type: clientType,
		Conn: conn,
		Send: make(chan []byte, BufferSize),
		Hub:  hub,
	}
}

func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		log.Printf("[WS] Pong received from %s %s, resetting deadline", c.Type, c.ID)
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	log.Printf("[WS] readPump started for %s %s", c.Type, c.ID)

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WS] Read error for %s %s: %v", c.Type, c.ID, err)
			} else {
				log.Printf("[WS] Connection closed for %s %s: %v", c.Type, c.ID, err)
			}
			break
		}

		log.Printf("[WS] Message from %s %s: %s", c.Type, c.ID, message)
	}

	log.Printf("[WS] readPump ended for %s %s", c.Type, c.ID)
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if c.Conn != nil {
			c.Conn.Close()
		}
	}()

	log.Printf("[WS] writePump started for %s %s", c.Type, c.ID)

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				log.Printf("[WS] Send channel closed for %s %s", c.Type, c.ID)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Printf("[WS] NextWriter error for %s %s: %v", c.Type, c.ID, err)
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				log.Printf("[WS] Write close error for %s %s: %v", c.Type, c.ID, err)
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[WS] Ping write error for %s %s: %v", c.Type, c.ID, err)
				return
			}
			log.Printf("[WS] Ping sent to %s %s", c.Type, c.ID)
		}
	}
}

func (c *Client) WriteJSON(v interface{}) error {
	c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.Conn.WriteJSON(v)
}
