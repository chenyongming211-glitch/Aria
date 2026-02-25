package rpc

import "time"

// SocketPath is the Unix domain socket path for RPC communication
const SocketPath = "/var/run/aria.sock"

// Request types
const (
	MethodGetPeers    = "GetPeers"
	MethodGetRoutes   = "GetRoutes"
	MethodSyncConfig  = "SyncConfig"
	MethodGetStatus   = "GetStatus"
	MethodFlushConfig = "FlushConfig"
	MethodPing        = "Ping"
	MethodGetMetrics  = "GetMetrics"
)

// Message is the generic RPC message wrapper
type Message struct {
	Method  string      `json:"method"`
	Payload interface{} `json:"payload,omitempty"`
}

// Response is the generic RPC response wrapper
type Response struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ====================
// GetPeers
// ====================

// PeerInfo represents a peer's current status
type PeerInfo struct {
	Hostname       string    `json:"hostname"`
	Region         string    `json:"region,omitempty"`
	CustomerID     string    `json:"customer_id,omitempty"`
	PublicKey      string    `json:"public_key"`
	AssignedIP     string    `json:"assigned_ip"`
	Endpoint       string    `json:"endpoint"`
	LastHandshake  time.Time `json:"last_handshake"`
	RxBytes        uint64    `json:"rx_bytes"`
	TxBytes        uint64    `json:"tx_bytes"`
	Latency        float64   `json:"latency,omitempty"` // milliseconds
	Status         string    `json:"status"`            // "online", "offline", "never"
}

type GetPeersRequest struct{}

type GetPeersResponse struct {
	Peers      []PeerInfo `json:"peers"`
	TotalCount int        `json:"total_count"`
	ActiveCount int       `json:"active_count"`
}

// ====================
// GetRoutes
// ====================

type RouteInfo struct {
	Target   string `json:"target"`   // e.g., "192.168.1.0/24"
	Via      string `json:"via"`      // e.g., "node-bj-01" or "aria0"
	Metric   int    `json:"metric"`
	Status   string `json:"status"`   // "active", "inactive"
	Type     string `json:"type"`     // "local", "peer", "advertised"
	Hostname string `json:"hostname"` // peer hostname for peer/advertised routes
	Region   string `json:"region"`   // peer region
}

type GetRoutesRequest struct{}

type GetRoutesResponse struct {
	Routes []RouteInfo `json:"routes"`
}

// ====================
// SyncConfig
// ====================

type SyncConfigRequest struct{}

type SyncConfigResponse struct {
	PeersUpdated int    `json:"peers_updated"`
	RoutesUpdated int   `json:"routes_updated"`
	Duration     string `json:"duration"`
}

// ====================
// GetStatus
// ====================

type StatusInfo struct {
	Version       string    `json:"version"`
	DeviceID      string    `json:"device_id"`
	AssignedIP    string    `json:"assigned_ip"`
	Region        string    `json:"region,omitempty"`
	CustomerID    string    `json:"customer_id,omitempty"`
	Interface     string    `json:"interface"`
	ControllerURL string    `json:"controller_url"`
	Uptime        string    `json:"uptime"`
	StartTime     time.Time `json:"start_time"`
	PeerCount     int       `json:"peer_count"`
	Status        string    `json:"status"` // "running", "degraded", "error"
}

type GetStatusRequest struct{}

type GetStatusResponse struct {
	Status StatusInfo `json:"status"`
}

// ====================
// FlushConfig
// ====================

type FlushConfigRequest struct{}

type FlushConfigResponse struct {
	PeersRemoved  int `json:"peers_removed"`
	RoutesCleared int `json:"routes_cleared"`
}

// ====================
// Ping
// ====================

type PingRequest struct {
	Target string `json:"target"` // hostname or IP
	Count  int    `json:"count"`
	Interval float64 `json:"interval"` // seconds
}

type PingResponse struct {
	Output string `json:"output"`
	Success bool  `json:"success"`
}

// ====================
// GetMetrics
// ====================

type MetricsSummary struct {
	PeersTotal    int     `json:"peers_total"`
	PeersHealthy  int     `json:"peers_healthy"`
	AvgRTT        float64 `json:"avg_rtt_ms"`
	AvgLossRate   float64 `json:"avg_loss_rate"`
}

type PeerMetrics struct {
	PublicKey       string    `json:"public_key"`
	Endpoint        string    `json:"endpoint"`
	LastHandshake   time.Time `json:"last_handshake"`
	RTT             float64   `json:"rtt_ms"`
	LossRate        float64   `json:"loss_rate"`
	IsHealthy       bool      `json:"is_healthy"`
	RxBytes         uint64    `json:"rx_bytes"`
	TxBytes         uint64    `json:"tx_bytes"`
}

type SystemMetrics struct {
	CPUUsage    float64 `json:"cpu_usage_percent"`
	MemoryMB    float64 `json:"memory_mb"`
	Goroutines  int     `json:"goroutines"`
}

type GetMetricsRequest struct{}

type GetMetricsResponse struct {
	Summary     MetricsSummary  `json:"summary"`
	Peers       []PeerMetrics   `json:"peers,omitempty"`
	System      SystemMetrics   `json:"system"`
	Timestamp   time.Time       `json:"timestamp"`
}
