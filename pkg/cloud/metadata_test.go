package cloud

import (
	"fmt"
	"testing"
)

func TestDetectCloudInfo(t *testing.T) {
	info, err := DetectCloudInfo()
	if err != nil {
		t.Fatalf("Failed to detect cloud info: %v", err)
	}

	fmt.Printf("Detected: %s\n", info.Provider)
	if info.Provider != "Unknown" {
		fmt.Printf("Region: %s\n", info.Region)
	}
	fmt.Printf("VPC ID: %s\n", info.VPCID)
	fmt.Printf("Private IP: %s\n", info.PrivateIP)
	fmt.Printf("Public IP: %s\n", info.PublicIP)
}
