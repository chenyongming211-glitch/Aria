//go:build linux

package datapath

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/nftables"
)

//go:embed nftables-init.nft
var nftInitScriptTemplate string

// NftablesFirewallManager implements FirewallManager using nftables.
// Architecture (Stateless Bidirectional ACL with NOTRACK):
//   - Table: aria_raw (inet family, priority raw) - NOTRACK for WireGuard
//   - Table: aria_filter (inet family, priority filter) - ACL enforcement
//   - Chain: forward (hook forward, priority 0, type filter)
//   - Chain: input (hook input, priority 0, type filter)
//   - Set: aria_whitelist_outbound (saddr . daddr . protocol . dport)
//   - Set: aria_whitelist_inbound (saddr . daddr . protocol . sport)
//
// Design Decision: Dual Concatenated Sets for Stateless SD-WAN
//   - Precision: 100% exact matching (A→B:80 ≠ A→B:443)
//   - Performance: O(1) hash lookup, no conntrack overhead
//   - Stateless: NOTRACK for WireGuard, explicit bidirectional ACL rules
//   - Security: TCP flags check prevents external active connections
//
// Strategy:
//   - Init: Use nft command to create tables/chains/sets/rules (avoids Go expr bugs)
//   - Runtime: Go code only manages set elements (data filling)
//   - Bidirectional: Each ACL rule generates both outbound and inbound elements
//
// Safety rules (hardcoded in nft script, cannot be overridden):
//   - Allow SSH (tcp/22) from any
//   - Allow HTTPS (tcp/443) from any - for Web UI access
//   - Allow WireGuard (udp/51820) from any
//   - Allow ICMP/ICMPv6
//   - Allow loopback traffic
//   - Allow Controller API (tcp/8080 from localhost/private networks)
//   - Allow DHCP (udp/67-68)
type NftablesFirewallManager struct {
	mu sync.RWMutex

	conn              *nftables.Conn
	table             *nftables.Table
	whitelistOutbound *nftables.Set
	whitelistInbound  *nftables.Set

	enabled    bool
	ruleCount  int
	lastUpdate time.Time

	// Configuration
	wgPortMin int // WireGuard port range minimum (default: 51820)
	wgPortMax int // WireGuard port range maximum (default: 51827 for 8 tunnels)
}

// NftablesOption configures the NftablesFirewallManager.
type NftablesOption func(*NftablesFirewallManager)

// WithWireGuardPort sets the WireGuard port for safety rules.
func WithWireGuardPort(port int) NftablesOption {
	return func(m *NftablesFirewallManager) {
		m.wgPortMin = port
		m.wgPortMax = port
	}
}

// WithWireGuardPortRange sets the WireGuard port range for multi-tunnel mode.
func WithWireGuardPortRange(minPort, maxPort int) NftablesOption {
	return func(m *NftablesFirewallManager) {
		m.wgPortMin = minPort
		m.wgPortMax = maxPort
	}
}

