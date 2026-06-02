package victoriametrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client 封装对 VictoriaMetrics HTTP API 的查询
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient 创建 VM 查询客户端
// baseURL 示例: "http://localhost:8428"
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// QueryRange 执行范围查询，对应 VictoriaMetrics /api/v1/query_range
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryRangeResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(end.Unix(), 10))
	params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		log.Printf("[victoriametrics] failed to create query_range request: %v", err)
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[victoriametrics] query_range request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[victoriametrics] query_range returned status %d", resp.StatusCode)
		return nil, fmt.Errorf("query_range returned status %d", resp.StatusCode)
	}

	var result QueryRangeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[victoriametrics] failed to decode query_range response: %v", err)
		return nil, err
	}

	return &result, nil
}

// QueryInstant 执行即时查询，对应 VictoriaMetrics /api/v1/query
func (c *Client) QueryInstant(ctx context.Context, query string) (*QueryInstantResult, error) {
	params := url.Values{}
	params.Set("query", query)

	reqURL := fmt.Sprintf("%s/api/v1/query?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		log.Printf("[victoriametrics] failed to create query request: %v", err)
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[victoriametrics] query request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[victoriametrics] query returned status %d", resp.StatusCode)
		return nil, fmt.Errorf("query returned status %d", resp.StatusCode)
	}

	var result QueryInstantResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[victoriametrics] failed to decode query response: %v", err)
		return nil, err
	}

	return &result, nil
}

// emptyRangeResult 返回空的 range query 结果，用于 VM 不可用时的降级
func emptyRangeResult() *QueryRangeResult {
	return &QueryRangeResult{
		Status: "success",
		Data: QueryRangeData{
			ResultType: "matrix",
			Result:     []RangeResultItem{},
		},
	}
}

// emptyInstantResult 返回空的 instant query 结果，用于 VM 不可用时的降级
func emptyInstantResult() *QueryInstantResult {
	return &QueryInstantResult{
		Status: "success",
		Data: QueryInstantData{
			ResultType: "vector",
			Result:     []InstantResultItem{},
		},
	}
}
