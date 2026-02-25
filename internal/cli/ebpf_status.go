package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var ebpfStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check eBPF support and status",
	Long:  `Check if the system supports eBPF and the status of loaded programs.`,
	RunE:  runEbpfStatus,
}

func init() {
	ebpfCmd.AddCommand(ebpfStatusCmd)
}

func runEbpfStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Checking eBPF support and status...")
	fmt.Println()

	// Check kernel version
	kernelOK, kernelVersion := checkKernelVersion()
	fmt.Printf("Kernel version: %s - %s\n", kernelVersion, boolToStatus(kernelOK))

	// Check eBPF tools
	bpftoolOK := checkBpftool()
	fmt.Printf("bpftool available: %s\n", boolToStatus(bpftoolOK))

	// Check clang/llvm
	clangOK := checkClang()
	fmt.Printf("clang available: %s\n", boolToStatus(clangOK))

	// Check if system is Linux (eBPF requirement)
	isLinux := runtime.GOOS == "linux"
	fmt.Printf("Linux OS: %s\n", boolToStatus(isLinux))

	fmt.Println()

	// Show loaded eBPF programs if possible
	if bpftoolOK {
		fmt.Println("Loaded eBPF programs:")
		showEbpfPrograms()
		fmt.Println()

		fmt.Println("Loaded eBPF maps:")
		showEbpfMaps()
	}

	fmt.Println()

	if kernelOK && bpftoolOK && clangOK && isLinux {
		fmt.Println("✓ System is ready for eBPF development and execution")
	} else {
		fmt.Println("✗ System may not fully support eBPF")
		if !isLinux {
			fmt.Println("  - eBPF is primarily supported on Linux")
		}
		if !kernelOK {
			fmt.Println("  - eBPF requires Linux kernel 5.4 or newer")
		}
		if !bpftoolOK {
			fmt.Println("  - Install bpftool for managing eBPF programs")
		}
		if !clangOK {
			fmt.Println("  - Install clang/llvm for compiling eBPF programs")
		}
	}

	return nil
}

func checkKernelVersion() (bool, string) {
	if runtime.GOOS != "linux" {
		return false, "Not Linux"
	}

	// Execute 'uname -r' to get kernel version
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return false, "Unknown"
	}

	versionStr := strings.TrimSpace(string(output))

	// Parse major.minor version
	parts := strings.Split(versionStr, ".")
	if len(parts) < 2 {
		return false, versionStr
	}

	major := parseInt(parts[0])
	minor := parseInt(parts[1])

	// eBPF generally available since kernel 4.9, but for full functionality we suggest 5.4+
	if major > 5 || (major == 5 && minor >= 4) {
		return true, versionStr
	}

	return false, versionStr
}

func checkBpftool() bool {
	cmd := exec.Command("bpftool", "--version")
	err := cmd.Run()
	return err == nil
}

func checkClang() bool {
	cmd := exec.Command("clang", "--version")
	err := cmd.Run()
	return err == nil
}

func showEbpfPrograms() {
	cmd := exec.Command("sudo", "bpftool", "prog", "list")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("  Could not list programs:", err)
		return
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 1 {
		for i, line := range lines {
			if i < len(lines)-1 { // Skip last empty line
				fmt.Printf("  %s\n", line)
			}
		}
	} else {
		fmt.Println("  No eBPF programs loaded")
	}
}

func showEbpfMaps() {
	cmd := exec.Command("sudo", "bpftool", "map", "list")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("  Could not list maps:", err)
		return
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 1 {
		for i, line := range lines {
			if i < len(lines)-1 { // Skip last empty line
				fmt.Printf("  %s\n", line)
			}
		}
	} else {
		fmt.Println("  No eBPF maps loaded")
	}
}

func parseInt(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func boolToStatus(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}