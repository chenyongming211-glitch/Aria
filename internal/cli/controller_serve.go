package cli

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
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
	"aria/internal/nodeidentity"
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
	RuntimeToken     string   `json:"runtime_token,omitempty"`
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
	Peers               []NodeInfo        `json:"peers"`
	AssignedIP          string            `json:"assigned_ip"`
	LastUpdate          int64             `json:"last_update"`
	ACLRules            []ACLRuleJSON     `json:"acl_rules,omitempty"`            // Firewall ACL rules
	MetricsPushGateway  string            `json:"metrics_push_gateway,omitempty"` // VictoriaMetrics push gateway URL
	SnapshotComplete    bool              `json:"snapshot_complete"`
	DomainVersions      map[string]string `json:"domain_versions,omitempty"`
	CertificatePEM      string            `json:"certificate_pem,omitempty"`
	CertificateCA       string            `json:"certificate_ca,omitempty"`
	CertificateNotAfter int64             `json:"certificate_not_after,omitempty"`
}

type registrationAuthResult struct {
	RequestedTenantID uuid.UUID
}

type registrationAuthError struct {
	status  int
	message string
	err     error
}

func (e *registrationAuthError) Error() string {
	if e == nil {
		return ""
	}
	if e.err != nil {
		return e.err.Error()
	}
	return e.message
}

func newRegistrationAuthError(status int, message string, err error) *registrationAuthError {
	return &registrationAuthError{status: status, message: message, err: err}
}

// ACLRuleJSON represents an ACL rule in API responses.
type ACLRuleJSON struct {
	ID        string `json:"id,omitempty"` // Controller policy rule ID
	SrcNet    string `json:"src_net"`      // Source CIDR
	DstNet    string `json:"dst_net"`      // Destination CIDR
	Protocol  uint8  `json:"protocol"`     // IP protocol (6=TCP, 17=UDP, 0=any)
	MinPort   uint16 `json:"min_port"`     // Min port (0=any)
	MaxPort   uint16 `json:"max_port"`     // Max port (65535=any)
	Action    string `json:"action"`       // allow or deny
	Direction string `json:"direction"`    // ingress, egress, or both
	Ports     string `json:"ports"`        // port bitmap rule string
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

	publicIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(publicIP); err == nil {
		publicIP = host
	}

	req.RuntimeToken = registrationRuntimeTokenFromRequest(r, &req)
	assignedIP, err := c.processRegistration(&req, publicIP)
	if err != nil {
		c.writeRegistrationError(w, &req, err)
		return
	}

	c.syncNode(&req, assignedIP, w)
}

