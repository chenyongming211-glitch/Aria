# gRPC API 测试指南

## 前置条件

1. 安装 grpcurl
   ```bash
   # macOS
   brew install grpcurl
   
   # Linux
   go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
   ```

2. Controller 已启动并监听 :50051

## 测试步骤

### 1. 列出所有服务

```bash
grpcurl -plaintext localhost:50051 list
```

预期输出：
```
aria.agent.AgentService
aria.agent.ControllerService
```

### 2. 查看服务方法

```bash
grpcurl -plaintext localhost:50051 list aria.agent.ControllerService
```

预期输出：
```
Register
Sync
```

### 3. 测试 Register API

```bash
grpcurl -plaintext -d '{
  "public_key": "test123456789",
  "endpoint": ":51820",
  "public_ip": "1.2.3.4",
  "hostname": "test-node",
  "registered_at": '$(date +%s)',
  "token": "YOUR_TOKEN_HERE",
  "region": "test",
  "runtime_mode": "kernel",
  "kernel_version": "5.15.0",
  "has_aesni": true
}' localhost:50051 aria.agent.ControllerService/Register
```

预期输出：
```json
{
  "assignedIp": "100.64.0.X",
  "metricsPushGateway": "http://aria-victoria-metrics:8428/api/v1/import/prometheus"
}
```

### 4. 测试 Sync API

```bash
grpcurl -plaintext -d '{
  "public_key": "test123456789"
}' localhost:50051 aria.agent.ControllerService/Sync
```

预期输出：
```json
{
  "peers": [
    {
      "publicKey": "...",
      "endpoint": "...",
      "assignedIp": "...",
      ...
    }
  ],
  "assignedIp": "100.64.0.X",
  "lastUpdate": 1234567890,
  "aclRules": [],
  "metricsPushGateway": "..."
}
```

### 5. 通过 Nginx 测试（如果配置了 Nginx）

```bash
# 通过 Nginx (需要 TLS)
grpcurl -insecure -d '{
  "public_key": "test123456789"
}' your-server.com:443 aria.agent.ControllerService/Sync
```

## 错误排查

### 问题 1：Connection refused

```bash
# 检查 Controller 是否在运行
docker ps | grep controller

# 检查端口是否监听
netstat -tlnp | grep 50051

# 查看 Controller 日志
docker logs aria_controller
```

### 问题 2：Permission denied

```bash
# 检查 gRPC 端口是否暴露
docker exec aria_controller netstat -tlnp | grep 50051

# 如果在容器内，需要确保端口映射正确
```

### 问题 3：TLS 错误

```bash
# 使用 -plaintext 跳过 TLS（仅测试用）
grpcurl -plaintext localhost:50051 list

# 或使用 -insecure 跳过证书验证
grpcurl -insecure localhost:50051 list
```

## 自动化测试脚本

创建 `test-grpc.sh`：

```bash
#!/bin/bash
set -e

CONTROLLER_HOST="${1:-localhost:50051}"
PUBLIC_KEY="test-$(date +%s)"

echo "Testing gRPC Controller at $CONTROLLER_HOST"
echo "============================================"

echo ""
echo "1. Listing services..."
grpcurl -plaintext $CONTROLLER_HOST list

echo ""
echo "2. Testing Register..."
grpcurl -plaintext -d "{
  \"public_key\": \"$PUBLIC_KEY\",
  \"endpoint\": \":51820\",
  \"public_ip\": \"1.2.3.4\",
  \"hostname\": \"test-node\",
  \"registered_at\": $(date +%s),
  \"token\": \"\",
  \"region\": \"test\"
}" $CONTROLLER_HOST aria.agent.ControllerService/Register

echo ""
echo "3. Testing Sync..."
grpcurl -plaintext -d "{
  \"public_key\": \"$PUBLIC_KEY\"
}" $CONTROLLER_HOST aria.agent.ControllerService/Sync

echo ""
echo "✅ All tests passed!"
```

运行：
```bash
chmod +x test-grpc.sh
./test-grpc.sh localhost:50051
```

## 性能测试

使用 ghz 进行负载测试：

```bash
# 安装 ghz
go install github.com/bojand/ghz/cmd/ghz@latest

# 测试 Sync API 性能
ghz --insecure \
  --proto pkg/grpc/agentpb/aria-agent.proto \
  --call aria.agent.ControllerService/Sync \
  -d '{"public_key":"test123"}' \
  -n 1000 \
  -c 100 \
  localhost:50051
```

## 集成测试

创建 Go 测试文件 `internal/controller/grpc/server_test.go`：

```go
package grpc_test

import (
	"context"
	"testing"
	"time"

	"aria/pkg/grpc/agentpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestControllerGRPC(t *testing.T) {
	// 连接到 gRPC 服务器
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := agentpb.NewControllerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 测试 Register
	t.Run("Register", func(t *testing.T) {
		resp, err := client.Register(ctx, &agentpb.RegisterRequest{
			PublicKey:    "test-key",
			Endpoint:     ":51820",
			PublicIp:     "1.2.3.4",
			Hostname:     "test-node",
			RegisteredAt: time.Now().Unix(),
			Region:       "test",
		})
		if err != nil {
			t.Errorf("Register failed: %v", err)
		}
		if resp.AssignedIp == "" {
			t.Error("Expected assigned IP")
		}
		t.Logf("Assigned IP: %s", resp.AssignedIp)
	})

	// 测试 Sync
	t.Run("Sync", func(t *testing.T) {
		resp, err := client.Sync(ctx, &agentpb.SyncRequest{
			PublicKey: "test-key",
		})
		if err != nil {
			t.Errorf("Sync failed: %v", err)
		}
		if resp.AssignedIp == "" {
			t.Error("Expected assigned IP")
		}
		t.Logf("Got %d peers", len(resp.Peers))
	})
}
```

运行测试：
```bash
go test -v ./internal/controller/grpc/...
```
