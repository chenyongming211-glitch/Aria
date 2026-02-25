package monitor

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"aria/internal/eBPF" // Adjust import path according to your project structure
)

// AlertConsumer handles consuming alerts from the eBPF RingBuffer
type AlertConsumer struct {
	reader *ringbuf.Reader
	cancel context.CancelFunc
}

// NewAlertConsumer creates a new alert consumer for the drop_alerts RingBuffer
func NewAlertConsumer(dropAlertsMap *ringbuf.Map) (*AlertConsumer, error) {
	reader, err := ringbuf.NewReader(dropAlertsMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create ringbuf reader: %v", err)
	}

	return &AlertConsumer{
		reader: reader,
	}, nil
}

// Start begins consuming alerts in a separate goroutine
func (ac *AlertConsumer) Start(ctx context.Context) {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(ctx)
	ac.cancel = cancel

	// Start the consumption loop in a goroutine
	go ac.consumeLoop(ctx)
}

// consumeLoop handles the continuous consumption of ringbuffer records
func (ac *AlertConsumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Context cancelled, close the reader and return
			if err := ac.reader.Close(); err != nil {
				log.Printf("Error closing ringbuf reader: %v", err)
			}
			return
		default:
			record, err := ac.reader.Read()
			if err != nil {
				if errors.Is(err, ringbuf.ErrClosed) {
					return // 正常关闭
				}
				log.Printf("Ringbuf read error: %v", err)
				continue
			}
			ac.processRecord(record)
		}
	}
}

// processRecord processes a single ringbuffer record and formats the alert
func (ac *AlertConsumer) processRecord(record ringbuf.Record) {
	// Parse the DropEventT structure from the raw sample
	event, err := parseDropEvent(record.RawSample)
	if err != nil {
		log.Printf("Error parsing drop event: %v", err)
		return
	}

	// Format the alert message
	ac.formatAlert(event)
}

// parseDropEvent parses the raw bytes into a DropEventT structure
func parseDropEvent(data []byte) (*eBPF.DropEventT, error) {
	if len(data) < 32 { // DropEventT is 32 bytes
		return nil, fmt.Errorf("insufficient data length: got %d, expected at least 32", len(data))
	}

	var event eBPF.DropEventT
	offset := 0

	event.RuleID = binary.LittleEndian.Uint32(data[offset:]); offset += 4
	event.Reason = binary.LittleEndian.Uint32(data[offset:]); offset += 4
	event.SrcIP  = binary.BigEndian.Uint32(data[offset:]); offset += 4
	event.DstIP  = binary.BigEndian.Uint32(data[offset:]); offset += 4
	event.SrcPort = binary.BigEndian.Uint16(data[offset:]); offset += 2
	event.DstPort = binary.BigEndian.Uint16(data[offset:]); offset += 2
	event.Proto = data[offset]; offset++
	offset++ // skip pad1
	offset += 2 // skip pad2
	event.Timestamp = binary.LittleEndian.Uint64(data[offset:])

	return &event, nil
}

// formatAlert formats and prints the alert message in a structured way
func (ac *AlertConsumer) formatAlert(event *eBPF.DropEventT) {
	// Convert IP addresses to human-readable format
	srcIP := intToIP(event.SrcIP)
	dstIP := intToIP(event.DstIP)

	// Format the reason
	reason := formatReason(event.Reason)

	// Format the protocol
	proto := formatProtocol(event.Proto)

	// Print the formatted alert
	fmt.Printf("[%s] %s %s:%d -> %s:%d (%s) [RuleID: %d, UptimeNS: %d]\n",
		getCurrentTimeString(),
		reason,
		srcIP, event.SrcPort,
		dstIP, event.DstPort,
		proto,
		event.RuleID,
		event.Timestamp)
}

// formatReason formats the reason code into a human-readable string
func formatReason(reason uint32) string {
	switch reason {
	case 1:
		return "[ACL_DROP]"
	case 2:
		return "[QoS_LIMIT_DROP]"
	default:
		return fmt.Sprintf("[UNKNOWN_REASON_%d]", reason)
	}
}

// formatProtocol formats the protocol number into a human-readable string
func formatProtocol(proto uint8) string {
	switch proto {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	default:
		return fmt.Sprintf("PROTO_%d", proto)
	}
}

// intToIP converts a uint32 IP address to a net.IP string
func intToIP(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(ip>>24),
		byte(ip>>16),
		byte(ip>>8),
		byte(ip))
}


// getCurrentTimeString returns the current time in a consistent format
func getCurrentTimeString() string {
	return time.Now().Format("15:04:05.000") // 增加毫秒精度，对分析突发丢包很有帮助
}

// Stop gracefully stops the alert consumer
func (ac *AlertConsumer) Stop() {
	if ac.cancel != nil {
		ac.cancel()
	}
}