// NewNftablesFirewallManager creates a new nftables-based firewall manager.
func NewNftablesFirewallManager(opts ...NftablesOption) *NftablesFirewallManager {
	m := &NftablesFirewallManager{
		wgPortMin: 51820,
		wgPortMax: 51830, // Support up to 8 tunnels + prober port (51830)
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Init initializes firewall tables, chains, and default safety rules.
// Strategy: Use nft command to create complex rules, Go only manages set elements.
// This avoids bugs in manually crafting concatenated lookup expressions.
func (m *NftablesFirewallManager) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Println("Firewall: initializing nftables via embedded script (concatenated set mode)")

	// Step 1: Parse and render template with configuration
	tmpl, err := template.New("nft-init").Parse(nftInitScriptTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse nft template: %w", err)
	}

	// Template data - compute port string to handle single port vs range
	portStr := fmt.Sprintf("%d", m.wgPortMin)
	if m.wgPortMin != m.wgPortMax {
		portStr = fmt.Sprintf("%d-%d", m.wgPortMin, m.wgPortMax)
	}
	data := map[string]interface{}{
		"WireGuardPort": portStr,
	}

	var scriptBuf bytes.Buffer
	if err := tmpl.Execute(&scriptBuf, data); err != nil {
		return fmt.Errorf("failed to render nft template: %w", err)
	}

	// Step 2: Execute nft script via stdin (no temporary files, more secure)
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = &scriptBuf

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to execute nft script: %w, output: %s", err, output)
	}

	log.Printf("Firewall: nft script executed successfully (WireGuard ports: %d-%d)", m.wgPortMin, m.wgPortMax)

	// Step 3: Connect to nftables via netlink (for runtime set management)
	m.conn, err = nftables.New()
	if err != nil {
		return fmt.Errorf("failed to connect to nftables: %w", err)
	}

	// Step 4: Get references to existing table/set (created by nft script)
	m.table = &nftables.Table{
		Family: nftables.TableFamilyINet,
		Name:   "aria_filter",
	}

	// Get the set references (we don't need to create them, just reference them)
	sets, err := m.conn.GetSets(m.table)
	if err != nil {
		return fmt.Errorf("failed to get nftables sets: %w", err)
	}

	for _, set := range sets {
		if set.Name == "aria_whitelist_outbound" {
			m.whitelistOutbound = set
		} else if set.Name == "aria_whitelist_inbound" {
			m.whitelistInbound = set
		}
	}

	if m.whitelistOutbound == nil {
		return fmt.Errorf("aria_whitelist_outbound set not found (nft script may have failed)")
	}

	if m.whitelistInbound == nil {
		return fmt.Errorf("aria_whitelist_inbound set not found (nft script may have failed)")
	}

	m.enabled = true
	m.lastUpdate = time.Now()
	log.Println("Firewall: nftables initialized and ready (stateless bidirectional mode)")

	// Step 5: Detect co-located Controller and add necessary rules
	if detectController() {
		log.Println("Firewall: detected co-located Controller, adding Controller-specific rules")
		if err := addControllerRules(); err != nil {
			log.Printf("Warning: failed to add Controller rules: %v", err)
			// Don't fail initialization, just log the warning
		} else {
			log.Println("Firewall: Controller rules added successfully")
		}
	}

	return nil
}

// Cleanup removes all managed firewall rules and tables.
func (m *NftablesFirewallManager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return nil
	}

	// Delete the entire table (removes all chains, sets, rules)
	m.conn.DelTable(m.table)

	if err := m.conn.Flush(); err != nil {
		return fmt.Errorf("failed to cleanup nftables: %w", err)
	}

	m.enabled = false
	log.Println("Firewall: nftables cleaned up")

	return nil
}

// ApplyPolicy atomically applies a new set of ACL rules.
// This replaces all existing rules with the new set (atomic transaction).
// Rules define: SrcNet -> DstNet:Protocol:Port whitelist.
//
// Stateless Bidirectional Strategy:
//   Each ACL rule is automatically split into two elements:
//   1. Outbound element: Src . Dst . Proto . Dport (for new connections)
//   2. Inbound element: Dst . Src . Proto . Sport (for return traffic)
//
// Key Construction:
//   - Outbound: SrcIP . DstIP . Proto . Dport
//   - Inbound: DstIP . SrcIP . Proto . Sport (reversed)
//
// Security:
//   - TCP outbound: Only SYN packets allowed (new connections)
//   - TCP inbound: Only non-SYN packets allowed (prevents external active connections)
//   - UDP: Both directions allowed (stateless)
//
// This implementation ensures:
//   1. Perfect support for asymmetric routing (Active-Active dual-homing)
//   2. No conntrack dependency (NOTRACK for WireGuard)
//   3. External hosts cannot initiate connections (TCP flags check)
//   4. O(1) lookup performance (hash-based sets)
func (m *NftablesFirewallManager) ApplyPolicy(rules []ACLRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return fmt.Errorf("firewall not initialized")
	}

	// Strategy: Use nft command line tool for atomic application
	// This avoids complex key encoding issues with concatenated sets

	// Step 1: Generate bidirectional elements
	outboundElems, inboundElems := m.generateStatelessRules(rules)

	// Step 2: Build atomic nft script
	var nftScript strings.Builder

	// Flush both sets
	nftScript.WriteString("flush set inet aria_filter aria_whitelist_outbound\n")
	nftScript.WriteString("flush set inet aria_filter aria_whitelist_inbound\n")

	// Add outbound elements
	if len(outboundElems) > 0 {
		nftScript.WriteString("add element inet aria_filter aria_whitelist_outbound {\n")
		for i, elem := range outboundElems {
			if i > 0 {
				nftScript.WriteString(",\n")
			}
			nftScript.WriteString("    " + elem)
		}
		nftScript.WriteString("\n}\n")
	}

	// Add inbound elements
	if len(inboundElems) > 0 {
		nftScript.WriteString("add element inet aria_filter aria_whitelist_inbound {\n")
		for i, elem := range inboundElems {
			if i > 0 {
				nftScript.WriteString(",\n")
			}
			nftScript.WriteString("    " + elem)
		}
		nftScript.WriteString("\n}\n")
	}

	// Step 3: Execute nft script atomically
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(nftScript.String())

	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("Firewall: nft script failed:\n%s\nError: %v\nOutput: %s",
			nftScript.String(), err, output)
		return fmt.Errorf("failed to apply ACL rules: %w", err)
	}

	m.ruleCount = len(rules)
	m.lastUpdate = time.Now()
	log.Printf("Firewall: applied %d ACL rules (%d outbound + %d inbound elements) using stateless mode",
		len(rules), len(outboundElems), len(inboundElems))

	return nil
}

