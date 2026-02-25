package cli

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"aria/pkg/agentstorage"
	"aria/pkg/capability"
	"aria/pkg/config"
	"aria/pkg/datapath"
	"aria/pkg/metrics"
	"aria/pkg/monitor"
	"aria/pkg/nettuning"
	"aria/pkg/route"
	"aria/pkg/rpc"
	"aria/pkg/wgmanager"

	firewall_ebpf "aria/internal/agent/firewall"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Aria tunnel",
	Long: `Start the Aria tunnel using existing configuration.

This command:
  1. Loads existing configuration (run 'aria init' first if not initialized)
  2. Creates and configures the WireGuard interface
  3. Starts syncing peers from controller (runs in daemon mode by default)

Use 'systemctl enable --now aria' to start automatically on boot.

Examples:
  # Start tunnel (runs in background by default)
  aria up

  # Start in foreground (for debugging)
  aria up --daemon=false

  # Start with new controller URL
  aria up --server=https://new-controller:8443

  # Start in daemon mode
  aria up --daemon`,
	RunE: runUp,
}

var (
	upServer          string
	upToken           string
	upDaemon          bool
	upForce           bool
	upAdvertiseRoutes string
	upTune            bool
	upRegion          string
	upCustomerID      string
	upNoFirewall      bool // Disable firewall (emergency escape hatch)
	upMultiTunnel     bool // Enable multi-tunnel mode
	upTunnelCount     int  // Number of tunnels (0=default 4)
	upEBPFEnabled     bool // Enable eBPF-based firewall/QoS
	upEBPFAclOnly     bool // Enable only eBPF ACL (skip QoS)
	upEBPFQoSONly     bool // Enable only eBPF QoS (skip ACL)
)

func init() {
	rootCmd.AddCommand(upCmd)
	upCmd.Flags().StringVar(&upServer, "server", "", "Controller URL (required for first-time setup)")
	upCmd.Flags().StringVar(&upToken, "token", "", "Enrollment token (required for first-time setup)")
	upCmd.Flags().BoolVar(&upDaemon, "daemon", true, "Run as daemon (default: true)")
	upCmd.Flags().BoolVar(&upForce, "force", false, "Force re-registration even if config exists")
	upCmd.Flags().StringVar(&upAdvertiseRoutes, "advertise-routes", "", "Routes to advertise to other nodes (e.g., 192.168.1.0/24)")
	upCmd.Flags().BoolVar(&upTune, "tune", true, "Apply network performance optimizations (BBR, RPS, MSS clamping)")
	upCmd.Flags().StringVar(&upRegion, "region", "", "Region identifier (e.g., cn-shanghai, us-west)")
	upCmd.Flags().StringVar(&upCustomerID, "customer-id", "", "Customer identifier (e.g., customer-001)")
	upCmd.Flags().BoolVar(&upNoFirewall, "no-firewall", false, "Disable firewall/ACL (use only for troubleshooting)")
	upCmd.Flags().BoolVar(&upMultiTunnel, "multi-tunnel", false, "Enable multi-tunnel mode for higher throughput")
	upCmd.Flags().IntVar(&upTunnelCount, "tunnel-count", 0, "Number of tunnels (0=default 4, range: 1-8)")

	// eBPF flags
	upCmd.Flags().BoolVar(&upEBPFEnabled, "ebpf", false, "Enable eBPF-based firewall/QoS")
	upCmd.Flags().BoolVar(&upEBPFAclOnly, "ebpf-acl", false, "Enable only eBPF ACL (skip QoS)")
	upCmd.Flags().BoolVar(&upEBPFQoSONly, "ebpf-qos", false, "Enable only eBPF QoS (skip ACL)")
}

type upRegisterRequest struct {
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
	CustomerID       string   `json:"customer_id,omitempty"`
	// Capability detection fields
	RuntimeMode   string `json:"runtime_mode,omitempty"`
	KernelVersion string `json:"kernel_version,omitempty"`
	HasAESNI      bool   `json:"has_aesni,omitempty"`
}

type upSyncResponse struct {
	AssignedIP         string `json:"assigned_ip"`
	MetricsPushGateway string `json:"metrics_push_gateway,omitempty"`
}

type upNodeInfo struct {
	PublicKey        string   `json:"public_key"`
	Endpoint         string   `json:"endpoint"`
	PrivateIP        string   `json:"private_ip"`
	PublicIP         string   `json:"public_ip"`
	Region           string   `json:"region"`
	VPCID            string   `json:"vpc_id"`
	Hostname         string   `json:"hostname"`
	AssignedIP       string   `json:"assigned_ip"`
	Role             string   `json:"role"`
	AdvertisedRoutes []string `json:"advertised_routes,omitempty"` // Site-to-Site VPN
}

type upSyncResponseWithPeers struct {
	Peers              []upNodeInfo  `json:"peers"`
	AssignedIP         string        `json:"assigned_ip"`
	LastUpdate         int64         `json:"last_update"`
	ACLRules           []upACLRule   `json:"acl_rules,omitempty"`           // Firewall ACL rules
	MetricsPushGateway string        `json:"metrics_push_gateway,omitempty"` // VictoriaMetrics push gateway URL
}

// upACLRule represents an ACL rule from the Controller.
// Format: SrcNet -> DstNet:Protocol:Port whitelist.
type upACLRule struct {
	SrcNet   string `json:"src_net"`   // Source CIDR (e.g., "10.0.0.0/8")
	DstNet   string `json:"dst_net"`   // Destination CIDR (e.g., "192.168.0.0/16")
	Protocol uint8  `json:"protocol"`  // IP protocol (6=TCP, 17=UDP, 0=any)
	MinPort  uint16 `json:"min_port"`  // Min port (0=any)
	MaxPort  uint16 `json:"max_port"`  // Max port (65535=any)
}

