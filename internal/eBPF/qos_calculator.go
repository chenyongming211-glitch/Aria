package eBPF

import (
	"math"
)

const (
	// MTU 最小突发量（以太网标准）
	MTU = 1500
	// DefaultBurstWindow 默认突发时间窗口（20ms）
	DefaultBurstWindow = 0.02 // 20 ms
)

// CalculateBucketParams 将用户输入的带宽（Mbps）转换为 eBPF 令牌桶参数
// mbps：带宽值，例如 100.0 表示 100 Mbps
// ruleID：规则 ID，用于标识该 QoS 规则
// 返回值：rate（字节/秒），burst（字节），可直接写入 bucket_state
func CalculateBucketParams(mbps float64, ruleID uint32) (rate uint64, burst uint64) {
	// 1. 速率转换：Mbps → Bytes/s
	// 注意：网络带宽采用十进制 1 Mbps = 1,000,000 bits/s
	rate = uint64((mbps * 1_000_000) / 8) // 先乘后除避免精度损失

	// 2. 突发容量计算：rate * 时间窗口（秒）
	calculatedBurst := float64(rate) * DefaultBurstWindow

	// 3. 边界修正：至少一个 MTU，且防止 uint64 溢出
	if calculatedBurst < MTU {
		burst = MTU
	} else {
		// 理论上 calculatedBurst 远小于 MaxUint64，但做保护
		if calculatedBurst > float64(math.MaxUint64) {
			burst = math.MaxUint64
		} else {
			burst = uint64(calculatedBurst)
		}
	}

	return rate, burst
}

// 如果需要直接处理整数 Mbps（如 100），可保留此包装函数
func CalculateBucketParamsInt(mbps uint64, ruleID uint32) (rate uint64, burst uint64) {
	return CalculateBucketParams(float64(mbps), ruleID)
}