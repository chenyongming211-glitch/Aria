# Nginx gRPC 配置指南

## 方案 1：直接暴露 gRPC 端口（推荐用于测试）

修改服务器上的 Nginx 配置（`/root/aria-controller/nginx/default.conf`）：

```nginx
# 在 server 块之前添加 stream 块
stream {
    upstream grpc_controller {
        server aria_controller:50051;
    }
    
    server {
        listen 50051;
        proxy_pass grpc_controller;
    }
}

# 现有的 HTTP/HTTPS 配置保持不变
server {
    listen 80;
    # ...
}
```

**注意：** `stream` 块必须放在 `http` 块之外，在 `nginx.conf` 的主上下文中。

## 方案 2：通过 HTTP/2 代理 gRPC（推荐用于生产）

在现有的 HTTPS server 块中添加：

```nginx
server {
    listen 443 ssl http2;
    
    # 现有配置...
    
    # gRPC 代理（需要 HTTP/2）
    location /grpc/ {
        grpc_pass grpc://aria_controller:50051;
        grpc_set_header X-Real-IP $remote_addr;
    }
}
```

客户端调用时需要使用路径前缀：
```bash
grpcurl -insecure your-server.com:443 /grpc/aria.agent.ControllerService/Sync
```

## 方案 3：独立的 gRPC 入口（推荐用于生产）

创建新的 server 块专门处理 gRPC：

```nginx
server {
    listen 8443 ssl http2;
    server_name _;
    
    ssl_certificate /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;
    
    location / {
        grpc_pass grpc://aria_controller:50051;
        grpc_set_header X-Real-IP $remote_addr;
        grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## 部署步骤

### 1. 更新 Nginx 配置文件

```bash
# SSH 到服务器
ssh root@112.124.8.241

# 备份现有配置
cp /root/aria-controller/nginx/default.conf /root/aria-controller/nginx/default.conf.bak

# 编辑配置（选择上面的一个方案）
nano /root/aria-controller/nginx/default.conf
```

### 2. 测试 Nginx 配置

```bash
# 进入 aria_web 容器
docker exec -it aria_web sh

# 测试配置
nginx -t

# 如果成功，退出容器
exit
```

### 3. 重启 Nginx

```bash
# 方法 1：重启容器
docker restart aria_web

# 方法 2：在容器内重载配置
docker exec aria_web nginx -s reload
```

### 4. 验证端口监听

```bash
# 检查端口
netstat -tlnp | grep 50051

# 或
ss -tlnp | grep 50051
```

## 测试连接

### 本地测试（在服务器上）

```bash
# 直接连接 Controller（跳过 Nginx）
docker exec -it aria_controller sh
apk add grpcurl  # 如果没有安装
grpcurl -plaintext localhost:50051 list

# 通过 Nginx 连接（如果配置了）
grpcurl -plaintext localhost:50051 list
```

### 远程测试（从开发机）

```bash
# 使用测试脚本
./scripts/test-grpc.sh 112.124.8.241:50051

# 或手动测试
grpcurl -plaintext 112.124.8.241:50051 list
```

## 故障排查

### 问题 1：Connection refused

```bash
# 检查 Controller 是否运行
docker ps | grep aria_controller

# 检查 Controller 是否监听 50051
docker exec aria_controller netstat -tlnp | grep 50051

# 查看 Controller 日志
docker logs aria_controller --tail 50
```

### 问题 2：Nginx 配置错误

```bash
# 查看 Nginx 错误日志
docker logs aria_web --tail 50

# 检查 Nginx 配置语法
docker exec aria_web nginx -t
```

### 问题 3：防火墙阻止

```bash
# 检查防火墙规则
ufw status
iptables -L -n | grep 50051

# 开放端口（如果需要）
ufw allow 50051/tcp
```

## 安全建议

1. **生产环境使用 TLS**
   ```bash
   grpcurl -insecure your-server.com:8443 list
   ```

2. **限制访问来源**
   ```nginx
   # 在 server 块中添加
   allow 10.0.0.0/8;
   deny all;
   ```

3. **使用认证**
   ```bash
   # 在 gRPC 调用中添加认证 token
   grpcurl -H "Authorization: Bearer YOUR_TOKEN" ...
   ```

## 完整配置示例

### /root/aria-controller/nginx/default.conf

```nginx
# HTTP -> HTTPS 重定向
server {
    listen 80;
    server_name _;
    return 301 https://$host$request_uri;
}

# 上游服务定义
upstream aria_api {
    server aria_controller:8080;
    keepalive 32;
}

# gRPC 上游（用于方案 2）
upstream grpc_controller {
    server aria_controller:50051;
}

# HTTPS Server (REST API + Web UI)
server {
    listen 443 ssl http2;
    server_name _;

    ssl_certificate /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;

    # 现有的 location 配置...
    
    # gRPC 代理（方案 2）
    location /grpc/ {
        grpc_pass grpc://grpc_controller;
        grpc_set_header X-Real-IP $remote_addr;
    }
}

# gRPC 专用入口（方案 3）
server {
    listen 8443 ssl http2;
    server_name _;

    ssl_certificate /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;

    location / {
        grpc_pass grpc://grpc_controller;
    }
}
```

## 下一步

1. ✅ 更新 Nginx 配置
2. ✅ 重启 aria_web 容器
3. ✅ 运行测试脚本验证
4. ✅ 更新防火墙规则（如果需要）
