package datapath

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const (
	ebpfAgentSocketPath = "/run/aria-agent.sock"
	ebpfAgentTimeout    = 5 * time.Second
)

type ebpfRequest struct {
	Cmd  string                 `json:"cmd"`
	Args map[string]interface{} `json:"args"`
}

type ebpfResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type EbpfFirewallManager struct {
	socketPath string
	enabled    bool
}

func NewEbpfFirewallManager() *EbpfFirewallManager {
	return &EbpfFirewallManager{
		socketPath: ebpfAgentSocketPath,
		enabled:    false,
	}
}

func NewEbpfFirewallManagerWithPath(socketPath string) *EbpfFirewallManager {
	return &EbpfFirewallManager{
		socketPath: socketPath,
		enabled:    false,
	}
}

func (m *EbpfFirewallManager) call(cmd string, args map[string]interface{}) (*ebpfResponse, error) {
	conn, err := net.DialTimeout("unix", m.socketPath, ebpfAgentTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to eBPF agent at %s: %w (is aria-agent daemon running?)", m.socketPath, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(ebpfAgentTimeout))

	req := ebpfRequest{
		Cmd:  cmd,
		Args: args,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	_, err = conn.Write(append(reqJSON, '\n'))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp ebpfResponse
	if err := json.Unmarshal([]byte(respLine), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

func (m *EbpfFirewallManager) Init() error {
	resp, err := m.call("ping", nil)
	if err != nil {
		return fmt.Errorf("eBPF agent not available: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("eBPF agent ping failed: %s", resp.Message)
	}
	m.enabled = true
	return nil
}

func (m *EbpfFirewallManager) Cleanup() error {
	m.enabled = false
	return nil
}

func (m *EbpfFirewallManager) ApplyPolicy(rules []ACLRule) error {
	for _, rule := range rules {
		args := map[string]interface{}{
			"src_ip":   rule.SrcNet,
			"dst_ip":   rule.DstNet,
			"dst_port": rule.MinPort,
			"protocol": rule.Protocol,
		}

		resp, err := m.call("acl_allow", args)
		if err != nil {
			return fmt.Errorf("failed to apply ACL rule: %w", err)
		}
		if !resp.Success {
			return fmt.Errorf("failed to apply ACL rule: %s", resp.Message)
		}
	}
	return nil
}

func (m *EbpfFirewallManager) GetStats() (*FirewallStats, error) {
	return &FirewallStats{
		Enabled: m.enabled,
	}, nil
}

func (m *EbpfFirewallManager) IsEnabled() bool {
	return m.enabled
}

func (m *EbpfFirewallManager) BlockIP(ip string) error {
	resp, err := m.call("acl_block_src_ip", map[string]interface{}{"ip": ip})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to block IP: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) LimitIP(ip string, mbps int) error {
	resp, err := m.call("qos_limit_ip", map[string]interface{}{
		"ip":   ip,
		"mbps": mbps,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to limit IP: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) LimitPeerPair(srcIP, dstIP string, mbps int) error {
	resp, err := m.call("qos_limit_peer", map[string]interface{}{
		"src_ip": srcIP,
		"dst_ip": dstIP,
		"mbps":   mbps,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to limit peer pair: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) UnblockIP(ip string) error {
	resp, err := m.call("acl_unblock_src_ip", map[string]interface{}{"ip": ip})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to unblock IP: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) DenyRule(srcNet, dstNet string, dstPort uint16, protocol uint8) error {
	resp, err := m.call("acl_deny", map[string]interface{}{
		"src_ip":   srcNet,
		"dst_ip":   dstNet,
		"dst_port": dstPort,
		"protocol": protocol,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to add deny rule: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) RemoveRule(srcNet, dstNet string, dstPort uint16, protocol uint8) error {
	resp, err := m.call("acl_remove_rule", map[string]interface{}{
		"src_ip":   srcNet,
		"dst_ip":   dstNet,
		"dst_port": dstPort,
		"protocol": protocol,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to remove rule: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) RemoveIPLimit(ip string) error {
	resp, err := m.call("qos_remove_ip", map[string]interface{}{"ip": ip})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to remove IP limit: %s", resp.Message)
	}
	return nil
}

func (m *EbpfFirewallManager) RemovePeerLimit(srcIP, dstIP string) error {
	resp, err := m.call("qos_remove_peer", map[string]interface{}{
		"src_ip": srcIP,
		"dst_ip": dstIP,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("failed to remove peer limit: %s", resp.Message)
	}
	return nil
}
