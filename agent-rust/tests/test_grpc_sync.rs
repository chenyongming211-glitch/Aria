use aria_agent::grpc_client::GrpcClient;

#[tokio::main]
async fn main() {
    println!("=== Aria Agent gRPC Sync 测试（真实公钥）===\n");
    
    let controller_url = "https://112.124.8.241:50051".to_string();
    let ca_cert = "/etc/aria/certs/ca/ca.crt".to_string();
    let client_cert = "/etc/aria/certs/agents/agent-sh.crt".to_string();
    let client_key = "/etc/aria/certs/agents/agent-sh.key".to_string();
    
    // 使用真实的 sh 节点公钥
    let real_public_key = "g1uh7zx1hTKgeGAo2UVRw2YyhfOTFTG2jZ0kdv9XuTk=";
    
    println!("测试 1: 连接 Controller (mTLS)...");
    match GrpcClient::new(controller_url, ca_cert, client_cert, client_key).await {
        Ok(mut client) => {
            println!("✅ mTLS 连接成功！\n");
            
            println!("测试 2: 同步配置（使用真实公钥）...");
            match client.sync(real_public_key.to_string()).await {
                Ok(sync_result) => {
                    println!("✅ Sync 成功！");
                    println!("   分配的 IP: {}", sync_result.assigned_ip);
                    println!("   Peers 数量: {}", sync_result.peers.len());
                    println!("   ACL 规则数量: {}", sync_result.acl_rules.len());
                    
                    if !sync_result.peers.is_empty() {
                        println!("\n   Peers 列表:");
                        for (i, peer) in sync_result.peers.iter().enumerate() {
                            println!("   {}. {} ({}) - {}", 
                                i+1, 
                                peer.hostname, 
                                peer.assigned_ip,
                                peer.region
                            );
                        }
                    }
                    
                    if !sync_result.acl_rules.is_empty() {
                        println!("\n   ACL 规则:");
                        for (i, rule) in sync_result.acl_rules.iter().enumerate() {
                            println!("   {}. {} -> {} (proto: {}, ports: {}-{})", 
                                i+1,
                                rule.src_net,
                                rule.dst_net,
                                rule.protocol,
                                rule.min_port,
                                rule.max_port
                            );
                        }
                    }
                }
                Err(e) => {
                    println!("❌ Sync 失败: {:?}", e);
                }
            }
        }
        Err(e) => {
            println!("❌ 连接失败: {:?}", e);
        }
    }
    
    println!("\n=== 测试完成 ===");
}
