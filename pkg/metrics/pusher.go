package metrics

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

type Pusher struct {
	url      string
	jobName  string
	registry *prometheus.Registry
	client   *http.Client
	username string // Basic Auth
	password string // Basic Auth
	stopChan chan struct{}
}

type PusherOption func(*Pusher)

// WithBasicAuth 设置 Basic Auth 认证
func WithBasicAuth(username, password string) PusherOption {
	return func(p *Pusher) {
		p.username = username
		p.password = password
	}
}

func NewPusher(url, jobName string, registry *prometheus.Registry, opts ...PusherOption) *Pusher {
	p := &Pusher{
		url:      url,
		jobName:  jobName,
		registry: registry,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				// 跳过 TLS 证书验证（生产环境建议使用有效证书）
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		stopChan: make(chan struct{}),
	}

	// 应用选项
	for _, opt := range opts {
		opt(p)
	}

	return p
}

func (p *Pusher) Start(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[metrics] Pusher started: pushing to %s every %v", p.url, interval)

	// 立即推送一次
	if err := p.push(); err != nil {
		log.Printf("[metrics] Initial push failed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := p.push(); err != nil {
				log.Printf("[metrics] Push failed: %v", err)
			}
		case <-p.stopChan:
			log.Printf("[metrics] Pusher stopped")
			return
		}
	}
}

func (p *Pusher) push() error {
	// 收集所有指标
	mfs, err := p.registry.Gather()
	if err != nil {
		return fmt.Errorf("gather metrics failed: %w", err)
	}

	// 序列化为 Prometheus text format
	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.FmtText)
	for _, mf := range mfs {
		if err := encoder.Encode(mf); err != nil {
			return fmt.Errorf("encode metric failed: %w", err)
		}
	}

	// 构造 HTTP 请求
	req, err := http.NewRequest("POST", p.url, &buf)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", string(expfmt.FmtText))
	req.Header.Set("User-Agent", fmt.Sprintf("aria-agent/%s", p.jobName))

	// 【Basic Auth】
	if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
		authHeader := req.Header.Get("Authorization")
		log.Printf("[metrics] DEBUG - username: '%s', password: '%s', header: %s", p.username, p.password, authHeader)
	} else {
		log.Printf("[metrics] WARNING: No Basic Auth configured!")
	}

	// 发送请求
	log.Printf("[metrics] Pushing to %s", p.url)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	log.Printf("[metrics] Push succeeded: %d metrics, %d bytes", len(mfs), buf.Len())
	return nil
}

func (p *Pusher) Stop() {
	close(p.stopChan)
}

