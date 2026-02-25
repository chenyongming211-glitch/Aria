package datapath

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const (
	// VPNTable is the routing table ID for VPN routes.
	VPNTable = 100

	// DirectTable is the routing table ID for direct routes.
	DirectTable = 200

	// VPNPriority is the IP rule priority for VPN routes.
	VPNPriority = 100

	// MainTable is the main routing table ID.
	MainTable = 254
)

// LinuxRouteManager implements RouteManager for Linux systems.
type LinuxRouteManager struct {
	interfaceName string
}

// NewLinuxRouteManager creates a new Linux route manager.
func NewLinuxRouteManager(interfaceName string) *LinuxRouteManager {
	return &LinuxRouteManager{
		interfaceName: interfaceName,
	}
}

// Init initializes the routing engine.
func (r *LinuxRouteManager) Init() error {
	log.Printf("Route Manager: initializing with interface %s", r.interfaceName)

	if err := r.ensureTables(); err != nil {
		return fmt.Errorf("failed to ensure tables: %w", err)
	}

	if err := r.ensureRules(); err != nil {
		return fmt.Errorf("failed to ensure rules: %w", err)
	}

	log.Printf("Route Manager: initialized successfully")
	return nil
}

// Cleanup removes all managed routes and rules.
func (r *LinuxRouteManager) Cleanup() error {
	rules := []int{VPNPriority, VPNPriority + 1}
	for _, priority := range rules {
		cmd := exec.Command("ip", "rule", "del", "priority", strconv.Itoa(priority))
		cmd.Run() // Ignore errors
	}

	log.Printf("Route Manager: cleaned up")
	return nil
}

// AddRoute adds a route to the routing table.
// This function is idempotent: if route exists, it will be replaced
func (r *LinuxRouteManager) AddRoute(route *RouteEntry) error {
	if route == nil {
		return fmt.Errorf("route entry is required")
	}

	ifaceName := route.Interface
	if ifaceName == "" {
		ifaceName = r.interfaceName
	}

	table := route.Table
	if table == 0 {
		table = MainTable
	}

	// Use "replace" to make it idempotent
	args := []string{"route", "replace", route.Destination, "dev", ifaceName}

	if route.Gateway != "" {
		args = append(args, "via", route.Gateway)
	}

	if route.Metric > 0 {
		args = append(args, "metric", strconv.Itoa(route.Metric))
	}

	if table != MainTable {
		args = append(args, "table", strconv.Itoa(table))
	}

	cmd := exec.Command("ip", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, output)
	}

	log.Printf("Route Manager: added route %s via %s", route.Destination, ifaceName)
	return nil
}

// RemoveRoute removes a route by destination.
func (r *LinuxRouteManager) RemoveRoute(destination string) error {
	cmd := exec.Command("ip", "route", "del", destination, "dev", r.interfaceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "No such process") {
			return ErrRouteNotFound
		}
		return fmt.Errorf("failed to remove route: %w, output: %s", err, outputStr)
	}

	log.Printf("Route Manager: removed route %s", destination)
	return nil
}

// ListRoutes returns all routes on the managed interface.
func (r *LinuxRouteManager) ListRoutes() ([]*RouteEntry, error) {
	link, err := netlink.LinkByName(r.interfaceName)
	if err != nil {
		return nil, ErrInterfaceNotFound
	}

	routes, err := netlink.RouteList(link, unix.AF_INET)
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}

	entries := make([]*RouteEntry, 0, len(routes))
	for _, route := range routes {
		entry := &RouteEntry{
			Interface: r.interfaceName,
			Table:     route.Table,
			Metric:    route.Priority,
		}

		if route.Dst != nil {
			entry.Destination = route.Dst.String()
		}

		if route.Gw != nil {
			entry.Gateway = route.Gw.String()
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// AddVPNRoute adds a route to the VPN routing table.
func (r *LinuxRouteManager) AddVPNRoute(destination string) error {
	cmd := exec.Command("ip", "route", "add", destination, "dev", r.interfaceName, "table", strconv.Itoa(VPNTable))
	if output, err := cmd.CombinedOutput(); err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "File exists") {
			return nil // Route already exists, not an error
		}
		return fmt.Errorf("failed to add VPN route: %w, output: %s", err, outputStr)
	}

	log.Printf("Route Manager: added VPN route %s via %s", destination, r.interfaceName)
	return nil
}

// RemoveVPNRoute removes a route from the VPN routing table.
func (r *LinuxRouteManager) RemoveVPNRoute(destination string) error {
	cmd := exec.Command("ip", "route", "del", destination, "table", strconv.Itoa(VPNTable))
	if output, err := cmd.CombinedOutput(); err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "No such process") {
			return nil // Route doesn't exist, not an error
		}
		return fmt.Errorf("failed to remove VPN route: %w, output: %s", err, outputStr)
	}

	log.Printf("Route Manager: removed VPN route %s", destination)
	return nil
}

// ensureTables ensures routing tables exist.
func (r *LinuxRouteManager) ensureTables() error {
	tables := []int{VPNTable, DirectTable}
	for _, table := range tables {
		cmd := exec.Command("ip", "route", "show", "table", strconv.Itoa(table))
		if err := cmd.Run(); err != nil {
			log.Printf("Route Manager: table %d will be created on first use", table)
		}
	}
	return nil
}

// ensureRules ensures IP rules exist.
func (r *LinuxRouteManager) ensureRules() error {
	rules := []struct {
		table    int
		priority int
	}{
		{VPNTable, VPNPriority},
		{DirectTable, VPNPriority + 1},
	}

	for _, rule := range rules {
		// Check if rule exists
		cmd := exec.Command("ip", "rule", "show", "priority", strconv.Itoa(rule.priority))
		output, _ := cmd.Output()
		if strings.Contains(string(output), strconv.Itoa(rule.priority)) {
			continue // Rule already exists
		}

		// Create rule
		addCmd := exec.Command("ip", "rule", "add", "priority", strconv.Itoa(rule.priority), "table", strconv.Itoa(rule.table))
		if output, err := addCmd.CombinedOutput(); err != nil {
			log.Printf("Route Manager: failed to create rule: %v, output: %s", err, output)
		}
	}
	return nil
}

// AddDirectRoute adds a route to the direct routing table.
func (r *LinuxRouteManager) AddDirectRoute(destination, dev string) error {
	cmd := exec.Command("ip", "route", "add", destination, "dev", dev, "table", strconv.Itoa(DirectTable))
	if output, err := cmd.CombinedOutput(); err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "File exists") {
			return nil // Route already exists
		}
		return fmt.Errorf("failed to add direct route: %w, output: %s", err, outputStr)
	}

	log.Printf("Route Manager: added direct route %s via %s", destination, dev)
	return nil
}
