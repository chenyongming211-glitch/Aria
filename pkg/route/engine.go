package route

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const (
	VPNTable    = 100
	DirectTable = 200
	VPNPriority = 100
)

type Engine struct {
	interfaceName string
}

func NewEngine(interfaceName string) *Engine {
	return &Engine{
		interfaceName: interfaceName,
	}
}

func (e *Engine) Init() error {
	log.Printf("Route Engine: initializing with interface %s", e.interfaceName)

	if err := e.ensureTables(); err != nil {
		return fmt.Errorf("failed to ensure tables: %v", err)
	}

	if err := e.ensureRules(); err != nil {
		return fmt.Errorf("failed to ensure rules: %v", err)
	}

	log.Printf("Route Engine: initialized successfully")
	return nil
}

func (e *Engine) ensureTables() error {
	tables := []int{VPNTable, DirectTable}
	for _, table := range tables {
		cmd := exec.Command("ip", "route", "show", "table", strconv.Itoa(table))
		if err := cmd.Run(); err != nil {
			log.Printf("Route Engine: table %d not exists, creating", table)
		}
	}
	return nil
}

func (e *Engine) ensureRules() error {
	rules := []struct {
		table    int
		priority int
	}{
		{VPNTable, VPNPriority},
		{DirectTable, VPNPriority + 1},
	}

	for _, r := range rules {
		cmd := exec.Command("ip", "rule", "show", "priority", strconv.Itoa(r.priority))
		if err := cmd.Run(); err != nil {
			log.Printf("Route Engine: rule for table %d not exists, creating", r.table)
			addCmd := exec.Command("ip", "rule", "add", "priority", strconv.Itoa(r.priority), "table", strconv.Itoa(r.table))
			if err := addCmd.Run(); err != nil {
				log.Printf("Route Engine: failed to create rule: %v", err)
			}
		}
	}
	return nil
}

func (e *Engine) AddVPNRoute(dstIP string) error {
	_, _, err := net.ParseCIDR(dstIP)
	if err != nil {
		return fmt.Errorf("invalid IP: %v", err)
	}

	cmd := exec.Command("ip", "route", "add", dstIP, "dev", e.interfaceName, "table", strconv.Itoa(VPNTable))
	if err := cmd.Run(); err != nil {
		if !strings.Contains(err.Error(), "File exists") {
			return fmt.Errorf("failed to add VPN route: %v", err)
		}
	}

	log.Printf("Route Engine: added VPN route %s via %s", dstIP, e.interfaceName)
	return nil
}

func (e *Engine) AddDirectRoute(dstIP, dev string) error {
	cmd := exec.Command("ip", "route", "add", dstIP, "dev", dev, "table", strconv.Itoa(DirectTable))
	if err := cmd.Run(); err != nil {
		if !strings.Contains(err.Error(), "File exists") {
			return fmt.Errorf("failed to add direct route: %v", err)
		}
	}

	log.Printf("Route Engine: added direct route %s via %s", dstIP, dev)
	return nil
}

func (e *Engine) RemoveVPNRoute(dstIP string) error {
	cmd := exec.Command("ip", "route", "del", dstIP, "table", strconv.Itoa(VPNTable))
	if err := cmd.Run(); err != nil {
		if !strings.Contains(err.Error(), "No such process") {
			return fmt.Errorf("failed to remove VPN route: %v", err)
		}
	}

	log.Printf("Route Engine: removed VPN route %s", dstIP)
	return nil
}

func (e *Engine) Cleanup() error {
	rules := []int{VPNPriority, VPNPriority + 1}
	for _, priority := range rules {
		cmd := exec.Command("ip", "rule", "del", "priority", strconv.Itoa(priority))
		cmd.Run()
	}

	log.Printf("Route Engine: cleaned up")
	return nil
}

func (e *Engine) GetRoutes() ([]netlink.Route, error) {
	link, err := netlink.LinkByName(e.interfaceName)
	if err != nil {
		return nil, err
	}

	routes, err := netlink.RouteList(link, unix.AF_INET)
	if err != nil {
		return nil, err
	}

	return routes, nil
}
