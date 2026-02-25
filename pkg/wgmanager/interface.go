package wgmanager

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultInterfaceName 是 Aria 专用的 WireGuard 接口名
	// 使用 aria0 而不是 wg0，避免与用户自己安装的 WireGuard 冲突
	DefaultInterfaceName = "aria0"

	// macOS 使用 utun 接口（自动编号）
	DefaultMacOSInterface = "utun"
)

// InterfaceManager 管理 WireGuard 接口的生命周期
type InterfaceManager struct {
	interfaceName string
	runtimeMode   string // "kernel" or "userspace"
}

// NewInterfaceManager 创建接口管理器
func NewInterfaceManager(interfaceName, runtimeMode string) *InterfaceManager {
	if interfaceName == "" {
		interfaceName = DefaultInterfaceName
		if runtime.GOOS == "darwin" {
			interfaceName = DefaultMacOSInterface
		}
	}
	return &InterfaceManager{
		interfaceName: interfaceName,
		runtimeMode:   runtimeMode,
	}
}

// EnsureInterface 确保 WireGuard 接口存在且配置正确（幂等性）
// 无论执行多少次，结果都是一样的：有一个配置正确的接口存在
func (m *InterfaceManager) EnsureInterface(privateKey, assignedIP string) (string, error) {
	// 1. 检查接口是否已存在
	exists, actualName, err := m.checkInterfaceExists()
	if err != nil {
		return "", fmt.Errorf("failed to check interface: %v", err)
	}

	if exists {
		fmt.Printf("♻️ [复用] 发现现有接口 %s，验证配置...\n", actualName)

		// 验证接口是否是 WireGuard 类型
		if !m.isWireGuardInterface(actualName) {
			fmt.Printf("⚠️ [冲突] 接口 %s 不是 WireGuard 类型，正在清理...\n", actualName)
			if err := m.deleteInterface(actualName); err != nil {
				return "", fmt.Errorf("failed to delete conflicting interface: %v", err)
			}
			// 继续创建新接口
		} else {
			// 接口存在且类型正确，复用它
			// 更新配置以确保一致性
			if err := m.configureInterface(actualName, privateKey, assignedIP); err != nil {
				return "", fmt.Errorf("failed to configure existing interface: %v", err)
			}
			return actualName, nil
		}
	}

	// 2. 接口不存在或已被删除，创建新接口
	fmt.Printf("🆕 [创建] 正在初始化新接口 %s...\n", m.interfaceName)
	actualName, err = m.createInterface(privateKey, assignedIP)
	if err != nil {
		return "", fmt.Errorf("failed to create interface: %v", err)
	}

	return actualName, nil
}

// checkInterfaceExists 检查接口是否存在
func (m *InterfaceManager) checkInterfaceExists() (bool, string, error) {
	if runtime.GOOS == "darwin" {
		// macOS: 检查是否有 wireguard-go 进程运行
		output, err := exec.Command("pgrep", "-f", "wireguard-go").Output()
		if err != nil || len(output) == 0 {
			return false, "", nil
		}

		// 查找 utun 接口
		output, err = exec.Command("ifconfig", "-l").Output()
		if err != nil {
			return false, "", err
		}

		for _, iface := range strings.Fields(string(output)) {
			if strings.HasPrefix(iface, "utun") {
				// 检查这个 utun 是否是 WireGuard 接口
				cmd := exec.Command("wg", "show", iface)
				if err := cmd.Run(); err == nil {
					return true, iface, nil
				}
			}
		}
		return false, "", nil
	}

	// Linux: 检查接口是否存在
	cmd := exec.Command("ip", "link", "show", m.interfaceName)
	if err := cmd.Run(); err != nil {
		return false, "", nil
	}
	return true, m.interfaceName, nil
}

// isWireGuardInterface 验证接口是否是 WireGuard 类型
func (m *InterfaceManager) isWireGuardInterface(ifaceName string) bool {
	cmd := exec.Command("wg", "show", ifaceName)
	return cmd.Run() == nil
}