func runUp(cmd *cobra.Command, args []string) error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	// 获取进程锁，防止多个实例同时运行
	processLock, err := wgmanager.AcquireProcessLock()
	if err != nil {
		return err
	}
	defer func() {
		if err := processLock.Release(); err != nil {
			fmt.Printf("Warning: failed to release process lock: %v\n", err)
		}
	}()

	configPath := "/etc/aria/agent.yaml"
	if cfgFile != "" {
		configPath = cfgFile
	}

	configMgr := config.NewManager(configPath)

	// Check config exists
	if !configMgr.Exists() {
		return fmt.Errorf("configuration not found. Please run 'aria init' first to initialize and register with the controller")
	}

	// Load config
	agentConfig, err := configMgr.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Update controller URL if provided
	if upServer != "" && agentConfig.ControllerURL != upServer {
		fmt.Printf("Updating controller URL: %s\n", upServer)
		agentConfig.ControllerURL = upServer
		if err := configMgr.Save(agentConfig); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}

	deviceID := agentConfig.DeviceID
	if len(deviceID) > 16 {
		deviceID = deviceID[:16]
	}
	fmt.Printf("Using existing configuration (Device: %s...)\n", deviceID)


	// Handle --advertise-routes parameter to update advertised routes at runtime
	if upAdvertiseRoutes != "" {
		var newRoutes []string
		for _, route := range splitAndTrim(upAdvertiseRoutes, ",") {
			if route != "" {
				newRoutes = append(newRoutes, route)
			}
		}
		if len(newRoutes) > 0 {
			fmt.Printf("Updating advertised routes: %s\n", strings.Join(newRoutes, ", "))
			agentConfig.AdvertisedRoutes = newRoutes
			// Save updated config
			if err := configMgr.Save(agentConfig); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
		}
	}

	// Sync routes with controller if needed
	if len(agentConfig.AdvertisedRoutes) > 0 {
		fmt.Print("Syncing advertised routes with controller... ")

		privateIP := getLocalIPAddr()
		publicIP := getPublicIPAddr()
		hostname, _ := os.Hostname()

		sysInfo := &capability.SystemInfo{
			RuntimeMode:   capability.RuntimeMode(agentConfig.RuntimeMode),
			KernelVersion: "unknown",
			HasAESNI:      false,
		}

		_, _, err = registerWithCtrl(
			agentConfig.ControllerURL,
			"",  // Token not needed for re-registration
			agentConfig.PublicKey,
			privateIP,
			publicIP,
			hostname,
			agentConfig.AdvertisedRoutes,
			agentConfig.Region,
			agentConfig.CustomerID,
			sysInfo,
			agentConfig.CACert,
		)
		if err != nil {
			fmt.Printf("warning: %v\n", err)
		} else {
			fmt.Println("done")
		}
	}

	// Ensure WireGuard interface exists (幂等性操作)
	fmt.Print("Configuring WireGuard... ")
	runtimeMode := datapath.RuntimeMode(agentConfig.RuntimeMode)
	if runtimeMode == "" {
		runtimeMode = datapath.ModeKernel
	}

	// 使用 DataPath 管理数据面
	var dpOpts []datapath.Option

	// Determine WireGuard port range based on multi-tunnel configuration
	wgPortMin := 51820
	wgPortMax := 51820
	if upMultiTunnel || agentConfig.MultiTunnel.Enabled {
		// Calculate port range for multi-tunnel
		tunnelCount := datapath.DetermineTunnelCount(agentConfig.MultiTunnel.TunnelCount)
		wgPortMin = agentConfig.MultiTunnel.BasePort
		wgPortMax = agentConfig.MultiTunnel.BasePort + tunnelCount - 1
	}

	// Enable firewall based on selected mode
	useEBPF := upEBPFEnabled || upEBPFAclOnly || upEBPFQoSONly
	useTraditionalFirewall := !useEBPF && !upNoFirewall

	if useEBPF {
		// Initialize eBPF firewall/QoS
		fmt.Println("Initializing eBPF firewall/QoS...")

		ebpfAdapter, err := firewall_ebpf.NewEBPFAdapter()
		if err != nil {
			return fmt.Errorf("failed to initialize eBPF adapter: %w", err)
		}
		defer func() {
			if ebpfAdapter != nil {
				ebpfAdapter.Close()
			}
		}()

		// Configure based on selected components
		if upEBPFEnabled || upEBPFAclOnly {
			fmt.Println("  eBPF ACL: enabled")
		}
		if upEBPFEnabled || upEBPFQoSONly {
			fmt.Println("  eBPF QoS: enabled")
		}

		fmt.Printf("  eBPF enabled with options: all=%v, acl_only=%v, qos_only=%v\n", upEBPFEnabled, upEBPFAclOnly, upEBPFQoSONly)

		// Integrate with datapath - for now, we'll log the intent
		// The actual integration would happen through the datapath package
	} else if runtime.GOOS == "linux" && useTraditionalFirewall {
		dpOpts = append(dpOpts, datapath.WithNftablesFirewall(
			datapath.WithWireGuardPortRange(wgPortMin, wgPortMax),
		))
		fmt.Println("Firewall: enabled (use --no-firewall to disable)")
	} else if upNoFirewall {
		fmt.Println("Firewall: disabled (--no-firewall flag set)")
	} else if useEBPF {
		fmt.Println("eBPF: enabled")
	}

	dp, err := datapath.NewDataPath(agentConfig.Interface, runtimeMode, dpOpts...)
	if err != nil {
		return fmt.Errorf("failed to create datapath: %w", err)
	}

	// Initialize datapath (firewall, routing tables)
	if err := dp.Init(); err != nil {
		// On Linux, firewall init failure may indicate missing nftables
		if runtime.GOOS == "linux" && !upNoFirewall {
			fmt.Printf("Warning: firewall initialization failed: %v\n", err)
			fmt.Println("Hint: Install nftables with: apt install nftables (or yum install nftables)")
			fmt.Println("Or disable firewall with --no-firewall flag")
		} else {
			fmt.Printf("Warning: datapath initialization failed: %v\n", err)
		}
	}

	// 配置 WireGuard 接口
	ifaceCfg := &datapath.InterfaceConfig{
		Name:        agentConfig.Interface,
		PrivateKey:  agentConfig.PrivateKey,
		ListenPort:  51820,
		MTU:         1360,
		Address:     agentConfig.AssignedIP,
		RuntimeMode: runtimeMode,
	}

	// Check if multi-tunnel mode is enabled
	var multiTunnel *datapath.MultiTunnelManager
	if upMultiTunnel || agentConfig.MultiTunnel.Enabled {
		// Update config if flag is set
		if upMultiTunnel {
			agentConfig.MultiTunnel.Enabled = true
			if upTunnelCount > 0 {
				agentConfig.MultiTunnel.TunnelCount = upTunnelCount
			}
		}

		// Determine tunnel count (default is 4)
		tunnelCount := datapath.DetermineTunnelCount(agentConfig.MultiTunnel.TunnelCount)
		agentConfig.MultiTunnel.TunnelCount = tunnelCount

		fmt.Printf("Enabling multi-tunnel mode (%d tunnels)... ", tunnelCount)

		// Create multi-tunnel manager
		multiTunnel = datapath.NewMultiTunnelManager(
			"aria",
			agentConfig.MultiTunnel.BasePort,
			tunnelCount,
			runtimeMode,
			dp.Route,
		)

		// Create all tunnels
		if err := multiTunnel.EnsureAllTunnels(ifaceCfg); err != nil {
			return fmt.Errorf("failed to create multi-tunnel: %w", err)
		}

		// Update interface name to primary tunnel
		primaryTunnel := multiTunnel.GetPrimaryTunnel()
		if primaryTunnel != nil {
			agentConfig.Interface = primaryTunnel.Name
		}

		fmt.Println("done")
	} else {
		// Single tunnel mode (original behavior)
		if err := dp.Tunnel.EnsureInterface(ifaceCfg); err != nil {
			return fmt.Errorf("failed to ensure WireGuard interface: %w", err)
		}

		// 设置 IP 地址
		if err := dp.Tunnel.SetIPAddress(agentConfig.AssignedIP); err != nil {
			return fmt.Errorf("failed to set IP address: %w", err)
		}

		// Update config with actual interface name (important for macOS utun auto-numbering)
		actualInterface := dp.Tunnel.GetInterfaceName()
		if actualInterface != agentConfig.Interface {
			agentConfig.Interface = actualInterface
			if err := configMgr.Save(agentConfig); err != nil {
				return fmt.Errorf("failed to update config with actual interface: %w", err)
			}
		}
		fmt.Println("done")
	}

	// Initialize local storage for peer caching
	storage, err := agentstorage.NewStorage(agentConfig.StorageDir)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer storage.Close()

	// Apply network performance tuning
	if upTune {
		fmt.Print("Applying network optimizations... ")
		physIface := nettuning.AutoDetectPhysicalInterface()
		tuner := nettuning.NewTuner(physIface, agentConfig.Interface)
		result, err := tuner.Tune()
		if err != nil {
			fmt.Printf("warning: %v\n", err)
		} else {
			var optimizations []string
			if result.BBREnabled {
				optimizations = append(optimizations, "BBR")
			}
			if result.RPSEnabled {
				optimizations = append(optimizations, "RPS")
			}
			if result.MSSClamped {
				optimizations = append(optimizations, "MSS")
			}
			if len(optimizations) > 0 {
				fmt.Printf("done (%s)\n", strings.Join(optimizations, ", "))
			} else {
				fmt.Println("done")
			}
		}
	}

	// Initial peer sync
	fmt.Print("Syncing peers... ")
	if err := syncPeers(agentConfig, storage, dp, nil, nil, multiTunnel); err != nil {
		fmt.Printf("warning: %v\n", err)
	} else {
		fmt.Println("done")
	}

	// Note: Local advertised routes (agentConfig.AdvertisedRoutes) are included in
	// WireGuard AllowedIPs for remote peers. WireGuard automatically adds these to
	// the kernel routing table, but we don't need them locally.
	// So we remove them after peer configuration.
	if len(agentConfig.AdvertisedRoutes) > 0 && multiTunnel != nil {
		fmt.Print("Cleaning up local advertised routes from kernel... ")
		for _, route := range agentConfig.AdvertisedRoutes {
			// Delete the auto-added route by WireGuard
			cmd := exec.Command("ip", "route", "del", route)
			if err := cmd.Run(); err != nil {
				// Ignore errors - route might not exist
			}
		}
		fmt.Println("done")
	}

	fmt.Println()
	fmt.Printf("Aria is UP. VPN IP: %s\n", agentConfig.AssignedIP)
	if len(agentConfig.AdvertisedRoutes) > 0 {
		fmt.Printf("Advertising routes: %s\n", strings.Join(agentConfig.AdvertisedRoutes, ", "))
	}
	fmt.Println()

	if upDaemon {
		// Daemon mode: keep running and sync peers periodically
		return runDaemonLoop(agentConfig, storage, dp, multiTunnel)
	}

	fmt.Println("Run 'aria status' to check connection status.")
	fmt.Println("Run 'aria down' to disconnect.")
	return nil
}

