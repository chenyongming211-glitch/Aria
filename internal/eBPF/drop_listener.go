package eBPF

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"github.com/cilium/ebpf/ringbuf"
)

// StartDropListener 启动一个 goroutine 监听内核丢包事件
func (q *QoSManager) StartDropListener() error {
	rd, err := ringbuf.NewReader(q.maps.DropAlerts)
	if err != nil {
		return fmt.Errorf("failed to create ringbuf reader: %v", err)
	}

	go func() {
		log.Println("Drop event listener started")
		defer rd.Close()

		for {
			record, err := rd.Read()
			if err != nil {
				// ringbuf 关闭时正常退出
				if err == ringbuf.ErrClosed {
					log.Println("Drop event listener closed")
					return
				}
				log.Printf("Error reading ringbuf: %v", err)
				continue
			}

			// 解析二进制数据到结构体（注意字节序：小端）
			var event DropEventT
			if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("Failed to parse drop event: %v", err)
				continue
			}

			// 打印告警（可根据需要写入数据库或推送到前端）
			reasonStr := "ACL"
			if event.Reason == 2 {
				reasonStr = "QoS"
			}
			log.Printf("[Alert] Rule #%d triggered %s packet drops, source %s:%d -> dest %s:%d, protocol %d, timestamp %d",
				event.RuleID, reasonStr,
				intToIP(event.SrcIP), event.SrcPort,
				intToIP(event.DstIP), event.DstPort,
				event.Proto, event.Timestamp)
		}
	}()

	return nil
}

// 辅助函数：uint32 → IP 字符串
func intToIP(ip uint32) string {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)).String()
}

// StartDropListener 启动一个 goroutine 监听内核丢包事件
func (a *ACLManager) StartDropListener() error {
	rd, err := ringbuf.NewReader(a.maps.DropAlerts)
	if err != nil {
		return fmt.Errorf("failed to create ringbuf reader: %v", err)
	}

	go func() {
		log.Println("ACL Drop event listener started")
		defer rd.Close()

		for {
			record, err := rd.Read()
			if err != nil {
				// ringbuf 关闭时正常退出
				if err == ringbuf.ErrClosed {
					log.Println("ACL Drop event listener closed")
					return
				}
				log.Printf("Error reading ringbuf: %v", err)
				continue
			}

			// 解析二进制数据到结构体（注意字节序：小端）
			var event DropEventT
			if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("Failed to parse ACL drop event: %v", err)
				continue
			}

			// 打印告警（可根据需要写入数据库或推送到前端）
			reasonStr := "ACL"
			if event.Reason == 2 {
				reasonStr = "QoS"
			}
			log.Printf("[Alert] Rule #%d triggered %s packet drops, source %s:%d -> dest %s:%d, protocol %d, timestamp %d",
				event.RuleID, reasonStr,
				intToIP(event.SrcIP), event.SrcPort,
				intToIP(event.DstIP), event.DstPort,
				event.Proto, event.Timestamp)
		}
	}()

	return nil
}