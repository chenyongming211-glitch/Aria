//go:build darwin

package capability

import (
	"os/exec"
	"runtime"
	"strings"
)

func init() {
	platformDetector = func() Detector {
		return &DarwinDetector{}
	}
}

// DarwinDetector detects capabilities on macOS
type DarwinDetector struct{}

// Detect returns system capabilities on macOS
func (d *DarwinDetector) Detect() (*SystemInfo, error) {
	info := &SystemInfo{
		KernelVersion: d.getKernelVersion(),
		HasKernelWG:   false, // macOS doesn't have kernel WireGuard
		HasAESNI:      d.checkAESNI(),
		RuntimeMode:   ModeUserspace, // Always userspace on macOS
		CPUCores:      runtime.NumCPU(),
		IsContainer:   false, // Containers on macOS use Linux VMs
	}

	return info, nil
}

// getKernelVersion returns the macOS/Darwin kernel version
func (d *DarwinDetector) getKernelVersion() string {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return "darwin"
	}
	return strings.TrimSpace(string(output))
}

// checkAESNI checks if the CPU supports AES-NI
// Modern Intel/Apple Silicon Macs all support hardware AES
func (d *DarwinDetector) checkAESNI() bool {
	// Check for Intel AES-NI
	cmd := exec.Command("sysctl", "-n", "hw.optional.aes")
	output, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(output)) == "1" {
		return true
	}

	// Apple Silicon always has hardware crypto
	cmd = exec.Command("uname", "-m")
	output, err = cmd.Output()
	if err == nil && strings.Contains(string(output), "arm64") {
		return true
	}

	return false
}