func runDaemonLoop(agentConfig *config.AgentConfig, storage *agentstorage.Storage, dp *datapath.DataPath, multiTunnel *datapath.MultiTunnelManager) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// ========== 初始化 Prober 和 HealthManager ==========
	fmt.Print("Starting link quality monitor... ")

	// 创建 Prober（UDP 探测器）
	prober, err := monitor.NewProber()
	if err != nil {
		fmt.Printf("warning: failed to create prober: %v\n", err)
		prober = nil
	}

	// 创建路由引擎（用于健康管理器）
	routeEngine := route.NewEngine(agentConfig.Interface)

	// 创建 HealthManager
	var healthManager *monitor.HealthManager
	if prober != nil {
		healthManager = monitor.NewHealthManager(prober, routeEngine)
		healthManager.Start()

		// 启动 Prober（在 goroutine 中）
		go prober.Start()

		fmt.Println("done (port 51821)")
	} else {
		fmt.Println("skipped (prober unavailable)")
	}

	// 优雅关闭
	if prober != nil {
		defer prober.Stop()
	}
	if healthManager != nil {
		defer healthManager.Stop()
	}
	// ========== Prober 和 HealthManager 初始化完成 ==========

	// 启动 RPC server 用于 CLI 通信
	fmt.Print("Starting RPC server... ")
	daemon := rpc.NewDaemon(agentConfig, storage, Version)
	rpcServer := rpc.NewServer(daemon)
	if err := rpcServer.Start(); err != nil {
		fmt.Printf("warning: failed to start RPC server: %v\n", err)
	} else {
		fmt.Println("done (/var/run/aria.sock)")
	}
	defer rpcServer.Stop()

	// ========== 启动 Metrics Server ==========
	var metricsServer *metrics.Server
	var pusher *metrics.Pusher
	var agentControllerCollector *metrics.AgentControllerCollector

	if agentConfig.Metrics.Enabled {
		fmt.Print("Starting metrics server... ")

		// 获取版本号和主机名
		hostname := agentConfig.Hostname
		if hostname == "" {
			hostname, _ = os.Hostname()
		}

		// 创建带公共标签的 Registry
		labels := metrics.CommonLabels{
			HostID:  agentConfig.DeviceID[:16],
			Version: Version,
			Region:  agentConfig.Region,
			Role:    "agent",
		}
		registry := metrics.NewRegistry(labels)
		registerer := metrics.WrapRegistererWithLabels(registry, labels)

		// 创建采集器管理器
		manager := metrics.NewCollectorManager(registry)

		// 注册 WireGuard 采集器
		wgCollector := metrics.NewWireGuardCollector(dp.Tunnel, registerer)
		manager.Register(wgCollector)

		// 注册系统资源采集器
		// 获取网络信息用于监控标签
		localIP := getLocalIPAddr()
		publicIP := getPublicIPAddr()
		runtimeMode := string(agentConfig.RuntimeMode)
		if runtimeMode == "" {
			runtimeMode = "kernel"
		}
		sysCollector := metrics.NewSystemCollector(registerer, Version, commit, publicIP, localIP, runtimeMode)
		manager.Register(sysCollector)

		// 注册链路质量采集器（如果 Prober 可用）
		if prober != nil && healthManager != nil {
			healthCollector := metrics.NewHealthCollector(prober, healthManager, registerer)
			manager.Register(healthCollector)
		}

		// 注册 Agent Controller 连接状态采集器
		agentControllerCollector = metrics.NewAgentControllerCollector(registerer, storage)
		manager.Register(agentControllerCollector)

		// 注册防火墙采集器（如果 Firewall 可用）
		if dp != nil && dp.Firewall != nil {
			firewallCollector := metrics.NewFirewallCollector(dp.Firewall, registerer)
			manager.Register(firewallCollector)
		}

		// 启动 HTTP server
		collectInterval := agentConfig.Metrics.CollectInterval
		if collectInterval == 0 {
			collectInterval = 10 * time.Second
		}
		metricsServer = metrics.NewServer(agentConfig.Metrics.ListenAddr, manager, collectInterval)
		go func() {
			if err := metricsServer.Start(); err != nil {
				log.Printf("metrics server error: %v", err)
			}
		}()
		defer metricsServer.Stop()

		// 启动 Push Gateway（如果配置）
		if agentConfig.Metrics.PushGateway != "" {
			jobName := fmt.Sprintf("aria-agent-%s", hostname)

			// 准备 Pusher 选项
			pusherOpts := []metrics.PusherOption{}
			if agentConfig.Metrics.PushAuth.Username != "" {
				pusherOpts = append(pusherOpts, metrics.WithBasicAuth(
					agentConfig.Metrics.PushAuth.Username,
					agentConfig.Metrics.PushAuth.Password,
				))
			}

			pusher = metrics.NewPusher(agentConfig.Metrics.PushGateway, jobName, registry, pusherOpts...)

			pushInterval := agentConfig.Metrics.PushInterval
			if pushInterval == 0 {
				pushInterval = 15 * time.Second
			}
			go pusher.Start(pushInterval)
			defer pusher.Stop()

			fmt.Printf("done (%s, push to %s)\n", agentConfig.Metrics.ListenAddr, agentConfig.Metrics.PushGateway)
		} else {
			fmt.Printf("done (%s)\n", agentConfig.Metrics.ListenAddr)
		}
	}
	// ========== Metrics Server 启动完成 ==========

	syncFailCount := 0
	reloadSignalFile := "/var/run/aria.reload"

	for {
		select {
		case <-ticker.C:
			// Check for reload signal
			if _, err := os.Stat(reloadSignalFile); err == nil {
				// Signal file exists, trigger reload
				fmt.Println("Reload signal detected, re-registering with controller...")
				os.Remove(reloadSignalFile)

				// Reload config
				configMgr := config.NewManager("/etc/aria/agent.yaml")
				if newConfig, err := configMgr.Load(); err == nil {
					// Update in-memory config
					agentConfig.AdvertisedRoutes = newConfig.AdvertisedRoutes

					// Re-register with controller to sync advertised_routes
					if err := reRegisterWithController(agentConfig); err != nil {
						fmt.Printf("Warning: failed to re-register: %v\n", err)
					} else {
						fmt.Println("Re-registration successful, syncing peers...")
					}
				}
			}

			start := time.Now()
			err := syncPeers(agentConfig, storage, dp, prober, healthManager, multiTunnel)
			syncDuration := time.Since(start)

			if err != nil {
				// Record sync failure
				if agentControllerCollector != nil {
					agentControllerCollector.RecordSync(syncDuration, err)
				}
				syncFailCount++
				if syncFailCount%10 == 1 {
					fmt.Printf("Peer sync failed (%d): %v\n", syncFailCount, err)
				}
			} else {
				// Record sync success
				if agentControllerCollector != nil {
					agentControllerCollector.RecordSync(syncDuration, nil)
				}
				if syncFailCount > 0 {
					fmt.Printf("Peer sync recovered after %d failures\n", syncFailCount)
					syncFailCount = 0
				}
			}
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down...\n", sig)

			// 根据环境决定是否删除接口
			// 开发模式：删除接口以便调试
			// 生产模式：保留接口以维持数据面转发
			if os.Getenv("ARIA_ENV") == "dev" {
				fmt.Println("Development mode: cleaning up interface...")
				if err := dp.Tunnel.DeleteInterface(); err != nil {
					fmt.Printf("Warning: failed to delete interface: %v\n", err)
				}
			} else {
				fmt.Println("Production mode: keeping interface for data plane continuity.")
			}

			return nil
		}
	}
}

