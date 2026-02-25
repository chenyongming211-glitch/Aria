package rpc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// Server is the RPC server that handles CLI requests
type Server struct {
	listener net.Listener
	handlers map[string]HandlerFunc
	mu       sync.RWMutex
	running  bool
	daemon   DaemonInterface // Interface to interact with daemon
}

// HandlerFunc is the function signature for RPC handlers
type HandlerFunc func(payload []byte) (interface{}, error)

// DaemonInterface defines the methods that daemon must implement
type DaemonInterface interface {
	GetPeers() (*GetPeersResponse, error)
	GetRoutes() (*GetRoutesResponse, error)
	SyncConfig() (*SyncConfigResponse, error)
	GetStatus() (*GetStatusResponse, error)
	FlushConfig() (*FlushConfigResponse, error)
	Ping(target string, count int, interval float64) (*PingResponse, error)
	GetMetrics() (*GetMetricsResponse, error)
}

// NewServer creates a new RPC server
func NewServer(daemon DaemonInterface) *Server {
	return &Server{
		handlers: make(map[string]HandlerFunc),
		daemon:   daemon,
	}
}

// Start starts the RPC server
func (s *Server) Start() error {
	// Remove existing socket file if exists
	os.Remove(SocketPath)

	// Create Unix domain socket
	listener, err := net.Listen("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}

	// Set socket permissions (only root can access)
	if err := os.Chmod(SocketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	s.listener = listener
	s.running = true

	// Register handlers
	s.registerHandlers()

	// Start accepting connections
	go s.acceptLoop()

	return nil
}

// Stop stops the RPC server
func (s *Server) Stop() error {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if s.listener != nil {
		s.listener.Close()
		os.Remove(SocketPath)
	}

	return nil
}

// registerHandlers registers all RPC method handlers
func (s *Server) registerHandlers() {
	s.handlers[MethodGetPeers] = s.handleGetPeers
	s.handlers[MethodGetRoutes] = s.handleGetRoutes
	s.handlers[MethodSyncConfig] = s.handleSyncConfig
	s.handlers[MethodGetStatus] = s.handleGetStatus
	s.handlers[MethodFlushConfig] = s.handleFlushConfig
	s.handlers[MethodPing] = s.handlePing
	s.handlers[MethodGetMetrics] = s.handleGetMetrics
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	for {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()

		if !running {
			return
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				// Only log if we're still supposed to be running
				fmt.Printf("RPC accept error: %v\n", err)
			}
			return
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a single client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Read request
	decoder := json.NewDecoder(conn)
	var msg Message
	if err := decoder.Decode(&msg); err != nil {
		s.sendError(conn, fmt.Errorf("failed to decode request: %w", err))
		return
	}

	// Find handler
	handler, exists := s.handlers[msg.Method]
	if !exists {
		s.sendError(conn, fmt.Errorf("unknown method: %s", msg.Method))
		return
	}

	// Marshal payload back to JSON for handler
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		s.sendError(conn, fmt.Errorf("failed to marshal payload: %w", err))
		return
	}

	// Execute handler
	result, err := handler(payloadBytes)
	if err != nil {
		s.sendError(conn, err)
		return
	}

	// Send success response
	s.sendSuccess(conn, result)
}

// sendSuccess sends a success response
func (s *Server) sendSuccess(conn net.Conn, data interface{}) {
	resp := Response{
		Success: true,
		Data:    data,
	}
	json.NewEncoder(conn).Encode(resp)
}

// sendError sends an error response
func (s *Server) sendError(conn net.Conn, err error) {
	resp := Response{
		Success: false,
		Error:   err.Error(),
	}
	json.NewEncoder(conn).Encode(resp)
}

// Handler implementations

func (s *Server) handleGetPeers(payload []byte) (interface{}, error) {
	return s.daemon.GetPeers()
}

func (s *Server) handleGetRoutes(payload []byte) (interface{}, error) {
	return s.daemon.GetRoutes()
}

func (s *Server) handleSyncConfig(payload []byte) (interface{}, error) {
	return s.daemon.SyncConfig()
}

func (s *Server) handleGetStatus(payload []byte) (interface{}, error) {
	return s.daemon.GetStatus()
}

func (s *Server) handleFlushConfig(payload []byte) (interface{}, error) {
	return s.daemon.FlushConfig()
}

func (s *Server) handlePing(payload []byte) (interface{}, error) {
	var req PingRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid ping request: %w", err)
	}
	return s.daemon.Ping(req.Target, req.Count, req.Interval)
}

func (s *Server) handleGetMetrics(payload []byte) (interface{}, error) {
	return s.daemon.GetMetrics()
}
