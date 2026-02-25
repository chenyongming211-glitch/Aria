package metrics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server HTTP 指标服务器
type Server struct {
	addr           string
	manager        *CollectorManager
	httpServer     *http.Server
	collectTicker  *time.Ticker
	stopChan       chan struct{}
	collectTimeout time.Duration
}

// NewServer 创建 HTTP 指标服务器
func NewServer(addr string, manager *CollectorManager, collectInterval time.Duration) *Server {
	return &Server{
		addr:           addr,
		manager:        manager,
		stopChan:       make(chan struct{}),
		collectTimeout: 5 * time.Second, // 采集超时
		collectTicker:  time.NewTicker(collectInterval),
	}
}

// Start 启动服务器
func (s *Server) Start() error {
	// 设置路由
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(
		s.manager.Registry(),
		promhttp.HandlerOpts{},
	))
	mux.HandleFunc("/health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	// 启动定期采集
	go s.periodicCollect()

	// 启动 HTTP 服务器
	log.Printf("[metrics] HTTP server listening on %s", s.addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server failed: %w", err)
	}

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	close(s.stopChan)
	s.collectTicker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// periodicCollect 定期采集指标
func (s *Server) periodicCollect() {
	// 启动时立即采集一次
	ctx, cancel := context.WithTimeout(context.Background(), s.collectTimeout)
	if err := s.manager.CollectAll(ctx); err != nil {
		log.Printf("[metrics] Initial collection failed: %v", err)
	}
	cancel()

	for {
		select {
		case <-s.stopChan:
			return
		case <-s.collectTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), s.collectTimeout)
			if err := s.manager.CollectAll(ctx); err != nil {
				log.Printf("[metrics] Collection failed: %v", err)
			}
			cancel()
		}
	}
}

// handleHealth 健康检查端点
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}