func generateKeys() (privateKey, publicKey string, err error) {
	cmd := exec.Command("wg", "genkey")
	privKey, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	privateKey = strings.TrimSpace(string(privKey))

	cmd = exec.Command("wg", "pubkey")
	cmd.Stdin = bytes.NewReader([]byte(privateKey))
	pubKey, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	publicKey = strings.TrimSpace(string(pubKey))

	return privateKey, publicKey, nil
}

func getLocalIPAddr() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "unknown"
}

func getPublicIPAddr() string {
	services := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				continue
			}

			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	return "unknown"
}

// createTLSConfig creates TLS configuration based on CA certificate path
func createTLSConfig(caCertPath string) (*tls.Config, error) {
	if caCertPath == "" {
		// No CA cert specified, log warning and allow insecure for development
		log.Println("WARNING: TLS certificate verification is disabled (no CA cert configured). This is insecure for production.")
		return &tls.Config{InsecureSkipVerify: true}, nil
	}

	// Load and verify CA certificate
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert file: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		RootCAs: certPool,
	}, nil
}

func registerWithCtrl(serverURL, token, publicKey, privateIP, publicIP, hostname string, advertisedRoutes []string, region, customerID string, sysInfo *capability.SystemInfo, caCertPath string) (string, string, error) {
	req := upRegisterRequest{
		PublicKey:        publicKey,
		Endpoint:         ":51820",
		PrivateIP:        privateIP,
		PublicIP:         publicIP,
		Hostname:         hostname,
		RegisteredAt:     time.Now().Unix(),
		Token:            token,
		AdvertisedRoutes: advertisedRoutes, // Site-to-Site VPN
		Region:           region,
		CustomerID:       customerID,
		RuntimeMode:      string(sysInfo.RuntimeMode),
		KernelVersion:    sysInfo.KernelVersion,
		HasAESNI:         sysInfo.HasAESNI,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", "", err
	}

	// Create HTTP client with TLS configuration
	tlsConfig, err := createTLSConfig(caCertPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create TLS config: %w", err)
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
	resp, err := client.Post(
		serverURL+"/register",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := strings.TrimSpace(string(respBody))
		// Provide user-friendly error messages
		switch {
		case strings.Contains(errMsg, "Invalid token"):
			return "", "", fmt.Errorf("Invalid token")
		case strings.Contains(errMsg, "expired"):
			return "", "", fmt.Errorf("Token expired")
		case strings.Contains(errMsg, "exhausted"):
			return "", "", fmt.Errorf("Token exhausted (max uses reached)")
		case strings.Contains(errMsg, "revoked"):
			return "", "", fmt.Errorf("Token revoked")
		default:
			return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, errMsg)
		}
	}

	var syncResp upSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return "", "", err
	}

	return syncResp.AssignedIP, syncResp.MetricsPushGateway, nil
}

