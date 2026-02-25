package datapath

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// AddECMPRoute adds an ECMP (Equal-Cost Multi-Path) route
// This function is idempotent: if route exists, it will be replaced
// Example: ip route replace 192.168.0.0/16 proto static \
//     nexthop dev aria0 weight 1 \
//     nexthop dev aria1 weight 1 \
//     nexthop dev aria2 weight 1 \
//     nexthop dev aria3 weight 1
func (r *LinuxRouteManager) AddECMPRoute(destination string, interfaces []string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("ECMP routes are only supported on Linux")
	}

	if len(interfaces) == 0 {
		return fmt.Errorf("no interfaces provided for ECMP route")
	}

	// Use "ip route replace" instead of "add" to make it idempotent
	args := []string{"route", "replace", destination, "proto", "static"}

	for _, iface := range interfaces {
		args = append(args, "nexthop", "dev", iface, "weight", "1")
	}

	cmd := exec.Command("ip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add ECMP route: %w, output: %s", err, output)
	}

	return nil
}

// RemoveECMPRoute removes an ECMP route
func (r *LinuxRouteManager) RemoveECMPRoute(destination string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("ECMP routes are only supported on Linux")
	}

	cmd := exec.Command("ip", "route", "del", destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore "not found" errors
		if strings.Contains(string(output), "No such process") ||
			strings.Contains(string(output), "not found") {
			return nil
		}
		return fmt.Errorf("failed to remove ECMP route: %w, output: %s", err, output)
	}

	return nil
}

// ECMPRoute represents an ECMP route
type ECMPRoute struct {
	Destination string
	Interfaces  []string
}

// ListECMPRoutes lists all ECMP routes (not implemented yet)
func (r *LinuxRouteManager) ListECMPRoutes() ([]*ECMPRoute, error) {
	// TODO: Parse `ip route show` output to find ECMP routes
	return nil, fmt.Errorf("not implemented")
}
