package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"aria/pkg/config"
	"aria/pkg/wgmanager"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Disconnect from the Aria network",
	Long: `Disconnect from the Aria network and remove the WireGuard interface.

This command:
  1. Removes the WireGuard interface
  2. Optionally removes the configuration (with --purge)

The configuration file is preserved by default, allowing you to reconnect
with 'aria up' without needing to re-register.

Examples:
  aria down
  aria down --purge    # Also remove configuration`,
	RunE: runDown,
}

var downPurge bool

func init() {
	rootCmd.AddCommand(downCmd)
	downCmd.Flags().BoolVar(&downPurge, "purge", false, "Also remove configuration files")
}

func runDown(cmd *cobra.Command, args []string) error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	configPath := "/etc/aria/agent.yaml"
	if cfgFile != "" {
		configPath = cfgFile
	}

	configMgr := config.NewManager(configPath)

	// Load config to get interface name
	ifaceName := wgmanager.DefaultInterfaceName
	if runtime.GOOS == "darwin" {
		ifaceName = wgmanager.DefaultMacOSInterface
	}
	if configMgr.Exists() {
		cfg, err := configMgr.Load()
		if err == nil && cfg.Interface != "" {
			ifaceName = cfg.Interface
		}
	}

	// 1. 先删除 WireGuard 接口（在停止进程之前）
	fmt.Printf("Removing WireGuard interface %s... ", ifaceName)
	runtimeMode := "kernel"
	if configMgr.Exists() {
		if cfg, err := configMgr.Load(); err == nil && cfg.RuntimeMode != "" {
			runtimeMode = cfg.RuntimeMode
		}
	}
	ifaceManager := wgmanager.NewInterfaceManager(ifaceName, runtimeMode)
	if err := ifaceManager.DeleteInterface(ifaceName); err != nil {
		fmt.Printf("warning: %v\n", err)
	} else {
		fmt.Println("done")
	}

	// 2. 检查是否由 systemd 管理
	if isSystemdManaged() {
		fmt.Print("Stopping systemd service... ")
		if err := stopSystemdService(); err != nil {
			fmt.Printf("failed: %v\n", err)
			return fmt.Errorf("failed to stop systemd service: %w", err)
		}
		fmt.Println("done")
	} else {
		// 3. 手动模式：检查是否有运行中的进程
		running, pidStr, err := wgmanager.IsRunning()
		if err != nil {
			fmt.Printf("Warning: failed to check running status: %v\n", err)
		}

		if running {
			fmt.Printf("Stopping aria daemon (PID: %s)... ", pidStr)
			if err := stopDaemon(pidStr); err != nil {
				fmt.Printf("failed: %v\n", err)
				return fmt.Errorf("failed to stop daemon: %w", err)
			}
			fmt.Println("done")
		}
	}

	if downPurge {
		fmt.Print("Removing configuration... ")
		if err := configMgr.Delete(); err != nil {
			fmt.Printf("warning: %v\n", err)
		} else {
			fmt.Println("done")
		}
	}

	fmt.Println()
	fmt.Println("Aria is DOWN.")
	if !downPurge {
		fmt.Println("Configuration preserved. Run 'aria up' to reconnect.")
	}
	fmt.Println()

	return nil
}

// stopDaemon 停止运行中的 daemon 进程
func stopDaemon(pidStr string) error {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID: %s", pidStr)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	// 发送 SIGTERM 信号优雅退出
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// 等待进程退出（最多 5 秒）
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		// 检查进程是否还存在
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// 进程已退出
			return nil
		}
	}

	// 超时后强制 kill
	return process.Signal(syscall.SIGKILL)
}

// isSystemdManaged 检查当前是否由 systemd 管理
func isSystemdManaged() bool {
	// 检查 systemd service 是否存在且 active
	cmd := exec.Command("systemctl", "is-active", "aria.service")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	status := strings.TrimSpace(string(output))
	return status == "active" || status == "activating"
}

// stopSystemdService 停止 systemd 服务
func stopSystemdService() error {
	cmd := exec.Command("systemctl", "stop", "aria.service")
	return cmd.Run()
}
