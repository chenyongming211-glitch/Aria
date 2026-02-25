package tunnel

import (
	"testing"

	"github.com/vishvananda/netlink"
)

func TestCreateWireGuardInterface(t *testing.T) {
	// 创建一个名为 wg_test 的 WireGuard 接口
	link := &netlink.Wireguard{
		LinkAttrs: netlink.LinkAttrs{
			Name: "wg_test",
		},
	}

	err := netlink.LinkAdd(link)
	if err != nil {
		t.Fatalf("Failed to create WireGuard interface: %v", err)
	}
	t.Logf("Successfully created interface: %s", link.Name)

	// 解析地址
	addr, err := netlink.ParseAddr("10.0.0.1/24")
	if err != nil {
		t.Fatalf("Failed to parse address: %v", err)
	}

	// 将地址添加到接口
	err = netlink.AddrAdd(link, addr)
	if err != nil {
		t.Fatalf("Failed to add address: %v", err)
	}
	t.Logf("Successfully added address: %s", addr.IP)

	// 将接口 UP 起来
	err = netlink.LinkSetUp(link)
	if err != nil {
		t.Fatalf("Failed to set interface up: %v", err)
	}
	t.Logf("Successfully brought up interface: %s", link.Name)

	t.Log("Test completed successfully!")
	t.Log("Run 'ip addr' to verify wg_test interface exists")
}
