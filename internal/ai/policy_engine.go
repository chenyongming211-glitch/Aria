package ai

import (
	"encoding/json"
	"fmt"
	"log"

	firewall "aria/internal/agent/firewall"
)

// PolicyEngine manages AI-driven network policies using eBPF
type PolicyEngine struct {
	adapter *firewall.EBPFAdapter
}

// NewPolicyEngine creates a new AI-driven policy engine
func NewPolicyEngine(adapter *firewall.EBPFAdapter) *PolicyEngine {
	return &PolicyEngine{
		adapter: adapter,
	}
}

// ApplyThreatIntelligence applies threat intelligence rules
func (p *PolicyEngine) ApplyThreatIntelligence(iocs []string) error {
	log.Printf("Applying threat intelligence for %d IOCs", len(iocs))

	for _, ioc := range iocs {
		if err := p.adapter.BlockIP(ioc); err != nil {
			log.Printf("Failed to block IOC %s: %v", ioc, err)
		} else {
			log.Printf("Blocked IOC: %s", ioc)
		}
	}

	return nil
}

// ApplyBandwidthOptimization applies AI-driven bandwidth optimization rules
func (p *PolicyEngine) ApplyBandwidthOptimization(rules []BWRule) error {
	log.Printf("Applying bandwidth optimization for %d rules", len(rules))

	for _, rule := range rules {
		switch rule.Type {
		case "ip":
			if err := p.adapter.LimitIP(rule.Target, rule.Mbps); err != nil {
				log.Printf("Failed to limit IP %s: %v", rule.Target, err)
			} else {
				log.Printf("Limited IP %s to %d Mbps", rule.Target, rule.Mbps)
			}
		case "peer":
			if rule.SrcIP == "" || rule.DstIP == "" {
				log.Printf("Invalid peer rule: missing src or dst IP")
				continue
			}
			if err := p.adapter.LimitPeerPair(rule.SrcIP, rule.DstIP, rule.Mbps); err != nil {
				log.Printf("Failed to limit peer %s->%s: %v", rule.SrcIP, rule.DstIP, err)
			} else {
				log.Printf("Limited peer %s->%s to %d Mbps", rule.SrcIP, rule.DstIP, rule.Mbps)
			}
		default:
			log.Printf("Unknown rule type: %s", rule.Type)
		}
	}

	return nil
}

// BWRule represents a bandwidth rule
type BWRule struct {
	Type   string // "ip" or "peer"
	Target string // IP address for "ip" type
	SrcIP  string // Source IP for "peer" type
	DstIP  string // Destination IP for "peer" type
	Mbps   int    // Bandwidth limit in Mbps
}

// HandleAIMessage processes AI-generated network policy commands
func (p *PolicyEngine) HandleAIMessage(message string) error {
	log.Printf("Processing AI message: %s", message)

	// Parse the message as a JSON command
	var cmd AIMessage
	if err := json.Unmarshal([]byte(message), &cmd); err != nil {
		return fmt.Errorf("parsing AI message: %v", err)
	}

	switch cmd.Command {
	case "block_ip":
		ip, ok := cmd.Params["ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid IP in params")
		}

		if err := p.adapter.BlockIP(ip); err != nil {
			return fmt.Errorf("blocking IP %s: %v", ip, err)
		}

		log.Printf("AI command: Blocked IP %s", ip)

	case "limit_ip":
		ip, ok := cmd.Params["ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid IP in params")
		}

		mbps, ok := cmd.Params["mbps"].(float64) // JSON unmarshals numbers as float64
		if !ok {
			return fmt.Errorf("missing or invalid MBPS in params")
		}

		if err := p.adapter.LimitIP(ip, int(mbps)); err != nil {
			return fmt.Errorf("limiting IP %s: %v", ip, err)
		}

		log.Printf("AI command: Limited IP %s to %d Mbps", ip, int(mbps))

	case "limit_peer":
		srcIP, ok := cmd.Params["src_ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid source IP in params")
		}

		dstIP, ok := cmd.Params["dst_ip"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid destination IP in params")
		}

		mbps, ok := cmd.Params["mbps"].(float64)
		if !ok {
			return fmt.Errorf("missing or invalid MBPS in params")
		}

		if err := p.adapter.LimitPeerPair(srcIP, dstIP, int(mbps)); err != nil {
			return fmt.Errorf("limiting peer %s->%s: %v", srcIP, dstIP, err)
		}

		log.Printf("AI command: Limited peer %s->%s to %d Mbps", srcIP, dstIP, int(mbps))

	default:
		return fmt.Errorf("unknown AI command: %s", cmd.Command)
	}

	return nil
}

// AIMessage represents an AI-generated command
type AIMessage struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

// OptimizeForTrafficPattern analyzes traffic patterns and applies optimizations
func (p *PolicyEngine) OptimizeForTrafficPattern(analysis TrafficAnalysis) error {
	log.Printf("Optimizing for traffic pattern: %s", analysis.Pattern)

	// Based on the analysis, apply appropriate optimizations
	switch analysis.Pattern {
	case "high_concurrency":
		// For high concurrency, apply peer-level limits to prevent any single connection from monopolizing
		for _, conn := range analysis.Connections {
			if conn.Bandwidth > 10 { // If using more than 10 Mbps
				rule := BWRule{
					Type:  "peer",
					SrcIP: conn.SrcIP,
					DstIP: conn.DstIP,
					Mbps:  5, // Limit to 5 Mbps per connection
				}

				if err := p.ApplyBandwidthOptimization([]BWRule{rule}); err != nil {
					log.Printf("Failed to apply optimization for %s->%s: %v", conn.SrcIP, conn.DstIP, err)
				}
			}
		}
	case "bulk_transfer":
		// For bulk transfers, set higher limits but with fair sharing
		for _, ip := range analysis.TopTalkers {
			rule := BWRule{
				Type:   "ip",
				Target: ip,
				Mbps:   50, // Higher limit for bulk transfers
			}

			if err := p.ApplyBandwidthOptimization([]BWRule{rule}); err != nil {
				log.Printf("Failed to apply optimization for %s: %v", ip, err)
			}
		}
	case "real_time":
		// For real-time traffic, ensure minimum guaranteed bandwidth and low latency
		for _, ip := range analysis.RealTimeApps {
			// Apply QoS to prioritize real-time traffic
			rule := BWRule{
				Type:   "ip",
				Target: ip,
				Mbps:   100, // High priority traffic gets higher limits
			}

			if err := p.ApplyBandwidthOptimization([]BWRule{rule}); err != nil {
				log.Printf("Failed to apply optimization for %s: %v", ip, err)
			}
		}
	}

	return nil
}

// TrafficAnalysis represents analyzed traffic patterns
type TrafficAnalysis struct {
	Pattern       string     `json:"pattern"`
	Connections   []ConnInfo `json:"connections"`
	TopTalkers    []string   `json:"top_talkers"`
	RealTimeApps  []string   `json:"real_time_apps"`
}

// ConnInfo represents connection information
type ConnInfo struct {
	SrcIP      string `json:"src_ip"`
	DstIP      string `json:"dst_ip"`
	Bandwidth  int    `json:"bandwidth_mbps"`
	PacketsPS  int    `json:"packets_per_second"`
	Latency    int    `json:"latency_ms"`
	Protocol   string `json:"protocol"`
}