// fixMetricsGateway replaces internal VictoriaMetrics address with actual controller URL
func fixMetricsGateway(gateway, controllerURL string) string {
	// Always use controller URL + /victoria path (Nginx handles routing to VictoriaMetrics)
	// This ensures metrics are pushed through Nginx with proper TLS
	return controllerURL + "/victoria/api/v1/import/prometheus"
}

func createWgInterface(ifaceName, assignedIP, privateKey string, mode capability.RuntimeMode) (string, error) {
	if mode == capability.ModeUserspace {
		return createWgInterfaceUserspace(ifaceName, assignedIP, privateKey)
	}
	return createWgInterfaceKernel(ifaceName, assignedIP, privateKey)
}

func createWgInterfaceKernel(ifaceName, assignedIP, privateKey string) (string, error) {
	// Create interface
	cmd := exec.Command("ip", "link", "add", "dev", ifaceName, "type", "wireguard")
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "File exists") {
			return "", fmt.Errorf("failed to create interface: %w, output: %s", err, output)
		}
	}

	// Set private key
	cmd = exec.Command("wg", "set", ifaceName, "private-key", "/dev/stdin")
	cmd.Stdin = strings.NewReader(privateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to set private key: %w, output: %s", err, output)
	}

	// Set IP address
	cmd = exec.Command("ip", "addr", "add", assignedIP+"/16", "dev", ifaceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "File exists") {
			return "", fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
		}
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", "up", "dev", ifaceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to bring interface up: %w, output: %s", err, output)
	}

	// Set listen port
	cmd = exec.Command("wg", "set", ifaceName, "listen-port", "51820")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to set listen port: %w, output: %s", err, output)
	}

	return ifaceName, nil
}

