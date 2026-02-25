package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type CloudInfo struct {
	Provider  string `json:"provider"`
	Region    string `json:"region"`
	VPCID     string `json:"vpc_id"`
	PrivateIP string `json:"private_ip"`
	PublicIP  string `json:"public_ip"`
	MachineID string `json:"machine_id"`
}

func DetectCloudInfo() (*CloudInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 依次尝试各个云厂商的 Metadata URL
	providers := []struct {
		name string
		url  string
	}{
		{"AWS", "http://169.254.169.254/latest/dynamic/instance-identity/document"},
		{"Aliyun", "http://100.100.100.200/latest/meta-data/"},
		{"Tencent", "http://169.254.0.1/latest/meta-data/"},
	}

	for _, p := range providers {
		info, err := fetchMetadata(ctx, p.name, p.url)
		if err == nil {
			return info, nil
		}
	}

	// 返回 Unknown/IDC
	return &CloudInfo{
		Provider:  "Unknown",
		Region:    "",
		VPCID:     "",
		PrivateIP: getLocalIP(),
		PublicIP:  getPublicIP(),
		MachineID: getMachineID(),
	}, nil
}

func getMachineID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("unknown-%d", time.Now().UnixNano())
	}
	return hostname
}

func fetchMetadata(ctx context.Context, provider, url string) (*CloudInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 根据云厂商解析
	switch provider {
	case "AWS":
		return parseAWSMetadata(body)
	case "Aliyun":
		return parseAliyunMetadata(body)
	case "Tencent":
		return parseTencentMetadata(body)
	}

	return nil, fmt.Errorf("unknown provider")
}

func parseAWSMetadata(body []byte) (*CloudInfo, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	info := &CloudInfo{
		Provider:  "AWS",
		MachineID: getMachineID(),
	}

	if region, ok := doc["region"].(string); ok {
		info.Region = region
	}

	// 尝试获取 VPC ID
	if interfaces, ok := doc["networkInterfaces"].([]interface{}); ok {
		if len(interfaces) > 0 {
			if mac0, ok := interfaces[0].(map[string]interface{}); ok {
				if vpcID, ok := mac0["vpcId"].(string); ok {
					info.VPCID = vpcID
				}
				if ips, ok := mac0["localIPv4Addresses"].([]interface{}); ok {
					if len(ips) > 0 {
						info.PrivateIP = ips[0].(string)
					}
				}
			}
		}
	}

	return info, nil
}

func parseAliyunMetadata(body []byte) (*CloudInfo, error) {
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")

	info := &CloudInfo{
		Provider:  "Aliyun",
		MachineID: getMachineID(),
	}

	for i := 0; i < len(lines)-1; i++ {
		line := strings.TrimSpace(lines[i])
		nextLine := strings.TrimSpace(lines[i+1])

		switch line {
		case "region-id":
			info.Region = nextLine
			i++ // 跳过下一行
		case "vpc-id":
			info.VPCID = nextLine
			i++
		case "inner-ip-address":
			info.PrivateIP = nextLine
			i++
		case "eip-address":
			info.PublicIP = nextLine
			i++
		}
	}

	return info, nil
}

func parseTencentMetadata(body []byte) (*CloudInfo, error) {
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")

	info := &CloudInfo{
		Provider:  "Tencent",
		MachineID: getMachineID(),
	}

	for i := 0; i < len(lines)-1; i++ {
		line := strings.TrimSpace(lines[i])
		nextLine := strings.TrimSpace(lines[i+1])

		switch line {
		case "placement/region":
			info.Region = nextLine
			i++
		case "network/interfaces/macs/0/vpc-id":
			info.VPCID = nextLine
			i++
		case "network/interfaces/macs/0/local-ipv4s":
			// 格式可能是 "ip1\nip2"
			ips := strings.Split(nextLine, "\n")
			if len(ips) > 0 {
				info.PrivateIP = ips[0]
			}
			i++
		}
	}

	return info, nil
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func getPublicIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://api.ipify.org", nil)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body))
}
