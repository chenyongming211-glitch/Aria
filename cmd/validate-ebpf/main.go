package main

import (
	"fmt"

	"aria/internal/eBPF"
)

func main() {
	fmt.Println("Aria v3.0 eBPF Firewall & QoS System - Type Matching Validation")
	fmt.Println("=================================================================")

	// 运行结构体兼容性验证
	eBPF.RunValidation()

	fmt.Println("\nType matching validation completed successfully!")
	fmt.Println("C structures and Go structures are compatible for eBPF operations.")
}