func (c *Controller) writeRegistrationError(w http.ResponseWriter, req *RegisterRequest, err error) {
	var authErr *registrationAuthError
	if errors.As(err, &authErr) {
		c.logger.Warn("Registration rejected for node %s: %v", previewString(req.PublicKey, 8), authErr)
		http.Error(w, authErr.message, authErr.status)
		return
	}

	var conflict *controllerstorage.RouteConflictError
	if errors.As(err, &conflict) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if strings.Contains(err.Error(), "invalid CIDR") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.logger.Error("Registration failed for node %s: %v", previewString(req.PublicKey, 8), err)
	http.Error(w, "Failed to register node", http.StatusInternalServerError)
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

	// 只获取同租户且可参与同步的节点。删除、暂停、封禁节点不能继续作为 peer 下发。
	query := `SELECT id, public_key, machine_id, tenant_id, endpoint, private_ip, public_ip, region, vpc_id, hostname, assigned_ip, ip_offset, last_seen, registered_at, role, COALESCE(runtime_mode, 'kernel'), COALESCE(kernel_version, ''), COALESCE(has_aesni, false), COALESCE(status, 'online'), COALESCE(offline_since, 0), advertised_routes, COALESCE(enrolled_with_token, ''), created_at, updated_at FROM nodes WHERE tenant_id = $1 AND COALESCE(status, 'online') NOT IN ('deleted', 'suspended', 'banned')`
	rows, err := c.store.DB().Query(query, nodeTenantID)
	if err != nil {
		c.logger.Error("Failed to get peers for tenant %s: %v", nodeTenantID, err)
		http.Error(w, "Failed to load tenant peers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var peerInfos []NodeInfo
	var requestingNode *controllerstorage.Node
	for rows.Next() {
		peer, err := c.store.ScanNodeRows(rows)
		if err != nil {
			c.logger.Error("Failed to scan peer: %v", err)
			continue
		}
		if peer.PublicKey == req.PublicKey {
			requestingNode = peer
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

	if requestingNode == nil {
		c.logger.Error("Registered agent %s was not found in active peer set for tenant %s", previewString(req.PublicKey, 16), nodeTenantID)
		http.Error(w, "Registered node not found in active peer set", http.StatusInternalServerError)
		return
	}

	// Get enabled ACL rules owned by the requesting node. ACL is a node-scoped
	// runtime policy; region/advertised-route filtering belongs to the legacy
	// topology model and must not hide node ACLs from the owning agent.
	var aclRulesJSON []ACLRuleJSON
	aclRules, err := c.store.GetEnabledTenantNodeACLRules(nodeTenantID, requestingNode.ID)
	if err != nil {
		c.logger.Error("Failed to get node ACL rules for tenant %s node %s: %v", nodeTenantID, requestingNode.ID, err)
		http.Error(w, "Failed to load ACL rules", http.StatusInternalServerError)
		return
	}
	aclRulesJSON = aclRuleRecordsForSync(aclRules)
	c.logger.Info("Agent %s: sending %d node ACL rules", previewString(req.PublicKey, 16), len(aclRulesJSON))

	desiredVersion, err := c.ensureRESTDesiredStateVersion(requestingNode)
	if err != nil {
		c.logger.Error("Failed to determine desired state version for node %s: %v", previewString(req.PublicKey, 8), err)
		http.Error(w, "Failed to determine desired state version", http.StatusInternalServerError)
		return
	}

	response := SyncResponse{
		Peers:              peerInfos,
		AssignedIP:         assignedIP,
		LastUpdate:         time.Now().Unix(),
		ACLRules:           aclRulesJSON,
		MetricsPushGateway: c.metricsPushGateway,
		SnapshotComplete:   true,
		DomainVersions:     registrationDomainVersionsFromDesiredVersion(desiredVersion),
	}
	c.attachNodeCertificateToSyncResponse(req.PublicKey, &response)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (c *Controller) ensureRESTDesiredStateVersion(node *controllerstorage.Node) (string, error) {
	if c == nil || c.store == nil || node == nil || node.ID == uuid.Nil || node.TenantID == uuid.Nil {
		return "", nil
	}

	state, err := c.store.GetNodeControlState(node.TenantID, node.ID)
	if err != nil {
		return "", err
	}
	if state != nil && strings.TrimSpace(state.DesiredStateVersion) != "" {
		return state.DesiredStateVersion, nil
	}

	created, err := c.store.UpsertNodeDesiredState(node.TenantID, node.ID, controllerstorage.NewDesiredStateVersion(), map[string]interface{}{
		"source": "sync-baseline",
	})
	if err != nil {
		return "", err
	}
	return created.DesiredStateVersion, nil
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

func registrationRuntimeTokenFromRequest(r *http.Request, req *RegisterRequest) string {
	if req != nil && strings.TrimSpace(req.RuntimeToken) != "" {
		return strings.TrimSpace(req.RuntimeToken)
	}
	if r == nil {
		return ""
	}
	token, ok := runtimeBearerTokenFromRequest(r)
	if !ok {
		return ""
	}
	return token
}

func (c *Controller) authorizeRegistrationRequest(req *RegisterRequest, existingNode *controllerstorage.Node, runtimeToken string) (registrationAuthResult, *registrationAuthError) {
	if req == nil {
		return registrationAuthResult{}, newRegistrationAuthError(http.StatusBadRequest, "Invalid registration request", fmt.Errorf("registration request is required"))
	}
	if nodeRegistrationForbidden(existingNode) {
		return registrationAuthResult{}, newRegistrationAuthError(http.StatusForbidden, "Node registration is disabled", fmt.Errorf("node registration is disabled"))
	}

	isReRegistration := existingNode != nil
	requiresFreshEnrollment := nodeRequiresFreshEnrollment(existingNode)
	if isReRegistration && !requiresFreshEnrollment {
		if strings.TrimSpace(runtimeToken) != "" {
			if err := validateRegistrationRuntimeToken(runtimeToken, existingNode); err != nil {
				return registrationAuthResult{}, err
			}
			return registrationAuthResult{RequestedTenantID: existingNode.TenantID}, nil
		}

		if strings.TrimSpace(req.Token) == "" {
			return registrationAuthResult{}, newRegistrationAuthError(http.StatusUnauthorized, "Runtime token required", fmt.Errorf("runtime token required for node re-registration"))
		}
		tenantID, err := c.validateEnrollmentTokenTenant(req.Token)
		if err != nil {
			return registrationAuthResult{}, err
		}
		if tenantID != existingNode.TenantID {
			return registrationAuthResult{}, newRegistrationAuthError(http.StatusForbidden, "Node tenant ownership is immutable", fmt.Errorf("node tenant ownership is immutable"))
		}
		if !registrationMachineIDMatches(req.MachineID, existingNode.MachineID) {
			return registrationAuthResult{}, newRegistrationAuthError(http.StatusForbidden, "Node machine identity mismatch", fmt.Errorf("node machine identity mismatch"))
		}
		return registrationAuthResult{RequestedTenantID: tenantID}, nil
	}

	if strings.TrimSpace(req.Token) == "" {
		return registrationAuthResult{}, newRegistrationAuthError(http.StatusUnauthorized, "Token required", fmt.Errorf("token required"))
	}

	tenantID, err := c.validateEnrollmentTokenTenant(req.Token)
	if err != nil {
		return registrationAuthResult{}, err
	}
	if requiresFreshEnrollment && tenantID != existingNode.TenantID {
		return registrationAuthResult{}, newRegistrationAuthError(http.StatusForbidden, "Node tenant ownership is immutable", fmt.Errorf("node tenant ownership is immutable"))
	}

	return registrationAuthResult{RequestedTenantID: tenantID}, nil
}

func validateRegistrationRuntimeToken(runtimeToken string, existingNode *controllerstorage.Node) *registrationAuthError {
	if existingNode == nil {
		return newRegistrationAuthError(http.StatusUnauthorized, "Node not found", fmt.Errorf("node not found"))
	}

	claims, err := auth.ValidateRuntimeToken(runtimeToken)
	if err != nil {
		return newRegistrationAuthError(http.StatusUnauthorized, "Invalid runtime token", fmt.Errorf("invalid runtime token"))
	}
	tokenNodeID, err := uuid.Parse(claims.NodeID)
	if err != nil {
		return newRegistrationAuthError(http.StatusUnauthorized, "Invalid runtime token node id", fmt.Errorf("invalid runtime token node id"))
	}
	if tokenNodeID != existingNode.ID {
		return newRegistrationAuthError(http.StatusForbidden, "Runtime token node mismatch", fmt.Errorf("runtime token node mismatch"))
	}
	if strings.TrimSpace(claims.TenantID) == "" {
		return newRegistrationAuthError(http.StatusUnauthorized, "Runtime token tenant missing", fmt.Errorf("runtime token tenant missing"))
	}
	tokenTenantID, err := uuid.Parse(claims.TenantID)
	if err != nil {
		return newRegistrationAuthError(http.StatusUnauthorized, "Invalid runtime token tenant", fmt.Errorf("invalid runtime token tenant"))
	}
	if tokenTenantID != existingNode.TenantID {
		return newRegistrationAuthError(http.StatusForbidden, "Runtime token tenant mismatch", fmt.Errorf("runtime token tenant mismatch"))
	}
	return nil
}

func registrationMachineIDMatches(requestMachineID, storedMachineID string) bool {
	requestMachineID = strings.TrimSpace(requestMachineID)
	storedMachineID = strings.TrimSpace(storedMachineID)
	return requestMachineID != "" && storedMachineID != "" && requestMachineID == storedMachineID
}

func (c *Controller) validateEnrollmentTokenTenant(tokenValue string) (uuid.UUID, *registrationAuthError) {
	if c.tokenValidator == nil {
		return uuid.Nil, newRegistrationAuthError(http.StatusInternalServerError, "Token validator is not configured", fmt.Errorf("token validator is not configured"))
	}

	tkn, err := c.tokenValidator.Validate(tokenValue)
	if err != nil {
		switch err {
		case token.ErrTokenNotFound:
			return uuid.Nil, newRegistrationAuthError(http.StatusUnauthorized, "Invalid token", err)
		case token.ErrTokenExpired:
			return uuid.Nil, newRegistrationAuthError(http.StatusUnauthorized, "Token expired", err)
		case token.ErrTokenExhausted:
			return uuid.Nil, newRegistrationAuthError(http.StatusUnauthorized, "Token exhausted (max uses reached)", err)
		case token.ErrTokenRevoked:
			return uuid.Nil, newRegistrationAuthError(http.StatusUnauthorized, "Token revoked", err)
		default:
			return uuid.Nil, newRegistrationAuthError(http.StatusUnauthorized, "Invalid token", err)
		}
	}
	if c.logger != nil {
		c.logger.Debug("Token validated: %s (tag: %s)", previewString(tkn.Token, 12), tkn.Tag)
	}

	tenantID, err := c.store.GetTenantIDByToken(tokenValue)
	if err != nil {
		return uuid.Nil, newRegistrationAuthError(http.StatusInternalServerError, "Failed to get tenant ID from token", err)
	}
	return tenantID, nil
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
	if strings.TrimSpace(claims.TenantID) == "" {
		http.Error(w, "Runtime token tenant missing", http.StatusUnauthorized)
		return nil, false
	}
	tokenTenantID, err := uuid.Parse(claims.TenantID)
	if err != nil {
		http.Error(w, "Invalid runtime token tenant", http.StatusUnauthorized)
		return nil, false
	}
	if tokenTenantID != node.TenantID {
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
		if strings.TrimSpace(claims.TenantID) == "" {
			return nil, fmt.Errorf("invalid runtime token tenant")
		}
		tokenTenantID, err := uuid.Parse(claims.TenantID)
		if err != nil {
			return nil, fmt.Errorf("invalid runtime token tenant")
		}
		if tokenTenantID != node.TenantID {
			return nil, fmt.Errorf("tenant mismatch")
		}
		if nodeCertificateRequestForbidden(node) {
			return nil, fmt.Errorf("node access denied: current status is '%s'", strings.ToLower(strings.TrimSpace(node.Status)))
		}
		return node, nil
	}

	if req.Token != "" {
		return nil, fmt.Errorf("runtime_token is required for certificate issuance")
	}

	return nil, fmt.Errorf("runtime_token is required")
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
	if renewedFrom == nil {
		nodeID := node.ID
		if _, err := c.store.CreateAuditEvent(&controllerstorage.AuditEvent{
			TenantID:  node.TenantID,
			NodeID:    &nodeID,
			EventType: controllerstorage.AuditCertIssued,
			Actor:     "system",
			Summary:   fmt.Sprintf("节点 %s 证书已签发", node.Hostname),
			Detail: map[string]interface{}{
				"serial_number": certMeta.SerialNumber,
				"not_after":     certMeta.NotAfter.UTC().Format(time.RFC3339),
			},
		}); err != nil && c.logger != nil {
			c.logger.Warn("Failed to create cert.issued audit event for node %s: %v", node.Hostname, err)
		}
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
	if !cert.NotAfter.After(time.Now()) {
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

	claims, ok := c.authorizeJWTPermission(w, r, middleware.PermRoutesWrite)
	if !ok {
		return
	}

	var req struct {
		TenantID string `json:"tenant_id"`
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

	// Find node by hostname within the authenticated tenant. super_admin is
	// platform-scoped; tenant users are restricted to their JWT tenant.
	var allNodes []*controllerstorage.Node
	var err error
	var tenantID uuid.UUID
	if claims.Role == middleware.RoleSuperAdmin {
		tenantID, err = uuid.Parse(strings.TrimSpace(req.TenantID))
		if err != nil {
			http.Error(w, "tenant_id is required for super_admin network changes", http.StatusBadRequest)
			return
		}
		allNodes, err = c.store.GetNodesByTenant(tenantID)
	} else {
		var parseErr error
		tenantID, parseErr = uuid.Parse(claims.TenantID)
		if parseErr != nil {
			http.Error(w, "Invalid tenant context", http.StatusForbidden)
			return
		}
		allNodes, err = c.store.GetNodesByTenant(tenantID)
	}
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

		if err := controllerstorage.FindAdvertisedRouteConflict(allNodes, targetNode.PublicKey, targetNode.Region, []string{newNetwork.String()}); err != nil {
			var conflict *controllerstorage.RouteConflictError
			if errors.As(err, &conflict) {
				http.Error(w, fmt.Sprintf("请勿在不同 Region 添加重叠路由！路由 %s 与 Region %s 的节点 %s 冲突",
					req.CIDR, conflict.NodeRegion, conflict.NodeHostname), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
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
		c.heartbeat.PublishConfigChange(targetNode.TenantID.String(), "network_update", map[string]string{
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

func (c *Controller) authorizeJWTPermission(w http.ResponseWriter, r *http.Request, permission string) (*auth.Claims, bool) {
	tokenString, ok := runtimeBearerTokenFromRequest(r)
	if !ok {
		http.Error(w, "Authorization header required", http.StatusUnauthorized)
		return nil, false
	}

	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return nil, false
	}
	if claims.MustChangePassword {
		http.Error(w, "You must change your password before proceeding", http.StatusForbidden)
		return nil, false
	}
	if claims.Role == middleware.RoleSuperAdmin {
		return claims, true
	}
	if claims.TenantID == "" {
		http.Error(w, "Tenant context required", http.StatusForbidden)
		return nil, false
	}
	tenantID, err := uuid.Parse(claims.TenantID)
	if err != nil {
		http.Error(w, "Invalid tenant context", http.StatusForbidden)
		return nil, false
	}

	roleName := controllerstorage.NormalizeRoleName(claims.Role)
	permissions, err := c.store.GetRolePermissions(tenantID, roleName)
	if err != nil {
		http.Error(w, "Role not found", http.StatusForbidden)
		return nil, false
	}
	for _, p := range permissions {
		if p == permission {
			return claims, true
		}
	}

	http.Error(w, "Insufficient permissions", http.StatusForbidden)
	return nil, false
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

// ========== gRPC Adapters ==========
// 以下两个方法将 REST API handler 适配为 gRPC 所需的格式

// createRegisterAdapter 创建注册适配器
// 将 Controller 的注册核心逻辑包装成 gRPC 可调用的 typed handler。
func (c *Controller) createRegisterAdapter() grpcserver.RegisterHandler {
	return func(regReq *grpcserver.RegistrationRequest) (*grpcserver.RegistrationResult, error) {
		if regReq == nil {
			return nil, fmt.Errorf("registration request is required")
		}

		req := &RegisterRequest{
			PublicKey:        regReq.PublicKey,
			Endpoint:         regReq.Endpoint,
			PrivateIP:        regReq.PrivateIP,
			PublicIP:         regReq.PublicIP,
			Region:           regReq.Region,
			Hostname:         regReq.Hostname,
			MachineID:        regReq.MachineID,
			RegisteredAt:     regReq.RegisteredAt,
			Token:            regReq.Token,
			RuntimeToken:     regReq.RuntimeToken,
			AdvertisedRoutes: append([]string(nil), regReq.AdvertisedRoutes...),
			RuntimeMode:      regReq.RuntimeMode,
			KernelVersion:    regReq.KernelVersion,
			HasAESNI:         regReq.HasAESNI,
		}

		assignedIP, err := c.processRegistration(req, "")
		if err != nil {
			return nil, err
		}

		node, err := c.store.GetNode(req.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load registered node: %w", err)
		}
		if node == nil {
			return nil, fmt.Errorf("registered node was not found")
		}

		runtimeToken, runtimeTokenExpiresAt, err := auth.GenerateRuntimeToken(node.ID.String(), node.TenantID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to issue runtime token: %w", err)
		}

		return &grpcserver.RegistrationResult{
			AssignedIP:            assignedIP,
			MetricsPushGateway:    c.metricsPushGateway,
			NodeID:                node.ID.String(),
			RuntimeToken:          runtimeToken,
			RuntimeTokenExpiresAt: runtimeTokenExpiresAt.Unix(),
		}, nil
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
	isReRegistration := existingNode != nil
	requiresFreshEnrollment := nodeRequiresFreshEnrollment(existingNode)
	authResult, authErr := c.authorizeRegistrationRequest(req, existingNode, strings.TrimSpace(req.RuntimeToken))
	if authErr != nil {
		return "", authErr
	}
	requestedTenantID := authResult.RequestedTenantID

	// Persist only the node's true public identity. VPC/local interface
	// addresses are not useful for SaaS inventory or cross-site reachability.
	normalizedPublicIP := nodeidentity.NormalizePublicIPv4(req.PublicIP)
	if normalizedPublicIP == "" {
		normalizedPublicIP = nodeidentity.NormalizePublicIPv4(publicIP)
	}
	normalizedEndpoint := nodeidentity.NormalizePublicEndpoint(req.Endpoint, normalizedPublicIP)
	if normalizedPublicIP == "" {
		normalizedPublicIP, _ = nodeidentity.NormalizeReportedNetwork("", normalizedEndpoint)
	}
	req.PublicIP = normalizedPublicIP
	req.Endpoint = normalizedEndpoint
	req.PrivateIP = ""

	var assignedIP string
	var ipOffset int

	existingNode, err := c.store.GetNode(req.PublicKey)
	if err == nil && existingNode != nil {
		isReRegistration = true
		requiresFreshEnrollment = nodeRequiresFreshEnrollment(existingNode)
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
		defaultTenantID, err := c.store.GetOrCreateTenant("default")
		if err != nil {
			return "", fmt.Errorf("failed to assign default tenant: %w", err)
		}
		tenantID = defaultTenantID
	}
	if err := c.validateAdvertisedRouteConflicts(tenantID, req.PublicKey, req.Region, req.AdvertisedRoutes); err != nil {
		return "", fmt.Errorf("failed to validate advertised routes: %w", err)
	}

	node := &controllerstorage.Node{
		PublicKey:         req.PublicKey,
		Endpoint:          req.Endpoint,
		PrivateIP:         "",
		PublicIP:          req.PublicIP,
		Region:            req.Region,
		VPCID:             req.VPCID,
		Hostname:          req.Hostname,
		MachineID:         req.MachineID,
		AssignedIP:        assignedIP,
		IPOffset:          ipOffset,
		AdvertisedRoutes:  req.AdvertisedRoutes,
		RuntimeMode:       req.RuntimeMode,
		KernelVersion:     req.KernelVersion,
		HasAESNI:          req.HasAESNI,
		EnrolledWithToken: req.Token,
		LastSeen:          time.Now().Unix(),
		RegisteredAt:      req.RegisteredAt,
		TenantID:          tenantID,
	}
	if existingNode != nil && strings.TrimSpace(existingNode.Role) != "" {
		node.Role = existingNode.Role
	} else {
		node.Role = "agent"
	}

	if existingNode != nil {
		node.ID = existingNode.ID
		node.RegisteredAt = existingNode.RegisteredAt
	}

	issueCertificateBeforeSave := isReRegistration && !requiresFreshEnrollment && c.certService != nil && strings.TrimSpace(req.CSRPEM) != ""
	if issueCertificateBeforeSave {
		if _, err := c.issueNodeCertificate(node, req.CSRPEM, nil); err != nil {
			return "", fmt.Errorf("failed to issue node certificate: %w", err)
		}
	}

	if err := c.store.SaveNode(node); err != nil {
		return "", fmt.Errorf("failed to save node: %w", err)
	}

	if strings.TrimSpace(req.Token) != "" {
		if err := c.tokenValidator.ConsumeToken(req.Token, req.PublicKey); err != nil {
			if c.logger != nil {
				c.logger.Warn("Failed to consume token: %v", err)
			}
			if !isReRegistration || requiresFreshEnrollment {
				if markErr := c.store.MarkNodeDeleted(req.PublicKey); markErr != nil && c.logger != nil {
					c.logger.Warn("Failed to roll back node %s after enrollment token consume failure: %v", previewString(req.PublicKey, 8), markErr)
				}
			}
			return "", fmt.Errorf("failed to consume enrollment token: %w", err)
		}
	}

	if c.certService != nil && strings.TrimSpace(req.CSRPEM) != "" && !issueCertificateBeforeSave {
		if _, err := c.issueNodeCertificate(node, req.CSRPEM, nil); err != nil {
			if !isReRegistration || requiresFreshEnrollment {
				if markErr := c.store.MarkNodeDeleted(req.PublicKey); markErr != nil && c.logger != nil {
					c.logger.Warn("Failed to roll back node %s after certificate issuance failure: %v", previewString(req.PublicKey, 8), markErr)
				}
			}
			return "", fmt.Errorf("failed to issue node certificate: %w", err)
		}
	}

	c.recordNodeRegistrationAudit(node, isReRegistration)

	c.logger.Info("Node registered successfully: %s (hostname=%s, IP=%s, region=%s)",
		previewString(req.PublicKey, 8), req.Hostname, assignedIP, req.Region)

	return assignedIP, nil
}

func (c *Controller) recordNodeRegistrationAudit(node *controllerstorage.Node, isReRegistration bool) {
	if c == nil || c.store == nil || node == nil || node.ID == uuid.Nil || node.TenantID == uuid.Nil {
		return
	}

	eventType := controllerstorage.AuditNodeRegistered
	summary := "Node registered"
	if isReRegistration {
		eventType = controllerstorage.AuditNodeReregistered
		summary = "Node re-registered"
	}

	nodeID := node.ID
	if _, err := c.store.CreateAuditEvent(&controllerstorage.AuditEvent{
		TenantID:  node.TenantID,
		NodeID:    &nodeID,
		EventType: eventType,
		Actor:     "agent",
		Summary:   summary,
		Detail: map[string]interface{}{
			"hostname":          node.Hostname,
			"assigned_ip":       node.AssignedIP,
			"public_key_prefix": previewString(node.PublicKey, 8),
			"region":            node.Region,
			"runtime_mode":      node.RuntimeMode,
		},
	}); err != nil && c.logger != nil {
		c.logger.Warn("Failed to create %s audit event for node %s: %v", eventType, previewString(node.PublicKey, 8), err)
	}
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
		if !nodeEligibleForSync(n) {
			continue
		}
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

	aclRuleRecords, err := c.store.GetEnabledTenantNodeACLRules(node.TenantID, node.ID)
	if err != nil {
		return nil, "", nil, uuid.Nil, fmt.Errorf("failed to get node ACL rules: %w", err)
	}

	aclRules := aclRuleRecordsForSync(aclRuleRecords)

	return peers, node.AssignedIP, aclRules, node.TenantID, nil
}

func aclRuleRecordsForSync(rules []*controllerstorage.ACLRuleRecord) []ACLRuleJSON {
	result := make([]ACLRuleJSON, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		syncRule := aclRuleRecordForSync(rule)
		if syncRule.Protocol == 0 && aclSyncRuleHasPortFilter(syncRule) {
			tcpRule := syncRule
			tcpRule.Protocol = 6
			udpRule := syncRule
			udpRule.Protocol = 17
			result = append(result, tcpRule, udpRule)
			continue
		}
		result = append(result, syncRule)
	}
	return result
}

func aclRuleRecordForSync(rule *controllerstorage.ACLRuleRecord) ACLRuleJSON {
	minPort, maxPort := aclPortsForSync(rule)
	return ACLRuleJSON{
		ID:        rule.ID.String(),
		SrcNet:    rule.SrcCIDR,
		DstNet:    rule.DstCIDR,
		Protocol:  uint8(rule.Protocol),
		MinPort:   minPort,
		MaxPort:   maxPort,
		Action:    defaultRegistrationACLAction(rule.Action),
		Direction: rule.Direction,
		Ports:     rule.Ports,
	}
}

func aclPortsForSync(rule *controllerstorage.ACLRuleRecord) (uint16, uint16) {
	if rule == nil {
		return 0, 0
	}
	if rule.DstPort > 0 && rule.DstPort <= 65535 {
		port := uint16(rule.DstPort)
		return port, port
	}
	if strings.TrimSpace(rule.Ports) == "" {
		return 0, 0
	}
	return parsePortRange(rule.Ports)
}

func aclSyncRuleHasPortFilter(rule ACLRuleJSON) bool {
	ports := strings.TrimSpace(rule.Ports)
	if ports != "" && !strings.EqualFold(ports, "all") {
		return true
	}
	return rule.MinPort > 0 || rule.MaxPort > 0
}

func nodeEligibleForSync(node *controllerstorage.Node) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Status)) {
	case "deleted", "suspended", "banned":
		return false
	default:
		return true
	}
}

func registrationDomainVersionsFromDesiredVersion(version string) map[string]string {
	version = strings.TrimSpace(version)
	if version == "" {
		return map[string]string{}
	}

	return map[string]string{
		"peer":        version,
		"acl":         version,
		"qos":         version,
		"route":       version,
		"blacklist":   version,
		"certificate": version,
	}
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

func (c *Controller) validateAdvertisedRouteConflicts(tenantID uuid.UUID, publicKey, region string, routes []string) error {
	if len(routes) == 0 {
		return nil
	}
	nodes, err := c.store.GetNodesByTenant(tenantID)
	if err != nil {
		return err
	}
	return controllerstorage.FindAdvertisedRouteConflict(nodes, publicKey, region, routes)
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
	username := strings.TrimSpace(os.Getenv("ARIA_SUPER_ADMIN"))
	rawPassword, passwordOverride := os.LookupEnv("ARIA_SUPER_ADMIN_PASSWORD")
	password := strings.TrimSpace(rawPassword)
	passwordConfigured := passwordOverride && password != ""
	syncConfigured := envBool("ARIA_SUPER_ADMIN_SYNC")

	if username == "" {
		username = "sysadmin"
	}

	var existingUserID, existingPasswordHash string
	err := db.QueryRow(`SELECT id, password_hash FROM users WHERE username = $1 AND role = 'super_admin'`, username).Scan(&existingUserID, &existingPasswordHash)
	if err == nil {
		if !passwordConfigured {
			logger.Info("Super admin already exists")
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(existingPasswordHash), []byte(password)) == nil {
			logger.Info("Super admin password already synchronized: %s", username)
			return nil
		}
		if !syncConfigured {
			logger.Warn("Configured super admin password differs for %s; keeping existing database password. Set ARIA_SUPER_ADMIN_SYNC=true to force synchronization.", username)
			return nil
		}

		hashedPwd, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		result, err := db.Exec(`UPDATE users SET password_hash = $1, must_change_password = TRUE WHERE id = $2`,
			string(hashedPwd), existingUserID)
		if err != nil {
			return fmt.Errorf("failed to update super admin password: %w", err)
		}
		if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
			return fmt.Errorf("failed to update super admin password: user %s was not updated", username)
		}

		logger.Info("Super admin password synchronized from ARIA_SUPER_ADMIN_PASSWORD: %s", username)
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check super admin user: %w", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'super_admin'").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check super admin: %w", err)
	}

	if count > 0 && !passwordConfigured {
		logger.Info("Super admin already exists")
		return nil
	}
	if !passwordConfigured {
		return fmt.Errorf("ARIA_SUPER_ADMIN_PASSWORD is required when creating the initial super admin user")
	}

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if count == 1 {
		if !syncConfigured {
			logger.Warn("Configured super admin username %s does not match existing super admin; keeping existing database username. Set ARIA_SUPER_ADMIN_SYNC=true to force migration.", username)
			return nil
		}

		var migrateUserID, migrateUsername string
		err := db.QueryRow(`SELECT id, username FROM users WHERE role = 'super_admin' ORDER BY created_at ASC LIMIT 1`).Scan(&migrateUserID, &migrateUsername)
		if err != nil {
			return fmt.Errorf("failed to load existing super admin for migration: %w", err)
		}
		result, err := db.Exec(`UPDATE users SET username = $1, password_hash = $2, must_change_password = TRUE WHERE id = $3`,
			username, string(hashedPwd), migrateUserID)
		if err != nil {
			return fmt.Errorf("failed to migrate super admin username: %w", err)
		}
		if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
			return fmt.Errorf("failed to migrate super admin username: user %s was not updated", migrateUsername)
		}
		logger.Info("Super admin username migrated from %s to %s", migrateUsername, username)
		return nil
	}

	_, err = db.Exec(`INSERT INTO users (username, password_hash, role, tenant_id, must_change_password) VALUES ($1, $2, 'super_admin', NULL, TRUE)`,
		username, string(hashedPwd))
	if err != nil {
		return fmt.Errorf("failed to create super admin: %w", err)
	}

	logger.Info("Configured super admin created: %s (password must be changed on first login)", username)
	return nil
}

func envBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
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
