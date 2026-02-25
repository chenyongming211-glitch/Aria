package im

import (
	"aria/pkg/logging"
)

// BaseChannel 定义通道的基本接口
type BaseChannel struct {
	Logger *logging.Logger
	Name   string
}

// Message 表示通道消息
type Message struct {
	ChatID string      `json:"chat_id"`
	Content string      `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewBaseChannel 创建基础通道
func NewBaseChannel(logger *logging.Logger, name string) *BaseChannel {
	return &BaseChannel{
		Logger: logger,
		Name:   name,
	}
}

// HandleMessage 处理通道消息（由子类型实现）
func (b *BaseChannel) HandleMessage(msg *Message) error {
	b.Logger.Info("Received message from %s: chat_id=%s, content=%s", b.Name, msg.ChatID, msg.Content)
	return nil
}
