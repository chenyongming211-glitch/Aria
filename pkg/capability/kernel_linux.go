//go:build linux

package capability

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

func init() {
	platformDetector = func() Detector {
		return &LinuxDetector{}
	}
}

// LinuxDetector detects capabilities on Linux systems
type LinuxDetector struct{}

// Detect returns system capabilities on Linux
func (d *LinuxDetector) Detect() (*SystemInfo, error) {
	info := &SystemInfo{
		CPUCores: runtime.NumCPU(),
	}

	// 1. Get kernel version
	info.KernelVersion = d.getKernelVersion()

	// 2. Check for kernel WireGuard module
	info.HasKernelWG = d.checkKernelWG()

	// 3. Check for AES-NI hardware acceleration
	info.HasAESNI = d.checkAESNI()

	// 4. Check if running in a container
	info.IsContainer = d.checkContainer()

	// 5. Determine runtime mode
	if info.HasKernelWG && !info.IsContainer {
		info.RuntimeMode = ModeKernel
	} else {
		info.RuntimeMode = ModeUserspace
	}

	// 6. Warn if kernel version is suboptimal
	d.checkKernelVersionWarning(info.KernelVersion)

	return info, nil
}

// getKernelVersion returns the Linux kernel version
func (d *LinuxDetector) getKernelVersion() string {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return "unknown"
	}

	// Convert int8 array to string
	var buf []byte
	for _, c := range uname.Release {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}

// checkKernelWG checks if the kernel WireGuard module is available
func (d *LinuxDetector) checkKernelWG() bool {
	// Method 1: Try to check if wireguard module is loaded or available
	if _, err := os.Stat("/sys/module/wireguard"); err == nil {
		return true
	}

	// Method 2: Try to create a wireguard interface (requires root)
	if os.Geteuid() == 0 {
		cmd := exec.Command("ip", "link", "add", "wg-test-probe", "type", "wireguard")
		if err := cmd.Run(); err == nil {
			// Successfully created, clean up
			exec.Command("ip", "link", "del", "wg-test-probe").Run()
			return true
		}
	}

	// Method 3: Check kernel version (WireGuard built-in since 5.6)
	version := d.getKernelVersion()
	major, minor := parseKernelVersion(version)
	if major > 5 || (major == 5 && minor >= 6) {
		// Kernel 5.6+ has WireGuard built-in, but module might not be loaded
		// Try to load it
		if os.Geteuid() == 0 {
			exec.Command("modprobe", "wireguard").Run()
			if _, err := os.Stat("/sys/module/wireguard"); err == nil {
				return true
			}
		}
		// Even if we can't verify, assume it's available on 5.6+
		return true
	}

	return false
}

// parseKernelVersion extracts major.minor from kernel version string
func parseKernelVersion(version string) (major, minor int) {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		var m, n int
		if err := sscanf(parts[0], &m); err == nil {
			major = m
		}
		if err := sscanf(parts[1], &n); err == nil {
			minor = n
		}
	}
	return
}

// sscanf is a simple implementation for parsing integers from strings
func sscanf(s string, a *int) error {
	// Extract only digits from the string
	var digits []byte
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits = append(digits, byte(c))
		} else {
			break
		}
	}
	if len(digits) == 0 {
		return os.ErrInvalid
	}

	var result int
	for _, d := range digits {
		result = result*10 + int(d-'0')
	}
	*a = result
	return nil
}

// checkAESNI checks if the CPU supports AES-NI hardware acceleration
func (d *LinuxDetector) checkAESNI() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), " aes ")
}

// checkContainer checks if running inside a container
func (d *LinuxDetector) checkContainer() bool {
	// Check for Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check cgroup for container indicators
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "docker") ||
		strings.Contains(content, "kubepods") ||
		strings.Contains(content, "containerd") ||
		strings.Contains(content, "lxc")
}

// checkKernelVersionWarning logs a warning if kernel version is suboptimal
func (d *LinuxDetector) checkKernelVersionWarning(version string) {
	major, minor := parseKernelVersion(version)

	// WireGuard was merged into mainline kernel in 5.6
	// Kernel 5.10 is the first LTS with WireGuard
	if major < 5 || (major == 5 && minor < 6) {
		// Using stderr for warnings since we don't have a logger here
		os.Stderr.WriteString("⚠️  WARNING: Kernel version " + version + " detected\n")
		os.Stderr.WriteString("   WireGuard will run in userspace mode (30-50% performance penalty)\n")
		os.Stderr.WriteString("   Recommended: Upgrade to kernel 5.10+ for optimal performance\n")
		os.Stderr.WriteString("   - Ubuntu 20.04+: apt install linux-generic-hwe-20.04\n")
		os.Stderr.WriteString("   - CentOS/RHEL 8+: dnf install kernel-ml\n")
		os.Stderr.WriteString("   - Debian 11+: apt install linux-image-amd64\n\n")
	} else if major == 5 && minor < 10 {
		// Kernel 5.6-5.9: has WireGuard but not LTS
		os.Stderr.WriteString("ℹ️  INFO: Kernel " + version + " has WireGuard support\n")
		os.Stderr.WriteString("   Consider upgrading to 5.10+ (LTS) for better stability\n\n")
	}
}
