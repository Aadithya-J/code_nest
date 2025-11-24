package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now (dev mode)
	},
}

type Hub struct {
	// Map of sessionID to list of connections
	clients map[string][]*websocket.Conn
	mu      sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string][]*websocket.Conn),
	}
}

func (h *Hub) HandleConnection(c *gin.Context) {
	sessionID := c.Query("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId required"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ Failed to upgrade connection: %v", err)
		return
	}

	h.register(sessionID, conn)
	defer h.unregister(sessionID, conn)

	// Keep connection alive and handle close
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *Hub) register(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[sessionID] = append(h.clients[sessionID], conn)
	log.Printf("🔌 Client connected to session %s", sessionID)
}

func (h *Hub) unregister(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.clients[sessionID]
	for i, c := range conns {
		if c == conn {
			h.clients[sessionID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	if len(h.clients[sessionID]) == 0 {
		delete(h.clients, sessionID)
	}
	conn.Close()
	log.Printf("🔌 Client disconnected from session %s", sessionID)
}

// HandleStatusUpdate implements rabbitmq.StatusHandler
func (h *Hub) HandleStatusUpdate(sessionID, status, message string) {
	h.mu.RLock()
	conns := h.clients[sessionID]
	h.mu.RUnlock()

	if len(conns) == 0 {
		return
	}

	update := map[string]string{
		"type":    "status_update",
		"status":  status,
		"message": message,
	}

	data, err := json.Marshal(update)
	if err != nil {
		log.Printf("❌ Failed to marshal status update: %v", err)
		return
	}

	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("❌ Failed to send status update: %v", err)
		}
	}
}
