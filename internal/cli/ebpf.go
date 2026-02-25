package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	firewall_ebpf "aria/internal/agent/firewall"
)

var ebpfCmd = &cobra.Command{
	Use:   "ebpf",
	Short: "管理 eBPF 防火墙和 QoS 功能",
	Long:  `管理基于 eBPF 的防火墙和服务质量功能。`,
}

var ebpfAclCmd = &cobra.Command{
	Use:   "acl",
	Short: "管理 eBPF 访问控制列表",
	Long:  `管理基于 eBPF 的访问控制列表功能。`,
}

var ebpfQosCmd = &cobra.Command{
	Use:   "qos",
	Short: "管理 eBPF 服务质量(QoS)",
	Long:  `管理基于 eBPF 的服务质量功能。`,
}

var ebpfAclBlockCmd = &cobra.Command{
	Use:   "block-ip [IP]",
	Short: "屏蔽特定IP地址",
	Long:  `使用 eBPF XDP 程序屏蔽特定IP地址。`,
	Args:  cobra.ExactArgs(1),
	RunE:  runEbpfaclBlock,
}

var ebpfQosLimitCmd = &cobra.Command{
	Use:   "limit-ip [IP] --mbps [bandwidth]",
	Short: "限制特定IP的带宽",
	Long:  `使用 eBPF TC 程序限制特定IP的带宽。`,
	Args:  cobra.ExactArgs(1),
	RunE:  runEbpfqosLimit,
}

var ebpfQosPeerLimitCmd = &cobra.Command{
	Use:   "limit-peer [SRC_IP] [DST_IP] --mbps [bandwidth]",
	Short: "限制两个IP之间的带宽",
	Long:  `使用 eBPF TC 程序限制两个IP地址之间的带宽。`,
	Args:  cobra.ExactArgs(2),
	RunE:  runEbpfqosPeerLimit,
}

var ebpfQosPortLimitCmd = &cobra.Command{
	Use:   "limit-port [PORT] --mbps [bandwidth]",
	Short: "限制特定端口的带宽",
	Long:  `使用 eBPF TC 程序限制特定端口的带宽。`,
	Args:  cobra.ExactArgs(1),
	RunE:  runEbpfqosPortLimit,
}

var ebpfQosServiceLimitCmd = &cobra.Command{
	Use:   "limit-service [SRC_IP] [DST_IP] [SRC_PORT] [DST_PORT] --mbps [bandwidth] --protocol [protocol]",
	Short: "限制特定服务的带宽（五元组）",
	Long:  `使用五元组（源IP、目标IP、源端口、目标端口、协议）通过 eBPF TC 程序限制特定服务的带宽。`,
	Args:  cobra.ExactArgs(4),
	RunE:  runEbpfqosServiceLimit,
}

var ebpfQosPortForIPCmd = &cobra.Command{
	Use:   "limit-ip-port [IP] [PORT] --mbps [bandwidth] --direction [direction]",
	Short: "限制IP特定端口的带宽",
	Long:  `使用 eBPF TC 程序限制IP特定端口的带宽。`,
	Args:  cobra.ExactArgs(2),
	RunE:  runEbpfqosPortForIP,
}

var (
	qosMbps    int
	qosPort    int
	qosProto   int
	qosDir     string
)

func init() {
	// eBPF command flags
	ebpfQosLimitCmd.Flags().IntVar(&qosMbps, "mbps", 0, "带宽限制 (Mbps) (必选)")
	ebpfQosLimitCmd.MarkFlagRequired("mbps")

	ebpfQosPeerLimitCmd.Flags().IntVar(&qosMbps, "mbps", 0, "带宽限制 (Mbps) (必选)")
	ebpfQosPeerLimitCmd.MarkFlagRequired("mbps")

	ebpfQosPortLimitCmd.Flags().IntVar(&qosMbps, "mbps", 0, "带宽限制 (Mbps) (必选)")
	ebpfQosPortLimitCmd.MarkFlagRequired("mbps")

	ebpfQosServiceLimitCmd.Flags().IntVar(&qosMbps, "mbps", 0, "带宽限制 (Mbps) (必选)")
	ebpfQosServiceLimitCmd.MarkFlagRequired("mbps")
	ebpfQosServiceLimitCmd.Flags().IntVar(&qosProto, "protocol", 6, "协议 (默认 6 表示 TCP)")

	ebpfQosPortForIPCmd.Flags().IntVar(&qosMbps, "mbps", 0, "带宽限制 (Mbps) (必选)")
	ebpfQosPortForIPCmd.MarkFlagRequired("mbps")
	ebpfQosPortForIPCmd.Flags().StringVar(&qosDir, "direction", "both", "方向 (src=源, dst=目标, both=双向)")

	// Add subcommands
	ebpfAclCmd.AddCommand(ebpfAclBlockCmd)
	ebpfQosCmd.AddCommand(ebpfQosLimitCmd, ebpfQosPeerLimitCmd, ebpfQosPortLimitCmd, ebpfQosServiceLimitCmd, ebpfQosPortForIPCmd)
	ebpfCmd.AddCommand(ebpfAclCmd, ebpfQosCmd)

	rootCmd.AddCommand(ebpfCmd)
}

