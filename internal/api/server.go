package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"opcuababy/internal/controller"
	"opcuababy/internal/opc"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub
	// The websocket connection.
	conn *websocket.Conn
	// Buffered channel of outbound messages.
	send chan *controller.WatchItem
	// A map of nodeIDs the client is subscribed to.
	subscriptions map[string]bool
	mu            sync.RWMutex
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan *controller.WatchItem
	register   chan *Client
	unregister chan *Client
	controller controller.NodeManager
	mu         sync.Mutex
	stop       chan struct{}
}

func newHub(ctrl controller.NodeManager) *Hub {
	return &Hub{
		broadcast:  ctrl.GetApiBroadcastChan(),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		controller: ctrl,
		stop:       make(chan struct{}),
	}
}

func (h *Hub) run(context.Context) {
	for {
		// Watch controller client context to close clients on OPC UA disconnect
		var ctrlDone <-chan struct{}
		if cctx := h.controller.GetClientContext(); cctx != nil {
			ctrlDone = cctx.Done()
		}
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case <-ctrlDone:
			// OPC UA client context cancelled; close all clients and reset broadcast
			log.Println("Hub: OPC UA client context done, closing websocket clients.")
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			h.broadcast = h.controller.GetApiBroadcastChan()
			continue
		case message, ok := <-h.broadcast:
			if !ok {
				// The broadcast channel was closed by the controller, indicating a disconnect.
				// Close all client connections.
				log.Println("Hub: Controller disconnected, closing all websocket clients.")
				h.mu.Lock()
				for client := range h.clients {
					close(client.send)
					delete(h.clients, client)
				}
				h.mu.Unlock()
				h.broadcast = h.controller.GetApiBroadcastChan()

				// The Hub continues to run, ready for a new connection cycle.
				// It will accept new clients once the controller reconnects and
				// re-creates the broadcast channel.
				continue // Continue the loop to accept new registrations etc.
			}

			h.mu.Lock()
			for client := range h.clients {
				client.mu.RLock()
				if client.subscriptions[message.NodeID] {
					select {
					case client.send <- message:
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
				client.mu.RUnlock()
			}
			h.mu.Unlock()
		case <-h.stop:
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return
		}
	}
}

// WebSocketMessage defines the structure for messages between client and server.
type WebSocketMessage struct {
	Action  string   `json:"action"` // "subscribe" or "unsubscribe"
	NodeIDs []string `json:"node_ids"`
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		var msg WebSocketMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		c.mu.Lock()
		switch msg.Action {
		case "subscribe":
			for _, nodeID := range msg.NodeIDs {
				c.subscriptions[nodeID] = true
				go c.hub.controller.AddWatch(nodeID)
			}
		case "unsubscribe":
			for _, nodeID := range msg.NodeIDs {
				delete(c.subscriptions, nodeID)
			}
		}
		c.mu.Unlock()
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for message := range c.send {
		if err := c.conn.WriteJSON(message); err != nil {
			log.Printf("error writing json: %v", err)
			return
		}
	}
	c.conn.WriteMessage(websocket.CloseMessage, []byte{})
}

// StartServer initializes and starts the API server. It returns the http.Server instance.
func StartServer(ctx context.Context, ctrl controller.NodeManager, apiStatus *string, cfg *opc.Config) *http.Server {
	hub := newHub(ctrl)
	go hub.run(ctx)
	router := gin.Default()

	// REST API endpoints
	api := router.Group("/api/v1")
	{
		api.POST("/read", func(c *gin.Context) {
			controllerCtx := hub.controller.GetClientContext()
			if controllerCtx == nil || controllerCtx.Err() != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OPC UA connection is not active"})
				return
			}

			var req struct {
				NodeID string `json:"node_id" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			attrs, err := ctrl.ReadNodeAttributes(req.NodeID)
			if err != nil {
				status := http.StatusInternalServerError
				if strings.Contains(err.Error(), "not connected") {
					status = http.StatusServiceUnavailable
				}
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, attrs)
		})

		api.POST("/write", func(c *gin.Context) {
			controllerCtx := hub.controller.GetClientContext()
			if controllerCtx == nil || controllerCtx.Err() != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OPC UA connection is not active"})
				return
			}

			var req struct {
				NodeID   string `json:"node_id" binding:"required"`
				DataType string `json:"data_type" binding:"required"`
				Value    string `json:"value" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			ctrl.WriteValue(req.NodeID, req.DataType, req.Value)
			c.JSON(http.StatusOK, gin.H{"status": "write request sent"})
		})
	}

	// WebSocket endpoint
	router.GET("/ws/subscribe", func(c *gin.Context) {
		controllerCtx := hub.controller.GetClientContext()
		if controllerCtx == nil || controllerCtx.Err() != nil {
			// controllerCtx is nil (never connected) or its .Done() channel is closed (disconnected).
			c.String(http.StatusServiceUnavailable, "OPC UA connection is not active.")
			return
		}
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("Failed to set websocket upgrade: %+v", err)
			return
		}
		client := &Client{
			hub:           hub,
			conn:          conn,
			send:          make(chan *controller.WatchItem, 256),
			subscriptions: make(map[string]bool),
		}
		client.hub.register <- client

		go client.writePump()
		go client.readPump()
	})

	// Documentation and client info
	router.GET("/", func(c *gin.Context) {
		data, err := webTemplate.ReadFile("templates/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Error reading index page")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	router.GET("/doc", func(c *gin.Context) {
		data, err := webTemplate.ReadFile("templates/doc.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Error reading documentation")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	router.GET("/api/v1/ws/clients", func(c *gin.Context) {
		hub.mu.Lock()
		defer hub.mu.Unlock()

		type clientInfo struct {
			RemoteAddr    string   `json:"remote_addr"`
			Subscriptions []string `json:"subscriptions"`
		}
		var clientsData []clientInfo

		for client := range hub.clients {
			client.mu.RLock()
			subs := make([]string, 0, len(client.subscriptions))
			for sub := range client.subscriptions {
				subs = append(subs, sub)
			}
			clientsData = append(clientsData, clientInfo{
				RemoteAddr:    client.conn.RemoteAddr().String(),
				Subscriptions: subs,
			})
			client.mu.RUnlock()
		}
		c.JSON(http.StatusOK, clientsData)
	})

	port := cfg.ApiPort
	if port == "" {
		port = "8080" // Default port
	}
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		*apiStatus = "Running on :" + port
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			*apiStatus = "Error: " + err.Error()
			log.Printf("listen: %s\n", err)
		}
	}()

	go func() {
		<-ctx.Done()
		close(hub.stop)
		*apiStatus = "API Server Stopped"
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server Shutdown Failed:%+v", err)
		}
	}()

	return srv
}
