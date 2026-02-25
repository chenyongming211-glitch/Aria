package capability

import "runtime"

// RuntimeMode represents the WireGuard operation mode
type RuntimeMode string

const (
	// ModeKernel uses the Linux kernel WireGuard module (best performance)
	ModeKernel RuntimeMode = "kernel"
	// ModeUserspace uses wireguard-go in userspace (compatible with old kernels)
	ModeUserspace RuntimeMode = "userspace"
)

// SystemInfo contains detected system capabilities
type SystemInfo struct {
	KernelVersion string      `json:"kernel_version"`
	HasKernelWG   bool        `json:"has_kernel_wg"`
	HasAESNI      bool        `json:"has_aesni"`
	RuntimeMode   RuntimeMode `json:"runtime_mode"`
	CPUCores      int         `json:"cpu_cores"`
	IsContainer   bool        `json:"is_container"`
}

// Detector interface for system capability detection
type Detector interface {
	Detect() (*SystemInfo, error)
}

// platformDetector is set by platform-specific init() functions
var platformDetector func() Detector

// NewDetector creates a platform-specific detector
func NewDetector() Detector {
	if platformDetector != nil {
		return platformDetector()
	}
	// Fallback to generic detector
	return &GenericDetector{}
}

// GenericDetector is a fallback detector for unsupported platforms
type GenericDetector struct{}

// Detect returns basic system info for unsupported platforms
func (d *GenericDetector) Detect() (*SystemInfo, error) {
	return &SystemInfo{
		KernelVersion: runtime.GOOS,
		HasKernelWG:   false,
		HasAESNI:      false,
		RuntimeMode:   ModeUserspace,
		CPUCores:      runtime.NumCPU(),
		IsContainer:   false,
	}, nil
}
