// Package vpn 实现 SD-WAN 隧道封装逻辑
package vpn

import "net"

// Tunnel 定义隧道接口
type Tunnel interface {
	// Open 建立隧道连接
	Open() error
	// Close 关闭隧道
	Close() error
	// Send 通过隧道发送数据
	Send(data []byte) error
	// Recv 从隧道接收数据
	Recv() ([]byte, error)
	// Status 获取隧道状态
	Status() TunnelStatus
}

// TunnelConfig 隧道配置
type TunnelConfig struct {
	Type       TunnelType
	LocalAddr  net.IP
	RemoteAddr net.IP
	LocalPort  int
	RemotePort int
	PrivateKey []byte
	PublicKey  []byte
	MTU        int
}

// TunnelType 隧道类型
type TunnelType int

const (
	// TunnelWireGuard WireGuard 隧道
	TunnelWireGuard TunnelType = iota
	// TunnelIPSec IPSec 隧道
	TunnelIPSec
	// TunnelGRE GRE 隧道
	TunnelGRE
	// TunnelVXLAN VXLAN 隧道
	TunnelVXLAN
)

// TunnelStatus 隧道状态
type TunnelStatus int

const (
	// StatusDisconnected 未连接
	StatusDisconnected TunnelStatus = iota
	// StatusConnecting 连接中
	StatusConnecting
	// StatusConnected 已连接
	StatusConnected
	// StatusError 错误状态
	StatusError
)