// generateStatelessRules generates bidirectional ACL elements from rules.
// Each rule is automatically split into outbound and inbound elements.
//
// Core Logic (Reversal):
//   Outbound: Src . Dst . Proto . Dport (for new connections)
//   Inbound:  Dst . Src . Proto . Sport (reversed - for return traffic)
//
// Example:
//   Rule: 10.0.1.0/24 -> 8.8.8.8/32 : UDP : 53
//   Outbound: "10.0.1.0/24 . 8.8.8.8/32 . udp . 53"
//   Inbound:  "8.8.8.8/32 . 10.0.1.0/24 . udp . 53"
//
// This ensures:
//   - Outbound traffic from 10.0.1.0/24 to 8.8.8.8:53 is allowed
//   - Return traffic from 8.8.8.8:53 to 10.0.1.0/24 is allowed
//   - External hosts cannot initiate connections (TCP flags check in nftables)
func (m *NftablesFirewallManager) generateStatelessRules(rules []ACLRule) ([]string, []string) {
	var outboundElems []string
	var inboundElems []string

	for _, rule := range rules {
		srcNet := normalizeCIDR(rule.SrcNet)
		dstNet := normalizeCIDR(rule.DstNet)

		// Map protocol number to name
		protoName := "tcp"
		if rule.Protocol == 17 {
			protoName = "udp"
		} else if rule.Protocol == 1 {
			protoName = "icmp"
		}

		// Format port range
		var portStr string
		if rule.MinPort == rule.MaxPort {
			portStr = fmt.Sprintf("%d", rule.MinPort)
		} else if rule.MinPort == 0 && rule.MaxPort == 65535 {
			portStr = "0-65535"
		} else {
			portStr = fmt.Sprintf("%d-%d", rule.MinPort, rule.MaxPort)
		}

		// Generate outbound element: Src . Dst . Proto . Dport
		outElem := fmt.Sprintf("%s . %s . %s . %s",
			srcNet, dstNet, protoName, portStr)
		outboundElems = append(outboundElems, outElem)

		// Generate inbound element: Dst . Src . Proto . Sport (reversed)
		// This allows return traffic from Dst:Sport to Src
		inElem := fmt.Sprintf("%s . %s . %s . %s",
			dstNet, srcNet, protoName, portStr)
		inboundElems = append(inboundElems, inElem)
	}

	return outboundElems, inboundElems
}

// GetStats returns firewall statistics.
func (m *NftablesFirewallManager) GetStats() (*FirewallStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &FirewallStats{
		Enabled:    m.enabled,
		RuleCount:  m.ruleCount,
		LastUpdate: m.lastUpdate,
	}

	if !m.enabled {
		return stats, nil
	}

	// 读取 drop counter
	dropPackets, dropBytes := m.readCounter("drop_counter")
	stats.DroppedPackets = dropPackets
	stats.DroppedBytes = dropBytes
	stats.PacketsDropped = dropPackets // Legacy field
	stats.BytesDropped = dropBytes     // Legacy field

	// 读取 invalid counter
	invalidPackets, _ := m.readCounter("invalid_counter")
	stats.InvalidPackets = invalidPackets

	// 读取协议 counter
	_, tcpBytes := m.readCounter("tcp_counter")
	_, udpBytes := m.readCounter("udp_counter")
	_, icmpBytes := m.readCounter("icmp_counter")

	stats.TCPBytes = tcpBytes
	stats.UDPBytes = udpBytes
	stats.ICMPBytes = icmpBytes

	return stats, nil
}

