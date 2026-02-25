package im

import (
	"encoding/json"
)

// ==========================================
// 1. 飞书卡片 V2 标准结构定义
// ==========================================

type FeishuCardV2 struct {
	Schema string         `json:"schema"` // 必须是 "2.0"
	Header *FeishuHeader `json:"header,omitempty"`
	Body   *FeishuBody   `json:"body,omitempty"`
}

type FeishuHeader struct {
	Title    FeishuText `json:"title"`
	Template string     `json:"template"` // blue, red, green, etc.
}

type FeishuBody struct {
	Elements []interface{} `json:"elements"`
}

type FeishuText struct {
	Content string `json:"content"`
	Tag     string `json:"tag"` // plain_text 或 lark_md
}

// ==========================================
// 2. CardBuilder 构建器实现
// ==========================================

type CardBuilder struct {
	card FeishuCardV2
}

// NewCard 创建一个新的卡片构建器
func NewCard(title string, color string) *CardBuilder {
	if color == "" {
		color = "blue"
	}
	return &CardBuilder{
		card: FeishuCardV2{
			Schema: "2.0",
			Header: &FeishuHeader{
				Title:    FeishuText{Tag: "plain_text", Content: title},
				Template: color,
			},
			Body: &FeishuBody{
				Elements: make([]interface{}, 0),
			},
		},
	}
}

// AddMarkdown 添加 Markdown 文本块
func (c *CardBuilder) AddMarkdown(content string) *CardBuilder {
	c.card.Body.Elements = append(c.card.Body.Elements, map[string]interface{}{
		"tag": "div",
		"text": map[string]interface{}{
			"tag":     "lark_md",
			"content": content,
		},
	})
	return c
}

// AddDoubleColumnText 添加双列文本 (用于 KV 展示)
func (c *CardBuilder) AddDoubleColumnText(left, right string) *CardBuilder {
	c.card.Body.Elements = append(c.card.Body.Elements, map[string]interface{}{
		"tag":              "column_set",
		"flex_mode":        "none",
		"background_style": "grey", // 灰色背景区分
		"columns": []interface{}{
			map[string]interface{}{
				"tag":    "column",
				"width":  "weighted",
				"weight": 1,
				"elements": []interface{}{
					map[string]interface{}{
						"tag": "div",
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": left,
						},
					},
				},
			},
			map[string]interface{}{
				"tag":    "column",
				"width":  "weighted",
				"weight": 1,
				"elements": []interface{}{
					map[string]interface{}{
						"tag": "div",
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": right,
						},
					},
				},
			},
		},
	})
	return c
}

// AddNote 添加底部备注
func (c *CardBuilder) AddNote(text string) *CardBuilder {
	c.card.Body.Elements = append(c.card.Body.Elements, map[string]interface{}{
		"tag": "note",
		"elements": []interface{}{
			map[string]interface{}{
				"tag":     "plain_text",
				"content": text,
			},
		},
	})
	return c
}

// AddButton 添加操作按钮 (可选，为将来做准备)
func (c *CardBuilder) AddButton(text string, value map[string]string, typeStr string) *CardBuilder {
	if typeStr == "" {
		typeStr = "default"
	}
	c.card.Body.Elements = append(c.card.Body.Elements, map[string]interface{}{
		"tag": "action",
		"actions": []interface{}{
			map[string]interface{}{
				"tag": "button",
				"text": map[string]interface{}{
					"tag":     "plain_text",
					"content": text,
				},
				"type":  typeStr,
				"value": value,
			},
		},
	})
	return c
}

// String 导出 JSON 字符串
func (c *CardBuilder) String() string {
	b, _ := json.Marshal(c.card)
	return string(b)
}
