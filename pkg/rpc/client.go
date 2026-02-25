package rpc

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client is the RPC client for CLI commands
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient creates a new RPC client
func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = SocketPath
	}
	return &Client{
		socketPath: socketPath,
		timeout:    10 * time.Second,
	}
}

// SetTimeout sets the client timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// call makes an RPC call
func (c *Client) call(method string, payload interface{}) (*Response, error) {
	// Connect to socket
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w (is aria running?)", err)
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(c.timeout))

	// Send request
	msg := Message{
		Method:  method,
		Payload: payload,
	}
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error
	if !resp.Success {
		return nil, fmt.Errorf("RPC error: %s", resp.Error)
	}

	return &resp, nil
}

// GetPeers retrieves current peer information
func (c *Client) GetPeers() (*GetPeersResponse, error) {
	resp, err := c.call(MethodGetPeers, GetPeersRequest{})
	if err != nil {
		return nil, err
	}

	// Convert response data to GetPeersResponse
	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result GetPeersResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal peers response: %w", err)
	}

	return &result, nil
}

// GetRoutes retrieves routing table information
func (c *Client) GetRoutes() (*GetRoutesResponse, error) {
	resp, err := c.call(MethodGetRoutes, GetRoutesRequest{})
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result GetRoutesResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal routes response: %w", err)
	}

	return &result, nil
}

// SyncConfig triggers immediate configuration sync
func (c *Client) SyncConfig() (*SyncConfigResponse, error) {
	resp, err := c.call(MethodSyncConfig, SyncConfigRequest{})
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result SyncConfigResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync response: %w", err)
	}

	return &result, nil
}

// GetStatus retrieves daemon status
func (c *Client) GetStatus() (*GetStatusResponse, error) {
	resp, err := c.call(MethodGetStatus, GetStatusRequest{})
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result GetStatusResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status response: %w", err)
	}

	return &result, nil
}

// FlushConfig clears all peers and routes
func (c *Client) FlushConfig() (*FlushConfigResponse, error) {
	resp, err := c.call(MethodFlushConfig, FlushConfigRequest{})
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result FlushConfigResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flush response: %w", err)
	}

	return &result, nil
}

// Ping performs a ping test through the VPN tunnel
func (c *Client) Ping(target string, count int, interval float64) (*PingResponse, error) {
	req := PingRequest{
		Target:   target,
		Count:    count,
		Interval: interval,
	}

	resp, err := c.call(MethodPing, req)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result PingResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ping response: %w", err)
	}

	return &result, nil
}

// IsServerRunning checks if the RPC server is accessible
func (c *Client) IsServerRunning() bool {
	conn, err := net.DialTimeout("unix", c.socketPath, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// GetMetrics retrieves current monitoring metrics
func (c *Client) GetMetrics() (*GetMetricsResponse, error) {
	resp, err := c.call(MethodGetMetrics, GetMetricsRequest{})
	if err != nil {
		return nil, err
	}

	// Convert response data to GetMetricsResponse
	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	var result GetMetricsResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics response: %w", err)
	}

	return &result, nil
}
