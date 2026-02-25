package im

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NodeInfo 定义节点信息结构
type NodeInfo struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	AssignedIP string `json:"assigned_ip"`
	PublicIP   string `json:"public_ip"`
	Region     string `json:"region"`
	Version    string `json:"version"`
}

// formatValueForCard 格式化值用于卡片显示
func formatValueForCard(value interface{}) string {
	if value == nil {
		return "null"
	}
	if value == "" {
		return "(空)"
	}

	switch v := value.(type) {
	case string:
		if len(v) > 50 {
			return v[:50] + "..."
		}
		return v
	case float64:
		if v == float64(int(v)) {
			return fmt.Sprintf("%d", int(v))
		}
		return fmt.Sprintf("%.2f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case bool:
		if v {
			return "✅"
		}
		return "❌"
	case []interface{}:
		return fmt.Sprintf("[%d 项]", len(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// BuildNodeListCard 构建节点列表卡片
func BuildNodeListCard(nodes []NodeInfo) string {
	card := NewCard("🌐 节点列表概览", "blue")

	for i, node := range nodes {
		statusIcon := "🔴"
		if strings.EqualFold(node.Status, "online") {
			statusIcon = "🟢"
		}

		// 第一行：图标 + 节点名 (加粗)
		card.AddMarkdown(fmt.Sprintf("### %s %s", statusIcon, node.Name))

		// 第二行：IP 和 区域
		ip := node.AssignedIP
		if ip == "" {
			ip = "N/A"
		}
		region := node.Region
		if region == "" {
			region = "Unknown"
		}

		card.AddDoubleColumnText(
			"**IP**: "+ip,
			"**区域**: "+region,
		)

		// 只有不是最后一个元素时，才加分割线
		if i < len(nodes)-1 {
			card.AddMarkdown("---")
		}
	}

	// 使用 Markdown 添加底部说明（v2 不再支持 note 标签）
	card.AddMarkdown(fmt.Sprintf("\n\n---\n\n*数据来源: Aria Controller • 共 %d 个节点*", len(nodes)))
	return card.String()
}

// BuildNodeDetailCard 构建单节点详情卡片
func BuildNodeDetailCard(node NodeInfo) string {
	statusIcon := "🔴"
	statusColor := "red"
	statusText := "离线 (Offline)"

	if strings.EqualFold(node.Status, "online") {
		statusIcon = "🟢"
		statusColor = "green"
		statusText = "在线 (Online)"
	}

	card := NewCard(fmt.Sprintf("%s 节点详情: %s", statusIcon, node.Name), statusColor)

	// 详细信息表格
	card.AddDoubleColumnText("**Assigned IP**", node.AssignedIP)
	card.AddDoubleColumnText("**Public IP**", node.PublicIP)
	card.AddDoubleColumnText("**Region**", node.Region)
	card.AddDoubleColumnText("**Version**", node.Version)
	card.AddDoubleColumnText("**Status**", statusText)

	// 分割线
	card.AddMarkdown("---")

	// 可以加一个操作按钮 (示例)
	// card.AddButton("重启节点", map[string]string{"action": "reboot", "node": node.Name}, "danger")

	return card.String()
}

// RouteInfo 定义路由信息结构
type RouteInfo struct {
	Hostname    string   `json:"hostname"`
	PublicIP   string   `json:"public_ip"`
	Region      string   `json:"region"`
	RouteCount  int      `json:"route_count"`
	Routes      []string `json:"routes"`
}

// BuildRouteListCard 构建路由列表卡片
func BuildRouteListCard(routes []map[string]interface{}) string {
	card := NewCard("🌐 节点路由列表", "blue")

	for i, route := range routes {
		hostname := getStringField(route, "hostname")
		publicIP := getStringField(route, "public_ip")
		region := getStringField(route, "region")
		routeCount := getIntField(route, "route_count")
		routesList := getStringSliceField(route, "routes")

		// 第一行：节点名称（加粗）
		card.AddMarkdown(fmt.Sprintf("### **%s**", hostname))

		// 第二行：IP 和 区域
		if publicIP != "" {
			card.AddDoubleColumnText("**IP**: "+publicIP, "**区域**: "+region)
		} else {
			card.AddDoubleColumnText("**区域**: "+region, "**路由数**: "+fmt.Sprintf("%d", routeCount))
		}

		// 第三行：路由列表
		if len(routesList) > 0 {
			card.AddMarkdown("**路由**:")
			for _, r := range routesList {
				card.AddMarkdown(fmt.Sprintf("  • %s", r))
			}
		} else {
			card.AddMarkdown("**路由**: *无配置路由*")
		}

		// 只有不是最后一个元素时，才加分割线
		if i < len(routes)-1 {
			card.AddMarkdown("---")
		}
	}

	// 使用 Markdown 添加底部说明
	card.AddMarkdown(fmt.Sprintf("\n\n---\n\n*数据来源: Aria Controller • 共 %d 个节点*", len(routes)))
	return card.String()
}

// 辅助函数：获取字符串字段
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// 辅助函数：获取整数字段
func getIntField(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 0
}

// 辅助函数：获取字符串切片字段
func getStringSliceField(m map[string]interface{}, key string) []string {
	if val, ok := m[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return []string{}
}

// BuildGenericJSONCard 构建通用 JSON 卡片
func BuildGenericJSONCard(jsonStr string) string {
	card := NewCard("📊 数据报表", "cyan")

	// 尝试解析为对象
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err == nil {
		// 对象格式：用 Markdown 表格显示
		markdown := "| 字段 | 值 |\n|---|---|\n"
		for key, value := range obj {
			formattedValue := formatValueForCard(value)
			markdown += fmt.Sprintf("| **%s** | %s |\n", key, formattedValue)
		}
		card.AddMarkdown(markdown)
		return card.String()
	}

	// 尝试解析为数组
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &arr); err == nil && len(arr) > 0 {
		// 数组格式：用 Markdown 表格显示
		headers := make([]string, 0, len(arr[0]))
		for k := range arr[0] {
			headers = append(headers, k)
		}

		// 表头
		markdown := "|"
		for _, h := range headers {
			markdown += fmt.Sprintf(" **%s** |", h)
		}
		markdown += "\n|---|"

		for _ = range headers {
			markdown += "---|"
		}
		markdown += "\n"

		// 数据行
		for _, item := range arr {
			for _, h := range headers {
				value := item[h]
				formattedValue := formatValueForCard(value)
				markdown += fmt.Sprintf(" %s |", formattedValue)
			}
			markdown += "\n"
		}

		card.AddMarkdown(markdown)
		return card.String()
	}

	// 无法识别的格式，直接显示原始 JSON
	card.AddMarkdown(fmt.Sprintf("```json\n%s\n```", jsonStr))
	return card.String()
}