func createWgInterfaceUserspace(ifaceName, assignedIP, privateKey string) (string, error) {
	// Clean up any existing interface
	if runtime.GOOS != "darwin" {
		exec.Command("ip", "link", "del", ifaceName).Run()
	}

	// Check for wireguard-go
	wgGoPath, err := exec.LookPath("wireguard-go")
	if err != nil {
		return "", fmt.Errorf("wireguard-go not found: %w (install with: brew install wireguard-go)", err)
	}

	// macOS: Get existing utun interfaces before creating new one
	var existingUtuns []string
	if runtime.GOOS == "darwin" {
		output, _ := exec.Command("ifconfig", "-l").Output()
		for _, iface := range strings.Fields(string(output)) {
			if strings.HasPrefix(iface, "utun") {
				existingUtuns = append(existingUtuns, iface)
			}
		}
	}

	// Start wireguard-go
	cmd := exec.Command(wgGoPath, ifaceName)
	cmd.Env = append(os.Environ(), "WG_I_PREFER_BUGGY_USERSPACE_TO_POLISHED_KMOD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to start wireguard-go: %w, output: %s", err, output)
	}

	// macOS: Find the newly created utun interface
	actualIfaceName := ifaceName
	if runtime.GOOS == "darwin" {
		for i := 0; i < 10; i++ {
			output, _ := exec.Command("ifconfig", "-l").Output()
			for _, iface := range strings.Fields(string(output)) {
				if strings.HasPrefix(iface, "utun") {
					isNew := true
					for _, existing := range existingUtuns {
						if iface == existing {
							isNew = false
							break
						}
					}
					if isNew {
						actualIfaceName = iface
						break
					}
				}
			}
			if actualIfaceName != ifaceName {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		ifaceName = actualIfaceName
	}

	// Wait for interface
	for i := 0; i < 10; i++ {
		var checkCmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			checkCmd = exec.Command("ifconfig", ifaceName)
		} else {
			checkCmd = exec.Command("ip", "link", "show", ifaceName)
		}
		if _, err := checkCmd.Output(); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Set MTU
	if runtime.GOOS == "darwin" {
		if output, err := exec.Command("ifconfig", ifaceName, "mtu", "1360").CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
		}
	} else {
		if output, err := exec.Command("ip", "link", "set", ifaceName, "mtu", "1360").CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
		}
	}

	// Set private key and listen port
	cmd = exec.Command("wg", "set", ifaceName, "private-key", "/dev/stdin", "listen-port", "51820")
	cmd.Stdin = strings.NewReader(privateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to set private key: %w, output: %s", err, output)
	}

	// Set IP address
	if runtime.GOOS == "darwin" {
		// macOS: ifconfig utun inet 100.64.0.5 100.64.0.5 netmask 255.255.0.0
		cmd = exec.Command("ifconfig", ifaceName, "inet", assignedIP, assignedIP, "netmask", "255.255.0.0")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
		}
	} else {
		cmd = exec.Command("ip", "addr", "add", assignedIP+"/16", "dev", ifaceName)
		if output, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "File exists") {
				return "", fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
			}
		}

		// Bring interface up (Linux only)
		cmd = exec.Command("ip", "link", "set", "up", "dev", ifaceName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to bring interface up: %w, output: %s", err, output)
		}
	}

	return ifaceName, nil
}

func deleteWgInterface(ifaceName string) error {
	if runtime.GOOS == "darwin" {
		// macOS: kill wireguard-go process, it will clean up the interface
		exec.Command("pkill", "-f", "wireguard-go.*"+ifaceName).Run()
		return nil
	}

	cmd := exec.Command("ip", "link", "delete", "dev", ifaceName)
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to delete interface: %w, output: %s", err, output)
	}
	return nil
}

func reRegisterWithController(agentConfig *config.AgentConfig) error {
	// Get current environment info
	privateIP := getLocalIPAddr()
	publicIP := getPublicIPAddr()
	hostname, _ := os.Hostname()

	sysInfo := &capability.SystemInfo{
		RuntimeMode:   capability.RuntimeMode(agentConfig.RuntimeMode),
		KernelVersion: "unknown",
		HasAESNI:      false,
	}

	// Re-register (without token - already registered)
	_, _, err := registerWithCtrl(
		agentConfig.ControllerURL,
		"", // No token needed for re-registration
		agentConfig.PublicKey,
		privateIP,
		publicIP,
		hostname,
		agentConfig.AdvertisedRoutes,
		agentConfig.Region,
		agentConfig.CustomerID,
		sysInfo,
		agentConfig.CACert,
	)

	return err
}

// syncWithRetry performs sync with exponential backoff retry
func syncWithRetry(agentConfig *config.AgentConfig, client *http.Client, publicKey string) (*http.Response, error) {
	maxRetries := 3
	baseDelay := 500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<attempt) // exponential backoff
			log.Printf("Retry %d/%d after %v", attempt, maxRetries-1, delay)
			time.Sleep(delay)
		}

		syncURL := fmt.Sprintf("%s/sync", agentConfig.ControllerURL)
		req, err := http.NewRequest("GET", syncURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}
		req.Header.Set("X-Public-Key", publicKey)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Success or non-retryable error
		if resp.StatusCode < 500 {
			return resp, nil
		}

		// Server error, retry
		resp.Body.Close()
		lastErr = fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	return nil, lastErr
}