// createInterface 创建新的 WireGuard 接口
func (m *InterfaceManager) createInterface(privateKey, assignedIP string) (string, error) {
	actualIfaceName := m.interfaceName

	// Linux kernel mode
	if runtime.GOOS == "linux" && m.runtimeMode == "kernel" {
		// Create wireguard interface using ip link
		cmd := exec.Command("ip", "link", "add", m.interfaceName, "type", "wireguard")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to create interface: %w, output: %s", err, output)
		}
	} else {
		// Userspace mode (macOS or Linux userspace)
		wgGoPath := "wireguard-go"

		// macOS: 获取现有 utun 列表（用于检测新创建的接口）
		var existingUtuns []string
		if runtime.GOOS == "darwin" {
			output, _ := exec.Command("ifconfig", "-l").Output()
			for _, iface := range strings.Fields(string(output)) {
				if strings.HasPrefix(iface, "utun") {
					existingUtuns = append(existingUtuns, iface)
				}
			}
		}

		// 启动 wireguard-go
		cmd := exec.Command(wgGoPath, m.interfaceName)
		cmd.Env = append(os.Environ(), "WG_I_PREFER_BUGGY_USERSPACE_TO_POLISHED_KMOD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to start wireguard-go: %w, output: %s", err, output)
		}

		// macOS: 找到新创建的 utun 接口
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
				if actualIfaceName != m.interfaceName {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	// 等待接口就绪
	if err := m.waitForInterface(actualIfaceName); err != nil {
		return "", err
	}

	// 配置接口
	if err := m.configureInterface(actualIfaceName, privateKey, assignedIP); err != nil {
		return "", err
	}

	return actualIfaceName, nil
}

// configureInterface 配置 WireGuard 接口
func (m *InterfaceManager) configureInterface(ifaceName, privateKey, assignedIP string) error {
	// 设置 MTU
	if runtime.GOOS == "darwin" {
		if output, err := exec.Command("ifconfig", ifaceName, "mtu", "1360").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
		}
	} else {
		if output, err := exec.Command("ip", "link", "set", ifaceName, "mtu", "1360").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
		}
	}

	// 设置私钥和监听端口
	cmd := exec.Command("wg", "set", ifaceName, "private-key", "/dev/stdin", "listen-port", "51820")
	cmd.Stdin = strings.NewReader(privateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, output)
	}

	// 设置 IP 地址
	if runtime.GOOS == "darwin" {
		// macOS: ifconfig utun inet 100.64.0.5 100.64.0.5 netmask 255.255.0.0
		cmd = exec.Command("ifconfig", ifaceName, "inet", assignedIP, assignedIP, "netmask", "255.255.0.0")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
		}
	} else {
		cmd = exec.Command("ip", "addr", "add", assignedIP+"/16", "dev", ifaceName)
		if output, err := cmd.CombinedOutput(); err != nil {
			// 忽略 IP 已存在的错误（复用接口时会出现）
			outputStr := string(output)
			if !strings.Contains(outputStr, "File exists") &&
			   !strings.Contains(outputStr, "Address already assigned") {
				return fmt.Errorf("failed to set IP address: %w, output: %s", err, output)
			}
		}

		// Bring interface up (Linux only)
		cmd = exec.Command("ip", "link", "set", "up", "dev", ifaceName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to bring interface up: %w, output: %s", err, output)
		}
	}

	return nil
}

// waitForInterface 等待接口就绪
func (m *InterfaceManager) waitForInterface(ifaceName string) error {
	for i := 0; i < 10; i++ {
		var checkCmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			checkCmd = exec.Command("ifconfig", ifaceName)
		} else {
			checkCmd = exec.Command("ip", "link", "show", ifaceName)
		}
		if _, err := checkCmd.Output(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("interface %s not ready after 1 second", ifaceName)
}

// deleteInterface 删除接口
func (m *InterfaceManager) deleteInterface(ifaceName string) error {
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

// DeleteInterface 删除接口（public 方法）
func (m *InterfaceManager) DeleteInterface(ifaceName string) error {
	return m.deleteInterface(ifaceName)
}
