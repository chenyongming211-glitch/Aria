use aria_agent::grpc_client::GrpcClient;

#[tokio::main]
async fn main() {
    println!("=== Aria Agent gRPC 客户端测试 ===\n");
    
    // Controller 配置
    let controller_url = "https://112.124.8.241:50051".to_string();
    let ca_cert = "/etc/aria/certs/ca/ca.crt".to_string();
    let client_cert = "/etc/aria/certs/agents/agent-sh.crt".to_string();
    let client_key = "/etc/aria/certs/agents/agent-sh.key".to_string();
    
    println!("Controller URL: {}", controller_url);
    println!("CA Certificate: {}", ca_cert);
    println!("Client Certificate: {}", client_cert);
    println!("Client Key: {}", client_key);
    println!();
    
    // 测试 1: 连接 Controller
    println!("测试 1: 连接 Controller (mTLS)...");
    match GrpcClient::new(controller_url.clone(), ca_cert.clone(), client_cert.clone(), client_key.clone()).await {
        Ok(client) => {
            println!("✅ mTLS 连接成功！\n");
            
            // 测试 2: 注册
            println!("测试 2: 注册到 Controller...");
            let public_key = "test-rust-agent-001".to_string();
            let endpoint = ":51820".to_string();
            let public_ip = "146.56.196.231".to_string();
            let hostname = "test-rust-agent".to_string();
            let token = "test-token".to_string();
            let region = "sh".to_string();
            
            match client.register(public_key.clone(), endpoint, public_ip, hostname, token, region).await {
                Ok(registration) => {
                    println!("✅ 注册成功！");
                    println!("   分配的 IP: {}", registration.assigned_ip);
                    println!("   节点 ID: {}\n", registration.node_id.as_deref().unwrap_or("(none)"));
                    
                    // 测试 3: 同步配置
                    println!("测试 3: 同步配置...");
                    match client.sync(registration.node_id.clone(), public_key).await {
                        Ok(sync_result) => {
                            println!("✅ 同步成功！");
                            println!("   Peers 数量: {}", sync_result.peers.len());
                            println!("   ACL 规则数量: {}", sync_result.acl_rules.len());
                            println!("   分配的 IP: {}", sync_result.assigned_ip);
                            
                            if !sync_result.peers.is_empty() {
                                println!("\n   Peers 列表:");
                                for (i, peer) in sync_result.peers.iter().enumerate() {
                                    println!("   {}. {} ({})", i+1, peer.hostname, peer.assigned_ip);
                                }
                            }
                        }
                        Err(e) => {
                            println!("❌ 同步失败: {:?}", e);
                        }
                    }
                }
                Err(e) => {
                    println!("❌ 注册失败: {:?}", e);
                }
            }
        }
        Err(e) => {
            println!("❌ 连接失败: {:?}", e);
            println!("\n可能的原因:");
            println!("  1. Controller 未运行或端口未开放");
            println!("  2. 证书路径不正确");
            println!("  3. 证书不受信任或已过期");
            println!("  4. 网络连接问题");
        }
    }
    
    println!("\n=== 测试完成 ===");
}