// readCounter reads packets and bytes from an nftables counter using JSON format.
// Returns (packets, bytes). Uses fallback to text parsing if JSON fails.
func (m *NftablesFirewallManager) readCounter(name string) (uint64, uint64) {
	// 优先使用 JSON 格式（nftables 0.9.0+ 支持）
	cmd := exec.Command("nft", "-j", "list", "counter", "inet", "aria_filter", name)
	output, err := cmd.Output()
	if err != nil {
		// Fallback: 尝试文本格式（兼容旧版本）
		return m.readCounterText(name)
	}

	// 解析 JSON 格式
	var result struct {
		Nftables []struct {
			Counter struct {
				Packets uint64 `json:"packets"`
				Bytes   uint64 `json:"bytes"`
			} `json:"counter"`
		} `json:"nftables"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		log.Printf("[firewall] Failed to parse nft JSON output: %v", err)
		return m.readCounterText(name)
	}

	if len(result.Nftables) > 0 {
		return result.Nftables[0].Counter.Packets, result.Nftables[0].Counter.Bytes
	}

	return 0, 0
}

// readCounterText reads counter from text format (fallback for older nftables versions).
func (m *NftablesFirewallManager) readCounterText(name string) (uint64, uint64) {
	cmd := exec.Command("nft", "list", "counter", "inet", "aria_filter", name)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	// 解析: "counter packets 12345 bytes 67890"
	// 更稳健的正则表达式解析（处理多空格、换行）
	re := regexp.MustCompile(`packets\s+(\d+)\s+bytes\s+(\d+)`)
	matches := re.FindStringSubmatch(string(output))

	if len(matches) == 3 {
		packets, _ := strconv.ParseUint(matches[1], 10, 64)
		bytes, _ := strconv.ParseUint(matches[2], 10, 64)
		return packets, bytes
	}

	return 0, 0
}

// IsEnabled returns true if the firewall is active.
func (m *NftablesFirewallManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// detectController checks if Aria Controller is running on the same node.
// It checks for Docker containers named aria-controller or aria_controller.
func detectController() bool {
	// Method 1: Check for Docker containers
	cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err == nil {
		containers := string(output)
		if strings.Contains(containers, "aria-controller") || strings.Contains(containers, "aria_controller") {
			return true
		}
	}

	// Method 2: Check if port 8080 is listening (Controller API)
	cmd = exec.Command("sh", "-c", "ss -ltn | grep -q ':8080 ' || netstat -ltn 2>/dev/null | grep -q ':8080 '")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// addControllerRules adds firewall rules to allow Controller services.
// This is necessary when Controller and Agent run on the same node.
//
// Controller port access policy:
//   - 443/tcp  (HTTPS Web UI) - PUBLIC ACCESS (already in template)
//   - 8080/tcp (Controller API) - PUBLIC ACCESS (for external API calls)
//   - 5432/tcp (PostgreSQL) - PRIVATE ACCESS (Docker + localhost only)
//   - 6379/tcp (Redis) - PRIVATE ACCESS (Docker + localhost only)
//
// Additionally, adds Docker bridge network rules to forward chain
// to ensure container-to-container communication works properly.
func addControllerRules() error {
	// No need to check for duplicates - Init() already flushed the table
	// Environment is guaranteed to be clean

	// Detect Docker bridge networks
	bridgeCmd := exec.Command("sh", "-c", "ip link show | awk '/^[0-9]+: br-/ {print $2}' | sed 's/:$//'")
	bridgeOutput, _ := bridgeCmd.Output()
	bridges := strings.Fields(string(bridgeOutput))

	// Build nft script to add Controller rules
	var scriptBuilder strings.Builder
	scriptBuilder.WriteString(`
# Allow Controller API from any IP (public access for external API calls)
add rule inet aria_filter input tcp dport 8080 accept comment "Controller API (public)"

# Allow PostgreSQL and Redis for Controller (private access for security)
add rule inet aria_filter input tcp dport { 5432, 6379 } ip saddr { 127.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 } accept comment "Controller Database (private)"
`)

	// Add Docker bridge rules to forward chain
	for _, bridge := range bridges {
		if bridge != "" {
			scriptBuilder.WriteString(fmt.Sprintf("\n# Allow Docker bridge %s traffic\n", bridge))
			scriptBuilder.WriteString(fmt.Sprintf("add rule inet aria_filter forward iifname \"%s\" accept comment \"Docker %s\"\n", bridge, bridge))
			scriptBuilder.WriteString(fmt.Sprintf("add rule inet aria_filter forward oifname \"%s\" accept comment \"Docker %s\"\n", bridge, bridge))
		}
	}

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(scriptBuilder.String())

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add Controller rules: %w, output: %s", err, output)
	}

	log.Printf("Firewall: Controller rules added - Web UI (443: public), API (8080: public), Database (5432/6379: private)")
	if len(bridges) > 0 {
		log.Printf("Firewall: Docker bridge rules added for: %v", bridges)
	}
	return nil
}

// Helper functions

func ptrChainPolicy(p nftables.ChainPolicy) *nftables.ChainPolicy {
	return &p
}

// normalizeCIDR ensures the input is in CIDR format.
// If input is a plain IP (e.g., "10.0.0.1"), it appends "/32" to make it a valid CIDR.
// If input already contains "/", it's returned as-is.
func normalizeCIDR(ipOrCIDR string) string {
	if ipOrCIDR == "" {
		return ""
	}
	// Check if already in CIDR format
	if strings.Contains(ipOrCIDR, "/") {
		return ipOrCIDR
	}
	// Plain IP address, append /32 for single host
	return ipOrCIDR + "/32"
}

// cidrToRange converts a CIDR to start/end IP addresses.
func cidrToRange(cidr string) (net.IP, net.IP, error) {
	// Normalize input to ensure CIDR format
	cidr = normalizeCIDR(cidr)

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, err
	}

	// Start IP is the network address
	start := ipnet.IP.To4()
	if start == nil {
		return nil, nil, fmt.Errorf("only IPv4 supported")
	}

	// End IP is broadcast address
	end := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		end[i] = start[i] | ^ipnet.Mask[i]
	}

	return start, end, nil
}

// isMaxIP checks if an IP address is 255.255.255.255 (maximum IPv4 address).
// Used to avoid overflow when incrementing IPs for interval end markers.
func isMaxIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	return ip[0] == 255 && ip[1] == 255 && ip[2] == 255 && ip[3] == 255
}

// incrementIP increments an IPv4 address by 1.
// Used for interval end markers in nftables (exclusive upper bound).
// Example: 192.168.0.255 -> 192.168.1.0
func incrementIP(ip net.IP) net.IP {
	// Make a copy to avoid modifying the original
	result := make(net.IP, len(ip))
	copy(result, ip)

	// Increment from the least significant byte
	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			// No carry, done
			break
		}
		// Carry to next byte
	}

	return result
}

// buildConcatKey builds a concatenated key for the nftables set.
//
// Format: srcIP(4) + dstIP(4) + proto(1) + pad(1) + port(2) = 12 bytes
//
// Memory layout:
//   [0:4]   Source IP (network byte order, already correct from net.IP)
//   [4:8]   Destination IP (network byte order, already correct from net.IP)
//   [8]     Protocol (single byte, no endianness issue)
//   [9]     Padding (0x00, for alignment)
//   [10:12] Port (Big Endian - CRITICAL!)
//
// Why Big Endian for port?
//   - nftables uses network byte order (Big Endian) for all multi-byte values
//   - Port 80 (0x0050) must be stored as [0x00, 0x50], not [0x50, 0x00]
//   - binary.BigEndian.PutUint16 does exactly this
//
// Why padding at offset 9?
//   - inet_proto is 1 byte, inet_service is 2 bytes
//   - nftables aligns multi-byte values to even offsets
//   - Without padding, port would start at offset 9 (odd), causing misalignment
//   - With padding, port starts at offset 10 (even), matching nft's expectation
//
// This should match what nft generates internally for:
//   set aria_whitelist { type ipv4_addr . ipv4_addr . inet_proto . inet_service }
func buildConcatKey(srcIP, dstIP net.IP, proto uint8, port uint16) []byte {
	key := make([]byte, 12)
	copy(key[0:4], srcIP.To4())
	copy(key[4:8], dstIP.To4())
	key[8] = proto
	key[9] = 0 // padding for alignment
	binary.BigEndian.PutUint16(key[10:12], port)
	return key
}
