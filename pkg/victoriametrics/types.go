package victoriametrics

// QueryRangeResult 表示 range query 的响应
// Feature: dashboard-realdata, 需求: 6.3, 6.4
type QueryRangeResult struct {
	Status string         `json:"status"`
	Data   QueryRangeData `json:"data"`
}

// QueryRangeData 包含 range query 的数据部分
type QueryRangeData struct {
	ResultType string            `json:"resultType"`
	Result     []RangeResultItem `json:"result"`
}

// RangeResultItem 表示 range query 中的单个时序结果
type RangeResultItem struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"` // [[timestamp, "value"], ...]
}

// QueryInstantResult 表示 instant query 的响应
type QueryInstantResult struct {
	Status string           `json:"status"`
	Data   QueryInstantData `json:"data"`
}

// QueryInstantData 包含 instant query 的数据部分
type QueryInstantData struct {
	ResultType string              `json:"resultType"`
	Result     []InstantResultItem `json:"result"`
}

// InstantResultItem 表示 instant query 中的单个结果
type InstantResultItem struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"` // [timestamp, "value"]
}
