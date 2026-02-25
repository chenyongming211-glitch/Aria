//go:build darwin

package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ensureUserspaceInterface creates WireGuard interface using wireguard-go on macOS
func (m *Manager) ensureUserspaceInterface(privateKey string) error {
	// On macOS, we use utun interfaces via wireguard-go

	// 1. Check if wireguard-go is available
	wgGoPath, err := exec.LookPath("wireguard-go")
	if err != nil {
		// Try common installation paths
		for _, path := range []string{"/usr/local/bin/wireguard-go", "/opt/homebrew/bin/wireguard-go"} {
			if _, err := os.Stat(path); err == nil {
				wgGoPath = path
				break
			}
		}
		if wgGoPath == "" {
			return fmt.Errorf("wireguard-go not found (install with: brew install wireguard-go)")
		}
	}

	// 2. Start wireguard-go with utun interface
	// On macOS, wireguard-go creates utun interfaces
	cmd := exec.Command(wgGoPath, "utun")
	cmd.Env = append(os.Environ(), "WG_TUN_NAME_FILE=/tmp/wg-tun-name")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start wireguard-go: %w, output: %s", err, output)
	}

	// 3. Read the actual interface name
	tunNameBytes, err := os.ReadFile("/tmp/wg-tun-name")
	if err != nil {
		return fmt.Errorf("failed to read tunnel name: %w", err)
	}
	actualIfaceName := strings.TrimSpace(string(tunNameBytes))
	if actualIfaceName != "" {
		m.interfaceName = actualIfaceName
	}

	// 4. Configure private key using wg command
	cmd = exec.Command("wg", "set", m.interfaceName, "private-key", "/dev/stdin", "listen-port", "51820")
	cmd.Stdin = strings.NewReader(privateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, output)
	}

	m.privateKey = privateKey
	return nil
}