func syncPeers(agentConfig *config.AgentConfig, storage *agentstorage.Storage, dp *datapath.DataPath, prober *monitor.Prober, healthMgr *monitor.HealthManager, multiTunnel *datapath.MultiTunnelManager) error {
	// Create TLS configuration
	tlsConfig, err := createTLSConfig(agentConfig.CACert)
	if err != nil {
		return fmt.Errorf("failed to create TLS config: %w", err)
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	// Use exponential backoff retry for sync
	resp, err := syncWithRetry(agentConfig, client, agentConfig.PublicKey)
	if err != nil {
		// Controller unreachable after retries: use local cache
		fmt.Printf("Controller unreachable after retries, using local cache: %v\n", err)
		return applyLocalPeers(agentConfig, storage, dp, prober, healthMgr, multiTunnel)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Controller error: use local cache
		fmt.Printf("Controller returned HTTP %d, using local cache\n", resp.StatusCode)
		return applyLocalPeers(agentConfig, storage, dp, prober, healthMgr, multiTunnel)
	}

	var syncResp upSyncResponseWithPeers
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return fmt.Errorf("failed to decode sync response: %w", err)
	}


	// Convert to storage format and save to cache
	var cachePeers []agentstorage.PeerConfig
	now := time.Now().Unix()

	// Update WireGuard peers
	for _, peer := range syncResp.Peers {
		if peer.PublicKey == agentConfig.PublicKey {
			continue
		}

		// Each peer should only allow its own VPN IP (not the entire subnet)
		// This ensures WireGuard can correctly route traffic to the right peer
		if peer.AssignedIP == "" {
			fmt.Printf("Warning: skipping peer %s: no assigned IP\n", peer.PublicKey[:8])
			continue
		}

		// Build AllowedIPs = Peer's VPN IP/32 + Advertised Routes
		allowedIPsList := []string{peer.AssignedIP + "/32"}
		if len(peer.AdvertisedRoutes) > 0 {
			allowedIPsList = append(allowedIPsList, peer.AdvertisedRoutes...)
		}
		endpoint := peer.PublicIP + ":51820"

		// Apply to WireGuard using datapath
		peerCfg := &datapath.PeerConfig{
			PublicKey:           peer.PublicKey,
			Endpoint:            endpoint,
			AllowedIPs:          allowedIPsList,
			PersistentKeepalive: 25,
		}

		// If multi-tunnel is enabled, configure peers on all tunnels
		if multiTunnel != nil {
			// Convert peer config to slice for multi-tunnel
			peers := []*datapath.PeerConfig{peerCfg}
			if err := multiTunnel.ConfigureAllPeers(peers); err != nil {
				fmt.Printf("Warning: failed to configure peer %s on multi-tunnel: %v\n", peer.PublicKey[:8], err)
				continue
			}

			// Setup ECMP routes for advertised routes
			if len(peer.AdvertisedRoutes) > 0 {
				if err := multiTunnel.SetupECMPRoutes(peer.AdvertisedRoutes); err != nil {
					fmt.Printf("Warning: failed to setup ECMP routes for peer %s: %v\n", peer.PublicKey[:8], err)
				}
			}
		} else {
			// Single tunnel mode
			if err := dp.Tunnel.AddPeer(peerCfg); err != nil {
				fmt.Printf("Warning: failed to configure peer %s: %v\n", peer.PublicKey[:8], err)
				continue
			}
		}

		// 添加到 Prober 和 HealthManager（链路质量监控）
		if prober != nil {
			prober.AddPeer(peer.PublicKey, peer.AssignedIP)
		}
		if healthMgr != nil {
			// 提取 VPC ID（从 peer 数据结构中获取，如果没有则使用空字符串）
			vpcID := peer.VPCID
			if vpcID == "" {
				vpcID = "default"
			}
			healthMgr.AddPeer(peer.PublicKey, peer.AssignedIP, vpcID)
		}

		// Manage advertised routes in system routing table
		// Get old advertised routes from storage
		oldPeer, _ := storage.GetPeerByPublicKey(peer.PublicKey)
		var oldRoutes []string
		if oldPeer != nil {
			oldRoutes = oldPeer.AllowedIPs
			// Filter out /32 (peer's VPN IP)
			var oldAdvertisedRoutes []string
			for _, route := range oldRoutes {
				if !strings.HasSuffix(route, "/32") {
					oldAdvertisedRoutes = append(oldAdvertisedRoutes, route)
				}
			}
			oldRoutes = oldAdvertisedRoutes
		}

		// Build map of new advertised routes
		newRoutesMap := make(map[string]bool)
		for _, route := range peer.AdvertisedRoutes {
			newRoutesMap[route] = true
		}

		// Delete routes that no longer exist using datapath
		for _, oldRoute := range oldRoutes {
			if !newRoutesMap[oldRoute] {
				if multiTunnel != nil {
					// Multi-tunnel: remove ECMP route
					dp.Route.RemoveECMPRoute(oldRoute)
				} else {
					// Single tunnel: remove regular route
					dp.Route.RemoveRoute(oldRoute)
				}
			}
		}

		// Add new advertised routes to system routing table using datapath
		if len(peer.AdvertisedRoutes) > 0 {
			for _, route := range peer.AdvertisedRoutes {
				// Skip if route already exists
				if oldPeer != nil {
					found := false
					for _, oldRoute := range oldRoutes {
						if oldRoute == route {
							found = true
							break
						}
					}
					if found {
						continue // Route already exists, skip
					}
				}

				// Add new route
				if multiTunnel != nil {
					// Multi-tunnel: add ECMP route
					interfaces := make([]string, len(multiTunnel.GetTunnels()))
					for i, tunnel := range multiTunnel.GetTunnels() {
						interfaces[i] = tunnel.Name
					}
					if err := dp.Route.AddECMPRoute(route, interfaces); err != nil && err != datapath.ErrRouteExists {
						fmt.Printf("Warning: failed to add ECMP route %s: %v\n", route, err)
					}
				} else {
					// Single tunnel: add regular route
					routeEntry := &datapath.RouteEntry{
						Destination: route,
						Interface:   agentConfig.Interface,
						Metric:      200,
					}
					if err := dp.Route.AddRoute(routeEntry); err != nil && err != datapath.ErrRouteExists {
						fmt.Printf("Warning: failed to add route %s: %v\n", route, err)
					}
				}
			}
		}

		// Add to cache list
		cachePeers = append(cachePeers, agentstorage.PeerConfig{
			PublicKey:           peer.PublicKey,
			Endpoint:            endpoint,
			AllowedIPs:          allowedIPsList, // Store as array
			PersistentKeepalive: 25,
			AddedAt:             now,
			AssignedIP:          peer.AssignedIP,
			Hostname:            peer.Hostname,
			Region:              peer.Region,
			VPCID:               peer.VPCID,
			Status:              "online",
			LastSeen:            now,
		})
	}

	// Save peers to local cache
	if err := storage.SavePeers(cachePeers); err != nil {
		fmt.Printf("Warning: failed to save peers to cache: %v\n", err)
	}

	// Apply ACL rules if firewall is enabled
	if len(syncResp.ACLRules) > 0 && dp.Firewall.IsEnabled() {
		aclRules := make([]datapath.ACLRule, len(syncResp.ACLRules))
		for i, rule := range syncResp.ACLRules {
			aclRules[i] = datapath.ACLRule{
				SrcNet:   rule.SrcNet,
				DstNet:   rule.DstNet,
				Protocol: rule.Protocol,
				MinPort:  rule.MinPort,
				MaxPort:  rule.MaxPort,
			}
		}
		if err := dp.Firewall.ApplyPolicy(aclRules); err != nil {
			fmt.Printf("Warning: failed to apply ACL rules: %v\n", err)
		} else {
			log.Printf("Applied %d ACL rules from controller\n", len(aclRules))

			// Cache ACL rules to local storage (Fail-Static support)
			storageRules := make([]agentstorage.ACLRule, len(syncResp.ACLRules))
			for i, rule := range syncResp.ACLRules {
				storageRules[i] = agentstorage.ACLRule{
					SrcNet:   rule.SrcNet,
					DstNet:   rule.DstNet,
					Protocol: rule.Protocol,
					MinPort:  rule.MinPort,
					MaxPort:  rule.MaxPort,
				}
			}
			if err := storage.SaveACLRules(storageRules); err != nil {
				log.Printf("Warning: failed to cache ACL rules: %v", err)
			}
		}
	} else if dp.Firewall.IsEnabled() {
		// No ACL rules from controller, but firewall is enabled
		// Try to load cached rules (Fail-Static behavior)
		cachedRules, err := storage.LoadACLRules()
		if err == nil && len(cachedRules) > 0 {
			log.Printf("Using %d cached ACL rules (Controller returned no rules)\n", len(cachedRules))
		}
	}

	// Update metrics push gateway if provided by controller
	if syncResp.MetricsPushGateway != "" {
		// Fix the gateway address to use external URL
		fixedGateway := fixMetricsGateway(syncResp.MetricsPushGateway, agentConfig.ControllerURL)
		if fixedGateway != agentConfig.Metrics.PushGateway {
			log.Printf("Updating metrics push gateway: %s", fixedGateway)
			agentConfig.Metrics.PushGateway = fixedGateway

			// Save updated config
			configMgr := config.NewManager("/etc/aria/agent.yaml")
			if err := configMgr.Save(agentConfig); err != nil {
				log.Printf("Warning: failed to save metrics push gateway: %v", err)
			}
		}
	}

	return nil
}

