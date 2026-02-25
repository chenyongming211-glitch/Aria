package tools

import (
	"encoding/json"
	"fmt"

	"aria/pkg/controllerstorage"
)

// NewGetNetworkConfigTool 创建查询网络配置工具
func NewGetNetworkConfigTool(store *controllerstorage.Storage) Tool {
	return Tool{
		Name:        "get_network_config",
		Description: "查询网络基础配置，包括 base_ip 和 cidr",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Run: func(args string) (string, error) {
			config := map[string]string{
				"base_ip": store.GetBaseIP(),
				"cidr":    store.GetCIDR(),
			}

			data, err := json.Marshal(config)
			if err != nil {
				return "", fmt.Errorf("数据序列化失败: %v", err)
			}
			// 使用特殊标记告诉飞书 handler 这是卡片数据
			return fmt.Sprintf("CARD_DATA:%s", string(data)), nil
		},
	}
}
