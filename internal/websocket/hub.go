// Package websocket provides WebSocket support for real-time scan progress updates
package websocket

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Message represents a WebSocket message
type Message struct {
	Type    string      `json:"type"`    // "progress", "issue", "complete", "error", "step"
	ScanID  string      `json:"scan_id"`
	Data    interface{} `json:"data"`
}

// Hub maintains active client connections
type Hub struct {
	clients    map[string]*Client
	clientsMux sync.RWMutex
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Message
	upgrader   websocket.Upgrader
}

// Client represents a WebSocket client
type Client struct {
	ID     string
	ScanID string
	Conn   *websocket.Conn
	Send   chan *Message
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	h := &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in development
			},
		},
	}
	go h.run()
	return h
}

// run processes hub events
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clientsMux.Lock()
			h.clients[client.ID] = client
			h.clientsMux.Unlock()
			log.Printf("[WebSocket] Client registered: %s for scan: %s", client.ID, client.ScanID)

		case client := <-h.unregister:
			h.clientsMux.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				client.Conn.Close()
			}
			h.clientsMux.Unlock()
			log.Printf("[WebSocket] Client unregistered: %s", client.ID)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// broadcastMessage sends a message to clients subscribed to a specific scan
func (h *Hub) broadcastMessage(message *Message) {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	for _, client := range h.clients {
		// Send to clients listening for this scan
		if client.ScanID == message.ScanID {
			if err := client.Conn.WriteJSON(message); err != nil {
				log.Printf("[WebSocket] Error sending to client %s: %v", client.ID, err)
				client.Conn.Close()
				h.unregister <- client
			}
		}
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket
func (h *Hub) HandleWebSocket(c *gin.Context) {
	scanID := c.Param("id")
	if scanID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scan_id required"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
		return
	}

	clientID := uuid.New().String()
	client := &Client{
		ID:     clientID,
		ScanID: scanID,
		Conn:   conn,
		Send:   make(chan *Message, 256),
	}

	h.register <- client

	// Read messages from client (keepalive)
	go func() {
		defer func() {
			h.unregister <- client
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()

	// Send initial connection message
	conn.WriteJSON(Message{
		Type:   "connected",
		ScanID: scanID,
		Data: map[string]interface{}{
			"message": "Connected to scan progress updates",
		},
	})
}

// BroadcastProgress sends a progress update to all connected clients
func (h *Hub) BroadcastProgress(scanID string, progress float64, message string, data map[string]interface{}) {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["progress"] = progress
	data["message"] = message

	h.broadcast <- &Message{
		Type:   "progress",
		ScanID: scanID,
		Data:   data,
	}
}

// BroadcastIssue sends a newly found issue to all connected clients
func (h *Hub) BroadcastIssue(scanID string, issue interface{}) {
	h.broadcast <- &Message{
		Type:   "issue",
		ScanID: scanID,
		Data:   issue,
	}
}

// BroadcastComplete sends completion message
func (h *Hub) BroadcastComplete(scanID string, result interface{}) {
	h.broadcast <- &Message{
		Type:   "complete",
		ScanID: scanID,
		Data:   result,
	}
}

// BroadcastError sends an error message
func (h *Hub) BroadcastError(scanID string, errMsg string) {
	h.broadcast <- &Message{
		Type:   "error",
		ScanID: scanID,
		Data: map[string]interface{}{
			"error": errMsg,
		},
	}
}

// BroadcastBatchStart sends batch start notification
func (h *Hub) BroadcastBatchStart(scanID string, batchID int, files []string) {
	h.broadcast <- &Message{
		Type:   "batch_start",
		ScanID: scanID,
		Data: map[string]interface{}{
			"batch_id": batchID,
			"files":    files,
			"message":  fmt.Sprintf("开始扫描批次 %d (%d 个文件)", batchID, len(files)),
		},
	}
}

// BroadcastBatchComplete sends batch completion notification
func (h *Hub) BroadcastBatchComplete(scanID string, batchID int, issuesFound int) {
	h.broadcast <- &Message{
		Type:   "batch_complete",
		ScanID: scanID,
		Data: map[string]interface{}{
			"batch_id":      batchID,
			"issues_found":  issuesFound,
			"message":       fmt.Sprintf("批次 %d 完成，发现 %d 个问题", batchID, issuesFound),
		},
	}
}

// BroadcastScanStart sends scan start notification
func (h *Hub) BroadcastScanStart(scanID string, totalFiles int, totalBatches int) {
	h.broadcast <- &Message{
		Type:   "scan_start",
		ScanID: scanID,
		Data: map[string]interface{}{
			"total_files":  totalFiles,
			"total_batches": totalBatches,
			"message":      fmt.Sprintf("开始扫描，共 %d 个文件，分 %d 批处理", totalFiles, totalBatches),
		},
	}
}

// BroadcastStep sends a step update during scanning
func (h *Hub) BroadcastStep(scanID string, step string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["step"] = step

	h.broadcast <- &Message{
		Type:   "step",
		ScanID: scanID,
		Data:   details,
	}
}

// BroadcastFileScan sends file scanning progress
func (h *Hub) BroadcastFileScan(scanID string, fileName string, current int, total int) {
	h.broadcast <- &Message{
		Type:   "file_scan",
		ScanID: scanID,
		Data: map[string]interface{}{
			"file":     fileName,
			"current":  current,
			"total":    total,
			"message":  fmt.Sprintf("正在扫描: %s (%d/%d)", fileName, current, total),
		},
	}
}

// BroadcastClaudeResponse sends Claude analysis response
func (h *Hub) BroadcastClaudeResponse(scanID string, batchID int, response string) {
	h.broadcast <- &Message{
		Type:   "claude_response",
		ScanID: scanID,
		Data: map[string]interface{}{
			"batch_id": batchID,
			"response": response,
		},
	}
}

// GetConnectedCount returns the number of connected clients for a scan
func (h *Hub) GetConnectedCount(scanID string) int {
	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()
	count := 0
	for _, client := range h.clients {
		if client.ScanID == scanID {
			count++
		}
	}
	return count
}
