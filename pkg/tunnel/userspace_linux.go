//go:build linux

package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ensureUserspaceInterface creates WireGuard interface using wireguard-go
func (m *Manager) ensureUserspaceInterface(privateKey string) error {
	// 1. Clean up any existing interface
	exec.Command("ip", "link", "del", m.interfaceName).Run()

	// 2. Check if wireguard-go is available
	wgGoPath, err := exec.LookPath("wireguard-go")
	if err != nil {
		return fmt.Errorf("wireguard-go not found in PATH: %w (install with: apt install wireguard-tools)", err)
	}

	// 3. Start wireguard-go to create the interface
	cmd := exec.Command(wgGoPath, m.interfaceName)
	cmd.Env = append(os.Environ(), "WG_I_PREFER_BUGGY_USERSPACE_TO_POLISHED_KMOD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start wireguard-go: %w, output: %s", err, output)
	}

	// 4. Wait for interface to be ready
	for i := 0; i < 10; i++ {
		if _, err := exec.Command("ip", "link", "show", m.interfaceName).Output(); err == nil {
			break
		}
		exec.Command("sleep", "0.1").Run()
	}

	// 5. Set MTU
	if output, err := exec.Command("ip", "link", "set", m.interfaceName, "mtu", "1360").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set MTU: %w, output: %s", err, output)
	}

	// 6. Configure private key using wg command
	cmd = exec.Command("wg", "set", m.interfaceName, "private-key", "/dev/stdin", "listen-port", "51820")
	cmd.Stdin = strings.NewReader(privateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, output)
	}

	// 7. Bring interface up
	if output, err := exec.Command("ip", "link", "set", m.interfaceName, "up").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %w, output: %s", err, output)
	}

	// 8. Enable IP forwarding
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	m.privateKey = privateKey
	return nil
}