// applyLocalPeers loads peers from cache and applies to WireGuard using datapath
func applyLocalPeers(agentConfig *config.AgentConfig, storage *agentstorage.Storage, dp *datapath.DataPath, prober *monitor.Prober, healthMgr *monitor.HealthManager, multiTunnel *datapath.MultiTunnelManager) error {
	peers, err := storage.LoadPeers()
	if err != nil {
		return fmt.Errorf("failed to load cached peers: %w", err)
	}

	if len(peers) == 0 {
		return fmt.Errorf("no cached peers available")
	}

	fmt.Printf("Applying %d peers from local cache\n", len(peers))

	for _, peer := range peers {
		// Skip deleted peers
		if peer.Status == "deleted" {
			continue
		}

		// Skip self
		if peer.PublicKey == agentConfig.PublicKey {
			continue
		}

		if len(peer.AllowedIPs) == 0 || peer.AssignedIP == "" {
			continue
		}

		// Apply to WireGuard using datapath
		peerCfg := &datapath.PeerConfig{
			PublicKey:           peer.PublicKey,
			Endpoint:            peer.Endpoint,
			AllowedIPs:          peer.AllowedIPs,
			PersistentKeepalive: peer.PersistentKeepalive,
		}

		// If multi-tunnel is enabled, configure peers on all tunnels
		if multiTunnel != nil {
			peers := []*datapath.PeerConfig{peerCfg}
			if err := multiTunnel.ConfigureAllPeers(peers); err != nil {
				fmt.Printf("Warning: failed to apply cached peer %s on multi-tunnel: %v\n", peer.PublicKey[:8], err)
			}
		} else {
			if err := dp.Tunnel.AddPeer(peerCfg); err != nil {
				fmt.Printf("Warning: failed to apply cached peer %s: %v\n", peer.PublicKey[:8], err)
			}
		}

		// 添加到 Prober 和 HealthManager
		if prober != nil {
			prober.AddPeer(peer.PublicKey, peer.AssignedIP)
		}
		if healthMgr != nil {
			vpcID := peer.VPCID
			if vpcID == "" {
				vpcID = "default"
			}
			healthMgr.AddPeer(peer.PublicKey, peer.AssignedIP, vpcID)
		}
	}

	// Apply cached ACL rules if firewall is enabled (Fail-Static support)
	if dp.Firewall.IsEnabled() {
		cachedRules, err := storage.LoadACLRules()
		if err != nil {
			fmt.Printf("Warning: failed to load cached ACL rules: %v\n", err)
		} else if len(cachedRules) > 0 {
			fmt.Printf("Applying %d ACL rules from local cache\n", len(cachedRules))
			aclRules := make([]datapath.ACLRule, len(cachedRules))
			for i, rule := range cachedRules {
				aclRules[i] = datapath.ACLRule{
					SrcNet:   rule.SrcNet,
					DstNet:   rule.DstNet,
					Protocol: rule.Protocol,
					MinPort:  rule.MinPort,
					MaxPort:  rule.MaxPort,
				}
			}
			if err := dp.Firewall.ApplyPolicy(aclRules); err != nil {
				fmt.Printf("Warning: failed to apply cached ACL rules: %v\n", err)
			}
		} else {
			fmt.Println("Warning: No cached ACL rules available (firewall may block all traffic)")
		}
	}

	return nil
}

// loadAgentConfig loads the agent configuration from file
func loadAgentConfig(path string) (*config.AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config.AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
