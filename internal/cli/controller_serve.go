package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/yaml.v3"

	"aria/internal/api/middleware"
	v2 "aria/internal/api/v2"
	"aria/internal/auth"
	grpcserver "aria/internal/controller/grpc"
	"aria/internal/im"
	"aria/internal/security/certissuance"
	"aria/internal/service"
	"aria/internal/token"
	"aria/pkg/controllerstorage"
	"aria/pkg/grpc/agentpb"
	"aria/pkg/logging"
	"aria/pkg/victoriametrics"
)

var controllerServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the controller service",
	Long: `Start the Aria Controller HTTP service.

The controller provides:
  - /api/v2/agents/register   - Agent registration endpoint
  - /api/v2/agents/unregister - Agent unregistration endpoint
  - /api/v2/auth/*            - Authentication APIs
  - /api/v2/tenants/*         - Tenant-scoped management APIs

Examples:
  aria controller serve
  aria controller serve --config=/etc/aria/controller.yaml`,
	RunE: runControllerServe,
}

var serveConfigPath string

func init() {
	controllerCmd.AddCommand(controllerServeCmd)
	controllerServeCmd.Flags().StringVar(&serveConfigPath, "config", "/etc/aria/controller.yaml", "Configuration file path")
}

// ControllerConfig represents the controller configuration
type ControllerConfig struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Storage struct {
		Postgres struct {
			Host     string `yaml:"host"`
			Port     int    `yaml:"port"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
			Database string `yaml:"database"`
			SSLMode  string `yaml:"sslmode"`
		} `yaml:"postgres"`
		Redis struct {
			Addr     string `yaml:"addr"`
			Password string `yaml:"password"`
		} `yaml:"redis"`
	} `yaml:"storage"`
	Network struct {
		BaseIP string `yaml:"base_ip"`
		CIDR   string `yaml:"cidr"`
	} `yaml:"network"`
	Metrics struct {
		PushGateway string `yaml:"push_gateway"` // VictoriaMetrics push gateway URL
	} `yaml:"metrics"`
	Logging struct {
		Level string `yaml:"level"`
		Dir   string `yaml:"dir"`
	} `yaml:"logging"`
	AI struct {
		Enabled      bool   `yaml:"enabled"`
		APIKey       string `yaml:"api_key"`
		BaseURL      string `yaml:"base_url"`
		Model        string `yaml:"model"`
		SystemPrompt string `yaml:"system_prompt"`
	} `yaml:"ai"`
	DingTalk struct {
		Enabled bool   `yaml:"enabled"`
		Webhook string `yaml:"webhook"`
		Secret  string `yaml:"secret"`
	} `yaml:"dingtalk"`
	Feishu struct {
		Enabled     bool   `yaml:"enabled"`
		AppID       string `yaml:"app_id"`
		AppSecret   string `yaml:"app_secret"`
		EncryptKey  string `yaml:"encrypt_key"`
		VerifyToken string `yaml:"verify_token"`
	} `yaml:"feishu"`
	JWT struct {
		Secret string `yaml:"secret"` // HMAC signing key for JWT tokens
	} `yaml:"jwt"`
}

// Controller represents the controller service
type Controller struct {
	store              *controllerstorage.Storage
	tenantScopedStore  *middleware.TenantScopedStorage // Enhanced tenant-scoped storage
	heartbeat          *controllerstorage.HeartbeatStore
	tokenStore         *token.Store
	tokenValidator     *token.Validator
	certService        *certissuance.Service
	baseIP             string
	cidr               string
	metricsPushGateway string
	logger             *logging.Logger
	dingtalkHandler    *im.DingTalkHandler
	feishuHandler      *im.FeishuHandler
}

// RegisterRequest is the request payload for registration
type RegisterRequest struct {
	PublicKey        string   `json:"public_key"`
	Endpoint         string   `json:"endpoint"`
	PrivateIP        string   `json:"private_ip"`
	PublicIP         string   `json:"public_ip"`
	Region           string   `json:"region"`
	VPCID            string   `json:"vpc_id"`
	Hostname         string   `json:"hostname"`
	MachineID        string   `json:"machine_id"`
	RegisteredAt     int64    `json:"registered_at"`
	Token            string   `json:"token"`
	AdvertisedRoutes []string `json:"advertised_routes,omitempty"` // Site-to-Site VPN
	// Capability detection fields
	RuntimeMode   string `json:"runtime_mode,omitempty"`
	KernelVersion string `json:"kernel_version,omitempty"`
	HasAESNI      bool   `json:"has_aesni,omitempty"`
	CSRPEM        string `json:"csr_pem,omitempty"`
}

// NodeInfo represents a node in the network
type NodeInfo struct {
	PublicKey         string   `json:"public_key"`
	Endpoint          string   `json:"endpoint"`
	PrivateIP         string   `json:"private_ip"`
	PublicIP          string   `json:"public_ip"`
	Region            string   `json:"region"`
	VPCID             string   `json:"vpc_id"`
	Hostname          string   `json:"hostname"`
	LastSeen          int64    `json:"last_seen"`
	AssignedIP        string   `json:"assigned_ip"`
	Role              string   `json:"role"`
	RuntimeMode       string   `json:"runtime_mode,omitempty"`
	KernelVersion     string   `json:"kernel_version,omitempty"`
	Status            string   `json:"status,omitempty"`            // online, offline, stale (never send deleted)
	AdvertisedRoutes  []string `json:"advertised_routes,omitempty"` // Site-to-Site VPN
	EnrolledWithToken string   `json:"enrolled_with_token,omitempty"`
}

// SyncResponse is the response for sync and register endpoints
type SyncResponse struct {
	Peers               []NodeInfo    `json:"peers"`
	AssignedIP          string        `json:"assigned_ip"`
	LastUpdate          int64         `json:"last_update"`
	ACLRules            []ACLRuleJSON `json:"acl_rules,omitempty"`            // Firewall ACL rules
	MetricsPushGateway  string        `json:"metrics_push_gateway,omitempty"` // VictoriaMetrics push gateway URL
	CertificatePEM      string        `json:"certificate_pem,omitempty"`
	CertificateCA       string        `json:"certificate_ca,omitempty"`
	CertificateNotAfter int64         `json:"certificate_not_after,omitempty"`
}

// ACLRuleJSON represents an ACL rule in API responses.
type ACLRuleJSON struct {
	SrcNet   string `json:"src_net"`  // Source CIDR
	DstNet   string `json:"dst_net"`  // Destination CIDR
	Protocol uint8  `json:"protocol"` // IP protocol (6=TCP, 17=UDP, 0=any)
	MinPort  uint16 `json:"min_port"` // Min port (0=any)
	MaxPort  uint16 `json:"max_port"` // Max port (65535=any)
	Action   string `json:"action"`   // allow or deny
}

func runControllerServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	configData, err := os.ReadFile(serveConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", serveConfigPath, err)
	}

	var cfg ControllerConfig
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Network.BaseIP == "" {
		cfg.Network.BaseIP = "100.64.0.0"
	}
	if cfg.Network.CIDR == "" {
		cfg.Network.CIDR = "100.64.0.0/16"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Dir == "" {
		cfg.Logging.Dir = "/var/log/aria"
	}

	// Initialize logger
	logger, err := logging.NewLogger(&logging.Config{
		Level:       logging.ParseLevel(cfg.Logging.Level),
		Component:   "controller",
		LogDir:      cfg.Logging.Dir,
		EnableColor: true,
	})
	if err != nil {
		fmt.Printf("Warning: Failed to initialize file logging: %v, using console only\n", err)
		logger = logging.GetLogger()
	}
	defer logger.Close()

	logger.Info("========================================")
	logger.Info("Aria Controller Starting")
	logger.Info("========================================")
	logger.Info("Version: %s", Version)
	logger.Info("Config: %s", serveConfigPath)
	logger.Info("Log Level: %s", cfg.Logging.Level)

	// Initialize JWT secret from config or environment.
	jwtSecret := strings.TrimSpace(cfg.JWT.Secret)
	if jwtSecret == "" {
		jwtSecret = strings.TrimSpace(os.Getenv("ARIA_JWT_SECRET"))
	}
	if jwtSecret == "" {
		return fmt.Errorf("jwt secret is required, please set jwt.secret or ARIA_JWT_SECRET: %w", auth.ErrJWTSecretNotConfigured)
	}
	auth.SetSecret(jwtSecret)
	logger.Info("JWT secret loaded")

	if err := auth.LoadRuntimeSecretFromEnv(); err != nil {
		return fmt.Errorf("runtime token secret is required, please set ARIA_RUNTIME_TOKEN_SECRET: %w", err)
	}
	logger.Info("Runtime token secret loaded from environment")

	// Check required config
	if cfg.Storage.Postgres.Host == "" {
		return fmt.Errorf("PostgreSQL host is required in config")
	}

	// Create storage
	pgCfg := &controllerstorage.Config{
		Host:     cfg.Storage.Postgres.Host,
		Port:     cfg.Storage.Postgres.Port,
		User:     cfg.Storage.Postgres.User,
		Password: cfg.Storage.Postgres.Password,
		Database: cfg.Storage.Postgres.Database,
		SSLMode:  cfg.Storage.Postgres.SSLMode,
	}

	logger.Info("PostgreSQL: %s:%d/%s", pgCfg.Host, pgCfg.Port, pgCfg.Database)

	store, err := controllerstorage.NewStorage(pgCfg, cfg.Network.BaseIP, cfg.Network.CIDR)
	if err != nil {
		return fmt.Errorf("failed to create storage: %w", err)
	}
	defer store.Close()
	logger.Info("Storage initialized successfully")

	if err := ensureDefaultTenant(store, logger); err != nil {
		return fmt.Errorf("failed to ensure default tenant: %w", err)
	}

	if err := ensureSuperAdmin(store.DB(), logger); err != nil {
		return fmt.Errorf("failed to ensure super admin: %w", err)
	}

	// Create token store
	tokenStore := token.NewStore(store.DB())
	if err := tokenStore.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate token table: %w", err)
	}
	logger.Info("Token store initialized")

	tokenValidator := token.NewValidator(tokenStore)

	// Connect to Redis if configured
	var heartbeat *controllerstorage.HeartbeatStore
	if cfg.Storage.Redis.Addr != "" {
		logger.Info("Redis: %s", cfg.Storage.Redis.Addr)
		heartbeat, err = controllerstorage.NewHeartbeatStore(cfg.Storage.Redis.Addr, cfg.Storage.Redis.Password, 0)
		if err != nil {
			logger.Warn("Failed to connect to Redis: %v", err)
		} else {
			logger.Info("Connected to Redis successfully")
		}
	}

	logger.Info("Network: %s (base: %s)", cfg.Network.CIDR, cfg.Network.BaseIP)

	// Set default metrics push gateway if not configured
	metricsPushGateway := cfg.Metrics.PushGateway
	if metricsPushGateway == "" {
		// Default to Controller's VictoriaMetrics
		metricsPushGateway = "http://127.0.0.1:8428/api/v1/import/prometheus"
	}
	logger.Info("Metrics Push Gateway: %s", metricsPushGateway)

	controller := &Controller{
		store:              store,
		tenantScopedStore:  middleware.NewTenantScopedStorage(store), // Initialize tenant-scoped storage
		heartbeat:          heartbeat,
		tokenStore:         tokenStore,
		tokenValidator:     tokenValidator,
		certService:        initCertIssuanceService(logger),
		baseIP:             cfg.Network.BaseIP,
		cidr:               cfg.Network.CIDR,
		metricsPushGateway: metricsPushGateway,
		logger:             logger,
	}

	// Start cleanup routine
	controller.StartCleanupRoutine()

	// Initialize AI Simple handler (MVP) - 使用独立的 MVP 实现

	// Initialize Bandwidth Management API with tenant awareness
	// Set up HTTP handlers using a local mux (avoid polluting http.DefaultServeMux)
	mux := http.NewServeMux()
	// Southbound API (Agent 南向接口) — v2 路径
	mux.HandleFunc("/api/v2/agents/register", controller.HandleRegister)
	mux.HandleFunc("/api/v2/agents/unregister", controller.HandleUnregister)
	mux.HandleFunc("/api/v2/agents/network", controller.HandleNetworkManage)
	mux.HandleFunc("/api/v2/agents/certificates/issue", controller.HandleIssueCertificate)
	mux.HandleFunc("/api/v2/agents/certificates/renew", controller.HandleRenewCertificate)
	mux.HandleFunc("/api/version", handleVersion)

	// Initialize API v2 skeleton
	// Derive VictoriaMetrics query base URL from push gateway
	vmBaseURL := "http://localhost:8428"
	if strings.Contains(metricsPushGateway, "/api/v1/import/prometheus") {
		vmBaseURL = strings.TrimSuffix(metricsPushGateway, "/api/v1/import/prometheus")
	}
	vmClient := victoriametrics.NewClient(vmBaseURL)
	v2.SetupRoutes(mux, store, vmClient)

	// AI handlers (Production)
	if cfg.AI.Enabled {
		// 初始化 AI Service (生产级架构，注入真实数据源)
		aiService := service.NewAIService(store)

		// Update v2 router with AI service if needed (v2.SetupRoutes already creates a handler, we might want to override)
		// Actually, let's keep the existing v2.SetupRoutes as it is for now.

		// DingTalk Integration
		if cfg.DingTalk.Enabled {
			controller.dingtalkHandler = im.NewDingTalkHandler(aiService, cfg.DingTalk.Webhook, cfg.DingTalk.Secret)
			mux.HandleFunc("/api/v2/integrations/dingtalk/webhook", controller.dingtalkHandler.HandleWebhook)
			logger.Info("DingTalk integration enabled: /api/v2/integrations/dingtalk/webhook")
		}

		// Feishu Integration
		if cfg.Feishu.Enabled {
			controller.feishuHandler = im.NewFeishuHandler(aiService, cfg.Feishu.AppID, cfg.Feishu.AppSecret, cfg.Feishu.EncryptKey, cfg.Feishu.VerifyToken)
			mux.HandleFunc("/api/v2/integrations/feishu/webhook", controller.feishuHandler.HandleWebhook)
			logger.Info("Feishu integration enabled: /api/v2/integrations/feishu/webhook")
		}
	}

	listenAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info("HTTP API listening on %s", listenAddr)
	logger.Info("Southbound: /api/v2/agents/register, /unregister, /network")
	logger.Info("Northbound: /api/v2/auth/*, /api/v2/tenants/*")
	logger.Info("========================================")
	logger.Info("Controller ready")
	logger.Info("========================================")

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received signal: %v", sig)
		controller.Close()
		os.Exit(0)
	}()

	// ========== Start gRPC Server ==========
	grpcPort := 50051
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port %d: %w", grpcPort, err)
	}

	// Load TLS configuration for gRPC
	var grpcServerOpts []grpc.ServerOption

	// TLS mode: disabled (plaintext), server (one-way TLS), mutual (mTLS)
	tlsMode := getEnvOrDefault("ARIA_GRPC_TLS_MODE", "server")

	if tlsMode == "mutual" {
		logger.Info("gRPC mTLS (mutual TLS) enabled, loading certificates...")

		// Load server certificate
		serverCertPath := getEnvOrDefault("ARIA_GRPC_SERVER_CERT", "/etc/aria/certs/server.crt")
		serverKeyPath := getEnvOrDefault("ARIA_GRPC_SERVER_KEY", "/etc/aria/certs/server.key")
		caCertPath := getEnvOrDefault("ARIA_GRPC_CA_CERT", "/etc/aria/certs/ca.crt")

		serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			return fmt.Errorf("failed to load server certificate: %w", err)
		}

		// Load CA certificate for client verification
		caCert, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			return fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate")
		}

		// Create TLS config with mTLS (mutual TLS)
		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{serverCert},
			ClientAuth:         tls.RequireAndVerifyClientCert,
			ClientCAs:          caCertPool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false,
		}

		// Create gRPC credentials
		creds := credentials.NewTLS(tlsConfig)
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(creds))

		logger.Info("gRPC mTLS configured with server cert: %s, CA: %s", serverCertPath, caCertPath)
	} else if tlsMode == "server" {
		logger.Info("gRPC one-way TLS enabled, loading server certificate...")

		// Load server certificate only (one-way TLS)
		serverCertPath := getEnvOrDefault("ARIA_GRPC_SERVER_CERT", "/etc/aria/certs/grpc-server.crt")
		serverKeyPath := getEnvOrDefault("ARIA_GRPC_SERVER_KEY", "/etc/aria/certs/grpc-server.key")

		serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
		if err != nil {
			return fmt.Errorf("failed to load server certificate: %w", err)
		}

		// Create TLS config without client certificate verification
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.NoClientCert,
			MinVersion:   tls.VersionTLS12,
		}

		// Create gRPC credentials
		creds := credentials.NewTLS(tlsConfig)
		grpcServerOpts = append(grpcServerOpts, grpc.Creds(creds))

		logger.Info("gRPC one-way TLS configured with server cert: %s", serverCertPath)
	} else {
		logger.Warn("⚠️  gRPC TLS disabled - using plaintext (not recommended for production)")
	}

	// Create gRPC server with TLS and auth interceptors
	grpcServerOpts = append(grpcServerOpts,
		grpc.UnaryInterceptor(grpcserver.UnaryAuthInterceptor(controller.store)),
		grpc.StreamInterceptor(grpcserver.StreamAuthInterceptor(controller.store)),
	)
	grpcSrv := grpc.NewServer(grpcServerOpts...)
	grpcController := grpcserver.NewControllerServer(
		controller.createRegisterAdapter(),
		controller.createSyncAdapter(),
		controller.store,
	)
	agentpb.RegisterControllerServiceServer(grpcSrv, grpcController)

	// Start gRPC server in goroutine
	go func() {
		logger.Info("gRPC server listening on :%d (TLS: %s)", grpcPort, tlsMode)
		if err := grpcSrv.Serve(grpcListener); err != nil {
			logger.Error("gRPC server error: %v", err)
		}
	}()

	// ========== Start HTTP Server ==========
	logger.Info("HTTP server listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}

func initCertIssuanceService(logger *logging.Logger) *certissuance.Service {
	caCertPath := getEnvOrDefault("ARIA_CERT_CA_CERT", "/etc/aria/certs/ca.crt")
	caKeyPath := getEnvOrDefault("ARIA_CERT_CA_KEY", "/etc/aria/certs/ca.key")
	caCertPEM, certErr := os.ReadFile(caCertPath)
	caKeyPEM, keyErr := os.ReadFile(caKeyPath)
	if certErr != nil || keyErr != nil {
		if logger != nil {
			logger.Warn("Certificate issuance disabled: CA files not ready (cert=%v key=%v)", certErr, keyErr)
		}
		return nil
	}

	validity := parseDurationEnv("ARIA_CERT_VALIDITY", 30*24*time.Hour)
	renewBefore := parseDurationEnv("ARIA_CERT_RENEW_BEFORE", 72*time.Hour)
	svc, err := certissuance.NewServiceFromPEM(caCertPEM, caKeyPEM, validity, renewBefore)
	if err != nil {
		if logger != nil {
			logger.Warn("Certificate issuance disabled: %v", err)
		}
		return nil
	}
	if logger != nil {
		logger.Info("Certificate issuance enabled (validity=%s renew_before=%s)", validity, renewBefore)
	}
	return svc
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func (c *Controller) Close() {
	c.logger.Info("Shutting down controller...")
	if c.heartbeat != nil {
		c.heartbeat.Close()
		c.logger.Debug("Redis connection closed")
	}
	c.store.Close()
	c.logger.Debug("Database connection closed")
	c.logger.Info("Controller shutdown complete")
}

func (c *Controller) StartCleanupRoutine() {
	c.logger.Info("Starting stale node cleanup routine (interval: 30s)")
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			c.cleanupStaleNodes()
			c.reconcileCertificateLifecycle()
			c.tokenStore.CleanupExpired()
		}
	}()
}

func (c *Controller) cleanupStaleNodes() {
	// 1. Query nodes that are about to go offline (last_seen > 60s, currently online)
	goingOffline, err := c.store.GetNodesGoingOffline(60)
	if err != nil {
		c.logger.Error("Failed to query nodes going offline: %v", err)
		// Continue with cleanup even if this query fails
		goingOffline = nil
	}

	// 2. Query nodes that have recovered (last_seen <= 60s, currently offline)
	recovering, err := c.store.GetNodesRecovering(60)
	if err != nil {
		c.logger.Error("Failed to query recovering nodes: %v", err)
		recovering = nil
	}

	// 3. Run the existing cleanup routine
	count, err := c.store.CleanupStaleNodes(120)
	if err != nil {
		c.logger.Error("Failed to cleanup stale nodes: %v", err)
		return
	}
	if count > 0 {
		c.logger.Info("Cleaned up %d stale nodes", count)
	}

	// 4. Generate alerts and audit events for newly offline nodes
	for _, node := range goingOffline {
		if err := c.store.GenerateNodeOfflineAlert(node.TenantID, node.ID, node.Hostname); err != nil {
			c.logger.Error("Failed to generate offline alert for node %s: %v", node.Hostname, err)
		}
		// Create node_offline audit event
		nodeID := node.ID
		auditEvent := &controllerstorage.AuditEvent{
			TenantID:  node.TenantID,
			NodeID:    &nodeID,
			EventType: "node_offline",
			Actor:     "system",
			Summary:   fmt.Sprintf("节点 %s 离线", node.Hostname),
		}
		if _, err := c.store.CreateAuditEvent(auditEvent); err != nil {
			c.logger.Error("Failed to create node_offline audit event for node %s: %v", node.Hostname, err)
		}
	}

	// 5. Handle recovering nodes: resolve alerts, update status, create audit events
	for _, node := range recovering {
		if err := c.store.ResolveNodeOfflineAlert(node.TenantID, node.ID); err != nil {
			c.logger.Error("Failed to resolve offline alert for node %s: %v", node.Hostname, err)
		}
		// Update node status to online
		if err := c.store.MarkNodeOnline(node.ID); err != nil {
			c.logger.Error("Failed to mark node %s as online: %v", node.Hostname, err)
		}
		// Create node_online audit event
		nodeID := node.ID
		auditEvent := &controllerstorage.AuditEvent{
			TenantID:  node.TenantID,
			NodeID:    &nodeID,
			EventType: "node_online",
			Actor:     "system",
			Summary:   fmt.Sprintf("节点 %s 恢复在线", node.Hostname),
		}
		if _, err := c.store.CreateAuditEvent(auditEvent); err != nil {
			c.logger.Error("Failed to create node_online audit event for node %s: %v", node.Hostname, err)
		}
	}
}

func (c *Controller) reconcileCertificateLifecycle() {
	if c.certService == nil {
		return
	}

	expired, err := c.store.MarkExpiredNodeCertificates(time.Now())
	if err != nil {
		c.logger.Error("Failed to mark expired node certificates: %v", err)
	} else {
		for _, cert := range expired {
			node, nodeErr := c.store.GetNodeByID(cert.NodeID)
			if nodeErr != nil {
				c.logger.Error("Failed to load node %s for expired certificate handling: %v", cert.NodeID, nodeErr)
				continue
			}
			hostname := cert.NodeID.String()
			if node != nil && node.Hostname != "" {
				hostname = node.Hostname
			}
			if node != nil {
				if err := c.store.ResolveCertificateExpiringAlert(node.TenantID, node.ID); err != nil {
					c.logger.Warn("Failed to resolve certificate_expiring alert for node %s: %v", hostname, err)
				}
				if err := c.store.GenerateCertificateExpiredAlert(node.TenantID, node.ID, hostname, cert.NotAfter); err != nil {
					c.logger.Warn("Failed to generate certificate_expired alert for node %s: %v", hostname, err)
				}
				nodeID := node.ID
				if _, err := c.store.CreateAuditEvent(&controllerstorage.AuditEvent{
					TenantID:  node.TenantID,
					NodeID:    &nodeID,
					EventType: "certificate_expired",
					Actor:     "system",
					Summary:   fmt.Sprintf("节点 %s 证书已过期", hostname),
					Detail: map[string]interface{}{
						"serial_number": cert.SerialNumber,
						"not_after":     cert.NotAfter.UTC().Format(time.RFC3339),
					},
				}); err != nil {
					c.logger.Warn("Failed to create certificate_expired audit event for node %s: %v", hostname, err)
				}
			}
		}
	}

	deadline := time.Now().Add(c.certService.RenewBefore())
	candidates, err := c.store.ListCertificatesExpiringBefore(deadline)
	if err != nil {
		c.logger.Error("Failed to list expiring node certificates: %v", err)
		return
	}
	for _, candidate := range candidates {
		tenantID, parseTenantErr := uuid.Parse(candidate.TenantID)
		nodeID, parseNodeErr := uuid.Parse(candidate.NodeID)
		if parseTenantErr != nil || parseNodeErr != nil {
			c.logger.Warn("Skipping certificate renewal candidate with invalid ids tenant=%q node=%q", candidate.TenantID, candidate.NodeID)
			continue
		}
		if err := c.store.GenerateCertificateExpiringAlert(tenantID, nodeID, candidate.Hostname, candidate.NotAfter); err != nil {
			c.logger.Warn("Failed to generate certificate_expiring alert for node %s: %v", candidate.Hostname, err)
		}
	}
}

func (c *Controller) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.logger.Warn("Invalid register request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.PublicKey == "" {
		c.logger.Warn("Registration rejected: empty public key from %s", req.Hostname)
		http.Error(w, "Public key required", http.StatusBadRequest)
		return
	}

	// ========== Token Validation ==========
	// 检查是否为已注册节点（重新注册以同步配置）
	existingNode, _ := c.store.GetNode(req.PublicKey)
	isReRegistration := (existingNode != nil)
	requiresFreshEnrollment := nodeRequiresFreshEnrollment(existingNode)
	var requestedTenantID uuid.UUID

	if nodeRegistrationForbidden(existingNode) {
		c.logger.Warn("Registration rejected: node %s status is %s", previewString(req.PublicKey, 8), existingNode.Status)
		http.Error(w, "Node registration is disabled", http.StatusForbidden)
		return
	}

	// 正常已注册节点可以无 token 重新注册；删除后的节点必须重新携带有效注册 token。
	if !isReRegistration || requiresFreshEnrollment {
		if req.Token == "" {
			c.logger.Warn("Registration rejected: no token provided from %s", req.Hostname)
			http.Error(w, "Token required", http.StatusUnauthorized)
			return
		}

		// Validate token for new registration
		tkn, err := c.tokenValidator.Validate(req.Token)
		if err != nil {
			c.logger.Warn("Registration rejected: %v from %s", err, req.Hostname)
			switch err {
			case token.ErrTokenNotFound:
				http.Error(w, "Invalid token", http.StatusUnauthorized)
			case token.ErrTokenExpired:
				http.Error(w, "Token expired", http.StatusUnauthorized)
			case token.ErrTokenExhausted:
				http.Error(w, "Token exhausted (max uses reached)", http.StatusUnauthorized)
			case token.ErrTokenRevoked:
				http.Error(w, "Token revoked", http.StatusUnauthorized)
			default:
				http.Error(w, "Invalid token", http.StatusUnauthorized)
			}
			return
		}

		tokenPreview := tkn.Token
		if len(tokenPreview) > 12 {
			tokenPreview = tokenPreview[:12]
		}
		c.logger.Debug("Token validated: %s (tag: %s)", tokenPreview, tkn.Tag)

		resolvedTenantID, err := c.store.GetTenantIDByToken(req.Token)
		if err != nil {
			c.logger.Error("Failed to get tenant ID by token: %v", err)
			http.Error(w, "Failed to get tenant ID from token", http.StatusInternalServerError)
			return
		}
		requestedTenantID = resolvedTenantID
		if requiresFreshEnrollment && requestedTenantID != existingNode.TenantID {
			c.logger.Warn("Registration rejected: deleted node %s attempted to switch tenant from %s to %s",
				previewString(req.PublicKey, 8), existingNode.TenantID, requestedTenantID)
			http.Error(w, "Node tenant ownership is immutable", http.StatusForbidden)
			return
		}
	} else {
		c.logger.Debug("Re-registration from existing node: %s", previewString(req.PublicKey, 8))
		if req.Token != "" {
			resolvedTenantID, err := c.store.GetTenantIDByToken(req.Token)
			if err != nil {
				c.logger.Error("Failed to get tenant ID by token: %v", err)
				http.Error(w, "Failed to get tenant ID from token", http.StatusInternalServerError)
				return
			}
			requestedTenantID = resolvedTenantID
			if requestedTenantID != existingNode.TenantID {
				c.logger.Warn("Registration rejected: node %s attempted to switch tenant from %s to %s",
					previewString(req.PublicKey, 8), existingNode.TenantID, requestedTenantID)
				http.Error(w, "Node tenant ownership is immutable", http.StatusForbidden)
				return
			}
		}
	}

	// ========== Normal Registration Flow ==========
	publicIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(publicIP); err == nil {
		publicIP = host
	}

	if req.PublicIP == "" {
		req.PublicIP = publicIP
	}

	c.logger.Debug("Processing registration: hostname=%s, publicIP=%s, publicKey=%s...",
		req.Hostname, req.PublicIP, previewString(req.PublicKey, 8))

	var assignedIP string
	var ipOffset int

	existingNode, err := c.store.GetNode(req.PublicKey)
	if err == nil && existingNode != nil {
		assignedIP = existingNode.AssignedIP
		ipOffset = existingNode.IPOffset
		c.logger.Info("Node re-registered: %s (hostname=%s), reusing IP: %s",
			previewString(req.PublicKey, 8), req.Hostname, assignedIP)
		// Preserve existing advertised routes if not provided in re-registration
		if len(req.AdvertisedRoutes) == 0 && len(existingNode.AdvertisedRoutes) > 0 {
			req.AdvertisedRoutes = existingNode.AdvertisedRoutes
			c.logger.Info("Preserving existing advertised routes for node %s: %v",
				req.Hostname, req.AdvertisedRoutes)
		}
	} else {
		// Try atomic hostname reuse with tenant isolation
		if requestedTenantID != uuid.Nil {
			reusedIP, reusedOffset, err := c.store.ReuseHostnameIP(req.Hostname, requestedTenantID)
			if err == nil {
				assignedIP = reusedIP
				ipOffset = reusedOffset
				c.logger.Info("Atomically reused IP for hostname %s: %s", req.Hostname, assignedIP)
			}
			// err == sql.ErrNoRows means no matching hostname, proceed to new allocation
		}

		if assignedIP == "" {
			offset, err := c.store.GetNextAvailableOffset()
			if err != nil {
				c.logger.Error("Failed to get next available offset: %v", err)
				http.Error(w, "Failed to allocate IP", http.StatusInternalServerError)
				return
			}
			ipOffset = offset

			newIP, err := c.store.CalculateIP(ipOffset)
			if err != nil {
				c.logger.Error("Failed to calculate IP for offset %d: %v", ipOffset, err)
				http.Error(w, "Failed to allocate IP", http.StatusInternalServerError)
				return
			}
			assignedIP = newIP
			c.logger.Info("New node registered: %s (hostname=%s), assigned IP: %s (offset=%d)",
				previewString(req.PublicKey, 8), req.Hostname, assignedIP, ipOffset)
		}
	}

	// Get tenant ID from token
	var tenantID uuid.UUID
	if existingNode != nil {
		tenantID = existingNode.TenantID
	} else if requestedTenantID != uuid.Nil {
		tenantID = requestedTenantID
	} else {
		// Default to system tenant if no token provided (should not happen)
		tenantID, err = c.store.GetOrCreateTenant("default")
		if err != nil {
			c.logger.Error("Failed to get default tenant: %v", err)
			http.Error(w, "Failed to assign tenant", http.StatusInternalServerError)
			return
		}
	}

	node := &controllerstorage.Node{
		PublicKey:         req.PublicKey,
		MachineID:         req.MachineID,
		TenantID:          tenantID,
		Endpoint:          req.Endpoint,
		PrivateIP:         req.PrivateIP,
		PublicIP:          req.PublicIP,
		Region:            req.Region,
		VPCID:             req.VPCID,
		Hostname:          req.Hostname,
		AssignedIP:        assignedIP,
		IPOffset:          ipOffset,
		LastSeen:          time.Now().Unix(),
		RegisteredAt:      req.RegisteredAt,
		RuntimeMode:       req.RuntimeMode,
		KernelVersion:     req.KernelVersion,
		HasAESNI:          req.HasAESNI,
		AdvertisedRoutes:  req.AdvertisedRoutes, // Site-to-Site VPN
		EnrolledWithToken: req.Token,            // Track which token was used
	}

	if err := c.store.SaveNode(node); err != nil {
		c.logger.Error("Failed to save node %s: %v", req.Hostname, err)
		http.Error(w, "Failed to save node", http.StatusInternalServerError)
		return
	}

	if c.certService != nil && req.CSRPEM != "" {
		if _, err := c.issueNodeCertificate(node, req.CSRPEM, nil); err != nil {
			c.logger.Error("Certificate issuance during register failed for node %s: %v", previewString(req.PublicKey, 8), err)
			if !isReRegistration || requiresFreshEnrollment {
				if markErr := c.store.MarkNodeDeleted(req.PublicKey); markErr != nil {
					c.logger.Warn("Failed to roll back node %s after certificate issuance failure: %v", previewString(req.PublicKey, 8), markErr)
				}
			}
			http.Error(w, "Failed to issue node certificate", http.StatusInternalServerError)
			return
		}
	}

	// ========== Consume Token ==========
	if req.Token != "" && (!isReRegistration || requiresFreshEnrollment) {
		if err := c.tokenValidator.ConsumeToken(req.Token, req.PublicKey); err != nil {
			c.logger.Warn("Failed to consume token: %v", err)
			// Don't fail the registration, just log
		}
	}

	c.syncNode(&req, assignedIP, w)
}

func (c *Controller) syncNode(req *RegisterRequest, assignedIP string, w http.ResponseWriter) {
	// 获取发起请求的节点的租户ID
	nodeTenantID, err := c.store.GetTenantIDByToken(req.Token)
	if err != nil {
		// For re-registration without token, get the tenant from the existing node
		if req.Token == "" {
			existingNode, _ := c.store.GetNode(req.PublicKey)
			if existingNode != nil {
				nodeTenantID = existingNode.TenantID
			} else {
				c.logger.Error("Failed to get tenant ID by token and no existing node found: %v", err)
				http.Error(w, "Failed to get tenant information", http.StatusInternalServerError)
				return
			}
		} else {
			c.logger.Error("Failed to get tenant ID by token: %v", err)
			http.Error(w, "Failed to get tenant information", http.StatusInternalServerError)
			return
		}
	}

	// 只获取同租户的节点
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1`
	rows, err := c.store.DB().Query(query, nodeTenantID)
	if err != nil {
		c.logger.Error("Failed to get peers for tenant %s: %v", nodeTenantID, err)
		peers := []*controllerstorage.Node{}
		var peerInfos []NodeInfo
		for _, peer := range peers {
			if peer.PublicKey != req.PublicKey {
				role := peer.Role
				if role == "" {
					role = "spoke"
				}
				peerInfos = append(peerInfos, NodeInfo{
					PublicKey:     peer.PublicKey,
					Endpoint:      fmt.Sprintf("%s:51820", peer.PublicIP),
					PrivateIP:     peer.PrivateIP,
					PublicIP:      peer.PublicIP,
					Region:        peer.Region,
					VPCID:         peer.VPCID,
					Hostname:      peer.Hostname,
					LastSeen:      peer.LastSeen,
					AssignedIP:    peer.AssignedIP,
					Role:          role,
					RuntimeMode:   peer.RuntimeMode,
					KernelVersion: peer.KernelVersion,
				})
			}
		}

		// Get enabled ACL rules for sync (filtered by region and tenant)
		var aclRulesJSON []ACLRuleJSON
		var aclRules []*controllerstorage.ACLRule
		var err error

		// Use tenant-scoped ACL rules if available, fallback to global if not
		if c.tenantScopedStore != nil {
			// Create a temporary context with the node's tenant ID
			tempCtx := context.WithValue(context.Background(), middleware.TenantIDKey, nodeTenantID)
			aclRules, err = c.getTenantEnabledACLRules(tempCtx)
		} else {
			// Fallback to global ACL rules
			aclRules, err = c.store.GetEnabledACLRules()
		}

		if err != nil {
			c.logger.Error("Failed to get ACL rules: %v", err)
		} else {
			// Filter rules by region
			aclRulesJSON = c.getACLRulesForRegion(req.Region, aclRules)
			// Safe truncation of public key
			pk := req.PublicKey
			if len(pk) > 16 {
				pk = pk[:16]
			}
			c.logger.Info("Agent %s (Region: %s): sending %d ACL rules", pk, req.Region, len(aclRulesJSON))
		}

		response := SyncResponse{
			Peers:              peerInfos,
			AssignedIP:         assignedIP,
			LastUpdate:         time.Now().Unix(),
			ACLRules:           aclRulesJSON,
			MetricsPushGateway: c.metricsPushGateway,
		}
		c.attachNodeCertificateToSyncResponse(req.PublicKey, &response)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}
	defer rows.Close()

	var peerInfos []NodeInfo
	for rows.Next() {
		peer, err := c.store.ScanNodeRows(rows)
		if err != nil {
			c.logger.Error("Failed to scan peer: %v", err)
			continue
		}

		// Skip the requesting node itself
		if peer.PublicKey != req.PublicKey {
			role := peer.Role
			if role == "" {
				role = "spoke"
			}
			peerInfos = append(peerInfos, NodeInfo{
				PublicKey:     peer.PublicKey,
				Endpoint:      fmt.Sprintf("%s:51820", peer.PublicIP),
				PrivateIP:     peer.PrivateIP,
				PublicIP:      peer.PublicIP,
				Region:        peer.Region,
				VPCID:         peer.VPCID,
				Hostname:      peer.Hostname,
				LastSeen:      peer.LastSeen,
				AssignedIP:    peer.AssignedIP,
				Role:          role,
				RuntimeMode:   peer.RuntimeMode,
				KernelVersion: peer.KernelVersion,
			})
		}
	}

	// Get enabled ACL rules for sync (filtered by tenant and region)
	var aclRulesJSON []ACLRuleJSON
	aclRules, err := c.store.GetEnabledACLRulesByTenant(nodeTenantID)
	if err != nil {
		c.logger.Error("Failed to get ACL rules for tenant %s: %v", nodeTenantID, err)
	} else {
		// Filter rules by region
		aclRulesJSON = c.getACLRulesForRegion(req.Region, aclRules)
		// Safe truncation of public key
		pk := req.PublicKey
		if len(pk) > 16 {
			pk = pk[:16]
		}
		c.logger.Info("Agent %s (Region: %s): sending %d ACL rules", pk, req.Region, len(aclRulesJSON))
	}

	response := SyncResponse{
		Peers:              peerInfos,
		AssignedIP:         assignedIP,
		LastUpdate:         time.Now().Unix(),
		ACLRules:           aclRulesJSON,
		MetricsPushGateway: c.metricsPushGateway,
	}
	c.attachNodeCertificateToSyncResponse(req.PublicKey, &response)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVersion 返回版本信息
func handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"version": Version,
	})
}
func (c *Controller) HandleUnregister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runtimeToken, ok := runtimeBearerTokenFromRequest(r)
	if !ok {
		http.Error(w, "Runtime token required", http.StatusUnauthorized)
		return
	}

	var req struct {
		PublicKey string `json:"public_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	node, ok := c.authorizeRuntimeNodeByPublicKey(w, runtimeToken, req.PublicKey)
	if !ok {
		return
	}
	hostname := node.Hostname
	assignedIP := node.AssignedIP

	// Mark node as deleted (soft delete) instead of hard delete.
	if _, err := c.store.ApplyNodeLifecycleTransition(req.PublicKey, controllerstorage.NodeLifecycleTransition{
		TargetStatus:   "deleted",
		RevokeReason:   "node unregistered",
		AuditEventType: "node_unregistered",
		AuditActor:     "agent",
		AuditSummary:   "Node unregistered",
		AuditDetail: map[string]interface{}{
			"assigned_ip": assignedIP,
		},
	}); err != nil {
		c.logger.Error("Failed to mark node as deleted %s: %v", previewString(req.PublicKey, 8), err)
		http.Error(w, "Failed to delete node", http.StatusInternalServerError)
		return
	}

	c.logger.Info("Node marked as deleted: %s (hostname=%s, IP=%s)", previewString(req.PublicKey, 8), hostname, assignedIP)

	// 阶段3：发布 Redis Pub/Sub 通知所有 agent 删除该 peer
	if c.heartbeat != nil {
		if err := c.heartbeat.PublishNodeDeleted(req.PublicKey, hostname, assignedIP); err != nil {
			c.logger.Error("Failed to publish node deletion event: %v", err)
			// 不返回错误，因为节点已标记删除
		} else {
			c.logger.Debug("Published node deletion event for %s...", previewString(req.PublicKey, 8))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func runtimeBearerTokenFromRequest(r *http.Request) (string, bool) {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if authz == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		authz = strings.TrimSpace(authz[7:])
	}
	return authz, authz != ""
}

func (c *Controller) authorizeRuntimeNodeByPublicKey(w http.ResponseWriter, runtimeToken, publicKey string) (*controllerstorage.Node, bool) {
	if strings.TrimSpace(publicKey) == "" {
		http.Error(w, "Public key required", http.StatusBadRequest)
		return nil, false
	}

	claims, err := auth.ValidateRuntimeToken(runtimeToken)
	if err != nil {
		http.Error(w, "Invalid runtime token", http.StatusUnauthorized)
		return nil, false
	}

	node, err := c.store.GetNode(publicKey)
	if err != nil || node == nil {
		http.Error(w, "Node not found", http.StatusNotFound)
		return nil, false
	}

	tokenNodeID, err := uuid.Parse(claims.NodeID)
	if err != nil {
		http.Error(w, "Invalid runtime token node id", http.StatusUnauthorized)
		return nil, false
	}
	if tokenNodeID != node.ID {
		http.Error(w, "Runtime token node mismatch", http.StatusForbidden)
		return nil, false
	}
	if claims.TenantID != "" && claims.TenantID != node.TenantID.String() {
		http.Error(w, "Runtime token tenant mismatch", http.StatusForbidden)
		return nil, false
	}

	return node, true
}

type certificateIssueRequest struct {
	NodeID       string `json:"node_id,omitempty"`
	PublicKey    string `json:"public_key,omitempty"`
	CSRPEM       string `json:"csr_pem"`
	RuntimeToken string `json:"runtime_token,omitempty"`
	Token        string `json:"token,omitempty"`
}

func (c *Controller) HandleIssueCertificate(w http.ResponseWriter, r *http.Request) {
	c.handleIssueOrRenewCertificate(w, r, false)
}

func (c *Controller) HandleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	c.handleIssueOrRenewCertificate(w, r, true)
}

func (c *Controller) handleIssueOrRenewCertificate(w http.ResponseWriter, r *http.Request, isRenew bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c.certService == nil {
		http.Error(w, "Certificate issuance is not enabled", http.StatusServiceUnavailable)
		return
	}

	var req certificateIssueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.CSRPEM == "" {
		http.Error(w, "csr_pem is required", http.StatusBadRequest)
		return
	}

	node, err := c.resolveCertificateRequestNode(r, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var renewedFrom *uuid.UUID
	if isRenew {
		existing, err := c.store.GetNodeCertificate(node.ID)
		if err != nil {
			c.recordCertificateRenewFailure(node, "failed to load current certificate")
			http.Error(w, "Failed to load current certificate", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			c.recordCertificateRenewFailure(node, "no existing certificate to renew")
			http.Error(w, "No existing certificate to renew", http.StatusNotFound)
			return
		}
		if !strings.EqualFold(strings.TrimSpace(existing.Status), controllerstorage.CertStatusIssued) {
			c.recordCertificateRenewFailure(node, "current certificate is not eligible for renewal")
			http.Error(w, "Current certificate is not eligible for renewal", http.StatusConflict)
			return
		}
		renewedFrom = &existing.ID
	}

	cert, err := c.issueNodeCertificate(node, req.CSRPEM, renewedFrom)
	if err != nil {
		if isRenew {
			c.recordCertificateRenewFailure(node, err.Error())
		}
		http.Error(w, "Failed to issue certificate: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "success",
		"node_id":       node.ID.String(),
		"serial_number": cert.SerialNumber,
		"cert_pem":      cert.CertPEM,
		"ca_pem":        cert.CAPEM,
		"not_before":    cert.NotBefore.Unix(),
		"not_after":     cert.NotAfter.Unix(),
		"renew_before":  int64(c.certService.RenewBefore().Seconds()),
	})
}

func (c *Controller) recordCertificateRenewFailure(node *controllerstorage.Node, message string) {
	if c == nil || c.store == nil || node == nil {
		return
	}
	hostname := node.Hostname
	if strings.TrimSpace(hostname) == "" {
		hostname = node.ID.String()
	}
	if err := c.store.GenerateCertificateRenewFailedAlert(node.TenantID, node.ID, hostname, message); err != nil && c.logger != nil {
		c.logger.Warn("Failed to generate certificate_renew_failed alert for node %s: %v", hostname, err)
	}
	nodeID := node.ID
	if _, err := c.store.CreateAuditEvent(&controllerstorage.AuditEvent{
		TenantID:  node.TenantID,
		NodeID:    &nodeID,
		EventType: "certificate_renew_failed",
		Actor:     "system",
		Summary:   fmt.Sprintf("节点 %s 证书续签失败", hostname),
		Detail: map[string]interface{}{
			"error": message,
		},
	}); err != nil && c.logger != nil {
		c.logger.Warn("Failed to create certificate_renew_failed audit event for node %s: %v", hostname, err)
	}
}

func (c *Controller) resolveCertificateRequestNode(r *http.Request, req *certificateIssueRequest) (*controllerstorage.Node, error) {
	runtimeToken := strings.TrimSpace(req.RuntimeToken)
	if runtimeToken == "" {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			runtimeToken = strings.TrimSpace(authz[7:])
		}
	}
	if runtimeToken != "" {
		claims, err := auth.ValidateRuntimeToken(runtimeToken)
		if err != nil {
			return nil, fmt.Errorf("invalid runtime token")
		}
		nodeID, err := uuid.Parse(claims.NodeID)
		if err != nil {
			return nil, fmt.Errorf("invalid runtime token node id")
		}
		node, err := c.store.GetNodeByID(nodeID)
		if err != nil || node == nil {
			return nil, fmt.Errorf("node not found")
		}
		if claims.TenantID != "" && node.TenantID.String() != claims.TenantID {
			return nil, fmt.Errorf("tenant mismatch")
		}
		if nodeCertificateRequestForbidden(node) {
			return nil, fmt.Errorf("node access denied: current status is '%s'", strings.ToLower(strings.TrimSpace(node.Status)))
		}
		return node, nil
	}

	if req.Token != "" {
		if _, err := c.tokenValidator.Validate(req.Token); err != nil {
			return nil, fmt.Errorf("invalid enrollment token")
		}
		tenantID, err := c.store.GetTenantIDByToken(req.Token)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve tenant by token")
		}
		node, err := c.resolveNodeByRequest(req)
		if err != nil || node == nil {
			return nil, fmt.Errorf("node not found")
		}
		if node.TenantID != tenantID {
			return nil, fmt.Errorf("node tenant mismatch")
		}
		if nodeCertificateRequestForbidden(node) {
			return nil, fmt.Errorf("node access denied: current status is '%s'", strings.ToLower(strings.TrimSpace(node.Status)))
		}
		return node, nil
	}

	return nil, fmt.Errorf("runtime_token or token is required")
}

func (c *Controller) resolveNodeByRequest(req *certificateIssueRequest) (*controllerstorage.Node, error) {
	if req.NodeID != "" {
		id, err := uuid.Parse(req.NodeID)
		if err == nil {
			return c.store.GetNodeByID(id)
		}
	}
	if req.PublicKey != "" {
		return c.store.GetNode(req.PublicKey)
	}
	return nil, nil
}

func (c *Controller) issueNodeCertificate(node *controllerstorage.Node, csrPEM string, renewedFrom *uuid.UUID) (*controllerstorage.NodeCertificate, error) {
	if node == nil {
		return nil, fmt.Errorf("node is required")
	}
	if node.ID == uuid.Nil {
		return nil, fmt.Errorf("node id is required")
	}
	if node.TenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}

	issued, err := c.certService.IssueFromCSR(certissuance.IssueRequest{
		NodeID:   node.ID.String(),
		TenantID: node.TenantID.String(),
		CSRPEM:   []byte(csrPEM),
	})
	if err != nil {
		return nil, err
	}

	certMeta := &controllerstorage.NodeCertificate{
		TenantID:     node.TenantID,
		NodeID:       node.ID,
		SerialNumber: issued.SerialNumber,
		CertPEM:      issued.CertPEM,
		CAPEM:        issued.CAPEM,
		NotBefore:    issued.NotBefore,
		NotAfter:     issued.NotAfter,
		Status:       controllerstorage.CertStatusIssued,
		RenewedFrom:  renewedFrom,
	}
	if err := c.store.UpsertNodeCertificate(certMeta); err != nil {
		return nil, err
	}
	if renewedFrom != nil {
		if err := c.store.ResolveCertificateExpiringAlert(node.TenantID, node.ID); err != nil {
			c.logger.Warn("Failed to resolve certificate_expiring alert for node %s after renewal: %v", node.Hostname, err)
		}
		if err := c.store.ResolveCertificateRenewFailedAlert(node.TenantID, node.ID); err != nil {
			c.logger.Warn("Failed to resolve certificate_renew_failed alert for node %s after renewal: %v", node.Hostname, err)
		}
		nodeID := node.ID
		if _, err := c.store.CreateAuditEvent(&controllerstorage.AuditEvent{
			TenantID:  node.TenantID,
			NodeID:    &nodeID,
			EventType: "certificate_renewed",
			Actor:     "system",
			Summary:   fmt.Sprintf("节点 %s 证书已续签", node.Hostname),
			Detail: map[string]interface{}{
				"serial_number": certMeta.SerialNumber,
				"not_after":     certMeta.NotAfter.UTC().Format(time.RFC3339),
				"renewed_from":  renewedFrom.String(),
			},
		}); err != nil {
			c.logger.Warn("Failed to create certificate_renewed audit event for node %s: %v", node.Hostname, err)
		}
	}
	return c.store.GetNodeCertificate(node.ID)
}

func (c *Controller) attachNodeCertificateToSyncResponse(publicKey string, resp *SyncResponse) {
	if c.certService == nil || resp == nil || publicKey == "" {
		return
	}
	node, err := c.store.GetNode(publicKey)
	if err != nil || node == nil {
		return
	}
	cert, err := c.store.GetNodeCertificate(node.ID)
	if err != nil || cert == nil || cert.Status != controllerstorage.CertStatusIssued {
		return
	}
	resp.CertificatePEM = cert.CertPEM
	resp.CertificateCA = cert.CAPEM
	resp.CertificateNotAfter = cert.NotAfter.Unix()
}

// HandleNetworkManage manages network routes for nodes (Controller-side only)
func (c *Controller) HandleNetworkManage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !c.authorizeJWTPermission(w, r, middleware.PermRoutesWrite) {
		return
	}

	var req struct {
		Hostname string `json:"hostname"`
		CIDR     string `json:"cidr"`
		Action   string `json:"action"` // "add" or "remove"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate action
	if req.Action != "add" && req.Action != "remove" {
		http.Error(w, "action must be 'add' or 'remove'", http.StatusBadRequest)
		return
	}

	// Find node by hostname
	allNodes, err := c.store.GetAllNodes()
	if err != nil {
		c.logger.Error("Failed to get nodes: %v", err)
		http.Error(w, "Failed to get nodes", http.StatusInternalServerError)
		return
	}

	var targetNode *controllerstorage.Node
	for _, node := range allNodes {
		if node.Hostname == req.Hostname {
			targetNode = node
			break
		}
	}

	if targetNode == nil {
		http.Error(w, fmt.Sprintf("Node %s not found", req.Hostname), http.StatusNotFound)
		return
	}

	// Modify advertised routes
	if req.Action == "add" {
		// Validate CIDR format
		_, newNetwork, err := net.ParseCIDR(req.CIDR)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid CIDR format: %s", req.CIDR), http.StatusBadRequest)
			return
		}

		// Check if already exists
		for _, route := range targetNode.AdvertisedRoutes {
			if route == req.CIDR {
				http.Error(w, fmt.Sprintf("Route %s already advertised by %s", req.CIDR, req.Hostname), http.StatusBadRequest)
				return
			}
		}

		// Check for conflicts with existing routes in DIFFERENT regions
		// Same region nodes CAN have overlapping routes (for redundancy/Active-Active)
		// Different region nodes CANNOT have overlapping routes (for isolation)
		for _, node := range allNodes {
			// Skip nodes in the SAME region (allow overlap for redundancy)
			if node.Region == targetNode.Region {
				continue
			}

			// Skip the target node itself
			if node.PublicKey == targetNode.PublicKey {
				continue
			}

			for _, existingRoute := range node.AdvertisedRoutes {
				_, existingNetwork, err := net.ParseCIDR(existingRoute)
				if err != nil {
					continue // Skip invalid routes
				}

				// Check if networks overlap with nodes in DIFFERENT regions
				if cidrsOverlap(newNetwork, existingNetwork) {
					http.Error(w, fmt.Sprintf("请勿在不同 Region 添加重叠路由！路由 %s 与 Region %s 的节点 %s 冲突",
						req.CIDR, node.Region, node.Hostname), http.StatusConflict)
					return
				}
			}
		}

		targetNode.AdvertisedRoutes = append(targetNode.AdvertisedRoutes, req.CIDR)
		c.logger.Info("Added route %s to node %s", req.CIDR, req.Hostname)
	} else {
		// Remove route
		found := false
		newRoutes := make([]string, 0, len(targetNode.AdvertisedRoutes))
		for _, route := range targetNode.AdvertisedRoutes {
			if route == req.CIDR {
				found = true
				continue
			}
			newRoutes = append(newRoutes, route)
		}
		if !found {
			http.Error(w, fmt.Sprintf("Route %s not found on node %s", req.CIDR, req.Hostname), http.StatusNotFound)
			return
		}
		targetNode.AdvertisedRoutes = newRoutes
		c.logger.Info("Removed route %s from node %s", req.CIDR, req.Hostname)
	}

	// Save to database
	if err := c.store.SaveNode(targetNode); err != nil {
		c.logger.Error("Failed to save node: %v", err)
		http.Error(w, "Failed to save node", http.StatusInternalServerError)
		return
	}

	// Publish config change notification via Redis with staggered sync
	if c.heartbeat != nil {
		// Use Redis Pub/Sub to notify all peers
		// Peers will receive the notification and sync at different times
		c.heartbeat.PublishConfigChange("default", "network_update", map[string]string{
			"hostname": req.Hostname,
			"cidr":     req.CIDR,
			"action":   req.Action,
		})
		c.logger.Info("Published network update notification to all peers")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "success",
		"hostname": req.Hostname,
		"cidr":     req.CIDR,
		"action":   req.Action,
	})
}

func (c *Controller) authorizeJWTPermission(w http.ResponseWriter, r *http.Request, permission string) bool {
	tokenString, ok := runtimeBearerTokenFromRequest(r)
	if !ok {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return false
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return false
	}
	if claims.MustChangePassword {
		http.Error(w, "You must change your password before proceeding", http.StatusForbidden)
		return false
	}
	if claims.Role == middleware.RoleSuperAdmin {
		return true
	}
	if claims.TenantID == "" {
		http.Error(w, "Tenant context required", http.StatusForbidden)
		return false
	}
	tenantID, err := uuid.Parse(claims.TenantID)
	if err != nil {
		http.Error(w, "Invalid tenant context", http.StatusForbidden)
		return false
	}

	roleName := claims.Role
	if roleName == "member" || roleName == "owner" {
		roleName = controllerstorage.SystemRoleOperator
	}
	permissions, err := c.store.GetRolePermissions(tenantID, roleName)
	if err != nil {
		http.Error(w, "Role not found", http.StatusForbidden)
		return false
	}
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}

	http.Error(w, "Insufficient permissions", http.StatusForbidden)
	return false
}

// ========== Policy Management Handlers ==========

// PolicyRequest represents a policy creation request (frontend format)
type PolicyRequest struct {
	SrcNode  string `json:"srcNode"`
	SrcIp    string `json:"srcIp"`
	DstNode  string `json:"dstNode"`
	DstIp    string `json:"dstIp"`
	Protocol string `json:"protocol"` // "tcp" or "udp"
	DstPort  string `json:"dstPort"`
	Action   string `json:"action"`
}

// PolicyResponse represents a policy in the API response (frontend format)
type PolicyResponse struct {
	ID       int    `json:"id"`
	SrcNode  string `json:"srcNode"`
	SrcIp    string `json:"srcIp"`
	DstNode  string `json:"dstNode"`
	DstIp    string `json:"dstIp"`
	Protocol string `json:"protocol"` // "tcp", "udp", "icmp"
	DstPort  string `json:"dstPort"`
	Action   string `json:"action"`
}

// HandlePolicies manages ACL policies (CRUD operations)
func (c *Controller) getRegionByNetwork(network string) string {
	nodes, err := c.store.GetAllNodes()
	if err != nil {
		c.logger.Error("Failed to get nodes for region lookup: %v", err)
		return ""
	}

	return c.getRegionByNetworkInNodes(network, nodes)
}

func (c *Controller) getRegionByNetworkInNodes(network string, nodes []*controllerstorage.Node) string {
	// Parse the target network
	_, targetNet, err := net.ParseCIDR(network)
	if err != nil {
		c.logger.Error("Invalid network CIDR %s: %v", network, err)
		return ""
	}

	// Search through all nodes and their advertised routes
	for _, node := range nodes {
		if node.Region == "" {
			continue
		}

		// Check if any advertised route matches or contains the target network
		for _, route := range node.AdvertisedRoutes {
			_, routeNet, err := net.ParseCIDR(route)
			if err != nil {
				continue
			}

			// Check if the route matches or contains the target network
			if route == network || cidrsOverlap(routeNet, targetNet) {
				return node.Region
			}
		}
	}

	return ""
}

// Get enabled ACL rules for sync (filtered by region) - now with tenant isolation
func (c *Controller) getTenantEnabledACLRules(ctx context.Context) ([]*controllerstorage.ACLRule, error) {
	// Use tenant-scoped storage to get rules for current tenant
	if c.tenantScopedStore != nil {
		return c.tenantScopedStore.GetEnabledTenantACLRules(ctx)
	}
	// Fallback to original method if tenant-scoped storage not available
	return c.store.GetEnabledACLRules()
}

// getACLRulesForRegion filters ACL rules relevant to a specific region.
// A rule is relevant if either the source or destination network belongs to the region.
// This supports bidirectional traffic and asymmetric routing.
func (c *Controller) getACLRulesForRegion(region string, allRules []*controllerstorage.ACLRule) []ACLRuleJSON {
	nodes, err := c.store.GetAllNodes()
	if err != nil {
		c.logger.Error("Failed to get nodes for ACL region lookup: %v", err)
		return nil
	}
	return c.getACLRulesForRegionInNodes(region, allRules, nodes)
}

func (c *Controller) getACLRulesForRegionInNodes(region string, allRules []*controllerstorage.ACLRule, nodes []*controllerstorage.Node) []ACLRuleJSON {
	if region == "" {
		// If no region specified, return all rules (backward compatibility)
		c.logger.Warn("Agent has no region, returning all ACL rules")
		result := make([]ACLRuleJSON, 0, len(allRules))
		for _, rule := range allRules {
			result = append(result, ACLRuleJSON{
				SrcNet:   rule.SrcNet,
				DstNet:   rule.DstNet,
				Protocol: rule.Protocol,
				MinPort:  rule.MinPort,
				MaxPort:  rule.MaxPort,
				Action:   defaultRegistrationACLAction(rule.Action),
			})
		}
		return result
	}

	var result []ACLRuleJSON
	for _, rule := range allRules {
		// Find regions for source and destination networks
		srcRegion := c.getRegionByNetworkInNodes(rule.SrcNet, nodes)
		dstRegion := c.getRegionByNetworkInNodes(rule.DstNet, nodes)

		// Include rule if this region is involved (source or destination)
		// This ensures both outbound and inbound traffic are allowed
		if srcRegion == region || dstRegion == region {
			result = append(result, ACLRuleJSON{
				SrcNet:   rule.SrcNet,
				DstNet:   rule.DstNet,
				Protocol: rule.Protocol,
				MinPort:  rule.MinPort,
				MaxPort:  rule.MaxPort,
				Action:   defaultRegistrationACLAction(rule.Action),
			})
		}
	}

	c.logger.Debug("Region %s: filtered %d rules from %d total", region, len(result), len(allRules))
	return result
}

// ========== gRPC Adapters ==========
// 以下两个方法将 REST API handler 适配为 gRPC 所需的格式

// createRegisterAdapter 创建注册适配器
// 将 REST API 的 HandleRegister 逻辑包装成 gRPC 可调用的函数
func (c *Controller) createRegisterAdapter() func(interface{}) (string, string, error) {
	return func(reqInterface interface{}) (string, string, error) {
		reqMap, ok := reqInterface.(map[string]interface{})
		if !ok {
			return "", "", fmt.Errorf("invalid request format")
		}

		// 从 map 中提取字段
		req := &RegisterRequest{
			PublicKey:     getStringFromMap(reqMap, "public_key"),
			Endpoint:      getStringFromMap(reqMap, "endpoint"),
			PrivateIP:     getStringFromMap(reqMap, "private_ip"),
			PublicIP:      getStringFromMap(reqMap, "public_ip"),
			Region:        getStringFromMap(reqMap, "region"),
			VPCID:         getStringFromMap(reqMap, "vpc_id"),
			Hostname:      getStringFromMap(reqMap, "hostname"),
			MachineID:     getStringFromMap(reqMap, "machine_id"),
			RegisteredAt:  getInt64FromMap(reqMap, "registered_at"),
			Token:         getStringFromMap(reqMap, "token"),
			RuntimeMode:   getStringFromMap(reqMap, "runtime_mode"),
			KernelVersion: getStringFromMap(reqMap, "kernel_version"),
			HasAESNI:      getBoolFromMap(reqMap, "has_aesni"),
		}

		// 处理数组字段
		if routes, ok := reqMap["advertised_routes"].([]interface{}); ok {
			req.AdvertisedRoutes = make([]string, 0, len(routes))
			for _, r := range routes {
				if str, ok := r.(string); ok {
					req.AdvertisedRoutes = append(req.AdvertisedRoutes, str)
				}
			}
		}

		// 调用现有的注册逻辑
		assignedIP, err := c.processRegistration(req, "")
		if err != nil {
			return "", "", err
		}

		// 生成 Metrics Push Gateway URL
		// 注意：req 中没有 TenantID，需要从 token 中提取（已在 processRegistration 中处理）
		metricsGateway := c.metricsPushGateway

		return assignedIP, metricsGateway, nil
	}
}

// createSyncAdapter 创建同步适配器
// 将 REST API 的 HandleSync 逻辑包装成 gRPC 可调用的函数
func (c *Controller) createSyncAdapter() func(string) (interface{}, string, interface{}, string, error) {
	return func(publicKey string) (interface{}, string, interface{}, string, error) {
		// 调用现有的同步逻辑（提取自 HandleSync）
		peers, assignedIP, aclRules, tenantID, err := c.processSync(publicKey)
		if err != nil {
			return nil, "", nil, "", err
		}

		// 生成 Metrics Push Gateway URL
		metricsGateway := c.metricsPushGateway
		if tenantID != uuid.Nil {
			metricsGateway = fmt.Sprintf("%s?tenant=%s", c.metricsPushGateway, tenantID)
		}

		return peers, assignedIP, aclRules, metricsGateway, nil
	}
}

// processRegistration 处理注册逻辑（从 HandleRegister 提取）
func (c *Controller) processRegistration(req *RegisterRequest, publicIP string) (string, error) {
	// Token Validation
	existingNode, _ := c.store.GetNode(req.PublicKey)
	isReRegistration := (existingNode != nil)
	requiresFreshEnrollment := nodeRequiresFreshEnrollment(existingNode)
	var requestedTenantID uuid.UUID

	if nodeRegistrationForbidden(existingNode) {
		return "", fmt.Errorf("node registration is disabled")
	}

	if !isReRegistration || requiresFreshEnrollment {
		if req.Token == "" {
			return "", fmt.Errorf("token required")
		}

		tkn, err := c.tokenValidator.Validate(req.Token)
		if err != nil {
			return "", fmt.Errorf("invalid token: %w", err)
		}
		c.logger.Debug("Token validated: %s (tag: %s)", previewString(tkn.Token, 12), tkn.Tag)
		requestedTenantID, err = c.store.GetTenantIDByToken(req.Token)
		if err != nil {
			return "", fmt.Errorf("failed to get tenant ID by token: %w", err)
		}
		if requiresFreshEnrollment && requestedTenantID != existingNode.TenantID {
			return "", fmt.Errorf("node tenant ownership is immutable")
		}
	} else if req.Token != "" {
		var err error
		requestedTenantID, err = c.store.GetTenantIDByToken(req.Token)
		if err != nil {
			return "", fmt.Errorf("failed to get tenant ID by token: %w", err)
		}
		if requestedTenantID != existingNode.TenantID {
			return "", fmt.Errorf("node tenant ownership is immutable")
		}
	}

	// Use provided public IP or empty
	if req.PublicIP == "" {
		req.PublicIP = publicIP
	}

	var assignedIP string
	var ipOffset int

	existingNode, err := c.store.GetNode(req.PublicKey)
	if err == nil && existingNode != nil {
		assignedIP = existingNode.AssignedIP
		ipOffset = existingNode.IPOffset
		c.logger.Info("Node re-registered: %s (hostname=%s), reusing IP: %s",
			previewString(req.PublicKey, 8), req.Hostname, assignedIP)
		if len(req.AdvertisedRoutes) == 0 && len(existingNode.AdvertisedRoutes) > 0 {
			req.AdvertisedRoutes = existingNode.AdvertisedRoutes
		}
	} else {
		// Try atomic hostname reuse with tenant isolation
		if requestedTenantID != uuid.Nil {
			reusedIP, reusedOffset, err := c.store.ReuseHostnameIP(req.Hostname, requestedTenantID)
			if err == nil {
				assignedIP = reusedIP
				ipOffset = reusedOffset
				c.logger.Info("Atomically reused IP for hostname %s: %s", req.Hostname, assignedIP)
			}
		}
	}

	if assignedIP == "" {
		var err error
		ipOffset, err = c.store.GetNextAvailableOffset()
		if err != nil {
			return "", fmt.Errorf("failed to get next offset: %w", err)
		}
		assignedIP, err = c.store.CalculateIP(ipOffset)
		if err != nil {
			return "", fmt.Errorf("failed to calculate IP: %w", err)
		}
	}

	// Extract tenant ID from token
	var tenantID uuid.UUID
	if existingNode != nil {
		tenantID = existingNode.TenantID
	} else if requestedTenantID != uuid.Nil {
		tenantID = requestedTenantID
	} else {
		// Default to system tenant
		tenantID, _ = c.store.GetOrCreateTenant("default")
	}

	node := &controllerstorage.Node{
		PublicKey:        req.PublicKey,
		Endpoint:         req.Endpoint,
		PrivateIP:        req.PrivateIP,
		PublicIP:         req.PublicIP,
		Region:           req.Region,
		VPCID:            req.VPCID,
		Hostname:         req.Hostname,
		MachineID:        req.MachineID,
		AssignedIP:       assignedIP,
		IPOffset:         ipOffset,
		AdvertisedRoutes: req.AdvertisedRoutes,
		RuntimeMode:      req.RuntimeMode,
		KernelVersion:    req.KernelVersion,
		HasAESNI:         req.HasAESNI,
		LastSeen:         time.Now().Unix(),
		RegisteredAt:     req.RegisteredAt,
		Role:             "agent",
		TenantID:         tenantID,
	}

	if existingNode != nil {
		node.RegisteredAt = existingNode.RegisteredAt
	}

	if err := c.store.SaveNode(node); err != nil {
		return "", fmt.Errorf("failed to save node: %w", err)
	}

	if req.Token != "" && (!isReRegistration || requiresFreshEnrollment) {
		if err := c.tokenStore.IncrementUsage(req.Token, req.PublicKey); err != nil {
			c.logger.Warn("Failed to increment token usage: %v", err)
		}
	}

	c.logger.Info("Node registered successfully: %s (hostname=%s, IP=%s, region=%s)",
		previewString(req.PublicKey, 8), req.Hostname, assignedIP, req.Region)

	return assignedIP, nil
}

// processSync 处理同步逻辑（从 HandleSync 提取）
func (c *Controller) processSync(publicKey string) (interface{}, string, interface{}, uuid.UUID, error) {
	node, err := c.store.GetNode(publicKey)
	if err != nil {
		return nil, "", nil, uuid.Nil, fmt.Errorf("node not found: %w", err)
	}

	// Update last seen
	node.LastSeen = time.Now().Unix()
	if err := c.store.SaveNode(node); err != nil {
		c.logger.Warn("Failed to update last seen for %s: %v", previewString(publicKey, 8), err)
	}

	tenantNodes, err := c.store.GetNodesByTenant(node.TenantID)
	if err != nil {
		return nil, "", nil, uuid.Nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	var peers []map[string]interface{}
	for _, n := range tenantNodes {
		if n.PublicKey == publicKey {
			continue
		}
		peers = append(peers, map[string]interface{}{
			"public_key":        n.PublicKey,
			"endpoint":          n.Endpoint,
			"private_ip":        n.PrivateIP,
			"public_ip":         n.PublicIP,
			"region":            n.Region,
			"vpc_id":            n.VPCID,
			"hostname":          n.Hostname,
			"assigned_ip":       n.AssignedIP,
			"role":              n.Role,
			"advertised_routes": n.AdvertisedRoutes,
		})
	}

	allACLRules, err := c.store.GetACLRulesByTenant(node.TenantID)
	if err != nil {
		c.logger.Warn("Failed to get tenant ACL rules for %s: %v", node.TenantID, err)
		allACLRules = []*controllerstorage.ACLRule{}
	}

	enabledACLRules := make([]*controllerstorage.ACLRule, 0, len(allACLRules))
	for _, rule := range allACLRules {
		if rule.Enabled {
			enabledACLRules = append(enabledACLRules, rule)
		}
	}

	regionACLs := c.getACLRulesForRegionInNodes(node.Region, enabledACLRules, tenantNodes)
	var aclRules []map[string]interface{}
	for _, rule := range regionACLs {
		aclRules = append(aclRules, map[string]interface{}{
			"src_net":  rule.SrcNet,
			"dst_net":  rule.DstNet,
			"protocol": rule.Protocol,
			"min_port": rule.MinPort,
			"max_port": rule.MaxPort,
			"action":   defaultRegistrationACLAction(rule.Action),
		})
	}

	return peers, node.AssignedIP, aclRules, node.TenantID, nil
}

// 辅助函数：从 map 中安全获取字符串
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// 辅助函数：从 map 中安全获取 int64
func getInt64FromMap(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

// 辅助函数：从 map 中安全获取 bool
func getBoolFromMap(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// getEnvOrDefault gets environment variable or returns default value

// parsePortRange parses a port range string (e.g., "22", "80-443")
func parsePortRange(portStr string) (uint16, uint16) {
	if portStr == "" {
		return 0, 65535
	}

	// Handle comma-separated ports (e.g., "22,80,443")
	// For now, we'll just use the first port
	// TODO: Support multiple port ranges

	// Handle single port or port range
	var minPort, maxPort uint16
	if _, err := fmt.Sscanf(portStr, "%d-%d", &minPort, &maxPort); err == nil {
		return minPort, maxPort
	}
	if _, err := fmt.Sscanf(portStr, "%d", &minPort); err == nil {
		return minPort, minPort
	}
	return 0, 65535
}

// formatPortRange formats a port range for display
func formatPortRange(minPort, maxPort uint16) string {
	if minPort == 0 && maxPort == 65535 {
		return ""
	}
	if minPort == maxPort {
		return fmt.Sprintf("%d", minPort)
	}
	return fmt.Sprintf("%d-%d", minPort, maxPort)
}

// cidrsOverlap checks if two CIDR networks overlap.
// Returns true if:
//   - One network contains the other
//   - The networks have any IP addresses in common
func cidrsOverlap(a, b *net.IPNet) bool {
	// Check if a contains b's network address
	if a.Contains(b.IP) {
		return true
	}
	// Check if b contains a's network address
	if b.Contains(a.IP) {
		return true
	}
	return false
}

// ipInRoutes checks if an IP address is within any of the given CIDR routes.
func ipInRoutes(ipStr string, routes []string) (bool, string) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false, ""
	}

	for _, route := range routes {
		_, network, err := net.ParseCIDR(route)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true, route
		}
	}
	return false, ""
}

func ensureSuperAdmin(db *sql.DB, logger *logging.Logger) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'super_admin'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check super admin: %w", err)
	}

	if count > 0 {
		logger.Info("Super admin already exists")
		return nil
	}

	username := os.Getenv("ARIA_SUPER_ADMIN")
	password := os.Getenv("ARIA_SUPER_ADMIN_PASSWORD")

	if username == "" {
		username = "sysadmin"
	}
	if password == "" {
		password = "Sysadmin@123"
	}

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = db.Exec(`INSERT INTO users (username, password_hash, role, tenant_id, must_change_password) VALUES ($1, $2, 'super_admin', NULL, TRUE)`,
		username, string(hashedPwd))
	if err != nil {
		return fmt.Errorf("failed to create super admin: %w", err)
	}

	logger.Info("Default super admin created: %s (password must be changed on first login)", username)
	return nil
}

func ensureDefaultTenant(store *controllerstorage.Storage, logger *logging.Logger) error {
	var count int
	err := store.DB().QueryRow("SELECT COUNT(*) FROM tenants").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check tenants: %w", err)
	}

	if count > 0 {
		logger.Info("Tenants already exist (%d)", count)
		return nil
	}

	tenantID, err := store.GetOrCreateTenantByCode("default", "Aria Default")
	if err != nil {
		return fmt.Errorf("failed to create default tenant: %w", err)
	}
	if err := store.EnsureTenantRoles(tenantID); err != nil {
		return fmt.Errorf("failed to create default tenant roles: %w", err)
	}

	logger.Info("Default tenant created: Aria Default (code=default)")
	return nil
}

func previewString(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func nodeRequiresFreshEnrollment(node *controllerstorage.Node) bool {
	return node != nil && strings.EqualFold(strings.TrimSpace(node.Status), "deleted")
}

func nodeRegistrationForbidden(node *controllerstorage.Node) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Status)) {
	case "suspended", "banned":
		return true
	default:
		return false
	}
}

func nodeCertificateRequestForbidden(node *controllerstorage.Node) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Status)) {
	case "deleted", "suspended", "banned":
		return true
	default:
		return false
	}
}

func defaultRegistrationACLAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return "allow"
	}
	return action
}