func runEbpfaclBlock(cmd *cobra.Command, args []string) error {
	ip := args[0]

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("创建 eBPF 适配器失败: %v", err)
	}
	defer adapter.Close()

	if err := adapter.BlockIP(ip); err != nil {
		return fmt.Errorf("屏蔽 IP %s 失败: %v", ip, err)
	}

	fmt.Printf("成功屏蔽 IP: %s\n", ip)
	return nil
}

func runEbpfqosLimit(cmd *cobra.Command, args []string) error {
	ip := args[0]

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("创建 eBPF 适配器失败: %v", err)
	}
	defer adapter.Close()

	if err := adapter.LimitIP(ip, qosMbps); err != nil {
		return fmt.Errorf("限制 IP %s 的带宽至 %d Mbps 失败: %v", ip, qosMbps, err)
	}

	fmt.Printf("成功限制 IP %s 的带宽至 %d Mbps\n", ip, qosMbps)
	return nil
}

func runEbpfqosPeerLimit(cmd *cobra.Command, args []string) error {
	srcIP := args[0]
	dstIP := args[1]

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("创建 eBPF 适配器失败: %v", err)
	}
	defer adapter.Close()

	if err := adapter.LimitPeerPair(srcIP, dstIP, qosMbps); err != nil {
		return fmt.Errorf("限制对等方 %s -> %s 的带宽至 %d Mbps 失败: %v", srcIP, dstIP, qosMbps, err)
	}

	fmt.Printf("成功限制对等方 %s -> %s 的带宽至 %d Mbps\n", srcIP, dstIP, qosMbps)
	return nil
}

func runEbpfqosPortLimit(cmd *cobra.Command, args []string) error {
	port := args[0]

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("创建 eBPF 适配器失败: %v", err)
	}
	defer adapter.Close()

	qosPort := 0
	fmt.Sscanf(port, "%d", &qosPort)

	if err := adapter.LimitPort(qosPort, qosMbps); err != nil {
		return fmt.Errorf("限制端口 %d 的带宽至 %d Mbps 失败: %v", qosPort, qosMbps, err)
	}

	fmt.Printf("成功限制端口 %d 的带宽至 %d Mbps\n", qosPort, qosMbps)
	return nil
}

func runEbpfqosServiceLimit(cmd *cobra.Command, args []string) error {
	srcIP := args[0]
	dstIP := args[1]
	srcPort := args[2]
	dstPort := args[3]

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("创建 eBPF 适配器失败: %v", err)
	}
	defer adapter.Close()

	qosSrcPort := 0
	qosDstPort := 0
	fmt.Sscanf(srcPort, "%d", &qosSrcPort)
	fmt.Sscanf(dstPort, "%d", &qosDstPort)

	if err := adapter.LimitService(srcIP, dstIP, qosSrcPort, qosDstPort, qosProto, qosMbps); err != nil {
		return fmt.Errorf("限制服务 %s:%d -> %s:%d 的带宽至 %d Mbps 失败: %v", srcIP, qosSrcPort, dstIP, qosDstPort, qosMbps, err)
	}

	fmt.Printf("成功限制服务 %s:%d -> %s:%d 的带宽至 %d Mbps (协议 %d)\n", srcIP, qosSrcPort, dstIP, qosDstPort, qosMbps, qosProto)
	return nil
}

func runEbpfqosPortForIP(cmd *cobra.Command, args []string) error {
	ip := args[0]
	port := args[1]

	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("创建 eBPF 适配器失败: %v", err)
	}
	defer adapter.Close()

	qosPort := 0
	fmt.Sscanf(port, "%d", &qosPort)

	if err := adapter.LimitPortForIP(ip, qosPort, qosMbps, qosDir); err != nil {
		return fmt.Errorf("限制IP %s 端口 %d (方向: %s) 的带宽至 %d Mbps 失败: %v", ip, qosPort, qosDir, qosMbps, err)
	}

	fmt.Printf("成功限制IP %s 端口 %d (方向: %s) 的带宽至 %d Mbps\n", ip, qosPort, qosDir, qosMbps)
	return nil
}

// Helper function to get eBPF statistics (for future use)
func getEbpfStats() error {
	adapter, err := firewall_ebpf.NewEBPFAdapter()
	if err != nil {
		return fmt.Errorf("failed to create eBPF adapter: %v", err)
	}
	defer adapter.Close()

	fmt.Println("eBPF statistics retrieval not yet implemented in this version")
	return nil
}