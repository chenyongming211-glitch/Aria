// Package protocol 处理 SD-WAN 自定义协议
package protocol

// Protocol 定义了 SD-WAN 通信协议的接口
type Protocol interface {
	// Encode 将消息编码为字节流
	Encode(msg Message) ([]byte, error)
	// Decode 将字节流解码为消息
	Decode(data []byte) (Message, error)
}

// Message 表示协议消息的基础结构
type Message struct {
	Type    MessageType
	Payload []byte
}

// MessageType 消息类型
type MessageType uint8

const (
	// MsgTypeHeartbeat 心跳消息
	MsgTypeHeartbeat MessageType = iota
	// MsgTypeRoute 路由更新消息
	MsgTypeRoute
	// MsgTypeTunnel 隧道控制消息
	MsgTypeTunnel
	// MsgTypeConfig 配置同步消息
	MsgTypeConfig
)
