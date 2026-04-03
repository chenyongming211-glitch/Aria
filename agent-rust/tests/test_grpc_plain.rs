use tonic::transport::Channel;
use aria_agent::grpc_client::aria::controller_service_client::ControllerServiceClient;
use aria_agent::grpc_client::aria::SyncRequest;

#[tokio::main]
async fn main() {
    println!("=== Aria Agent gRPC 连接测试（不验证证书）===\n");
    
    // 测试 1: 不使用 TLS
    println!("测试 1: 连接 plaintext (无 TLS)...");
    let plain_url = "http://112.124.8.241:50051";
    
    match Channel::from_static(plain_url).connect().await {
        Ok(channel) => {
            println!("✅ 连接成功！\n");
            
            let mut client = ControllerServiceClient::new(channel);
            
            println!("测试 2: 调用 Sync API...");
            let request = tonic::Request::new(SyncRequest {
                public_key: "test-rust-agent-001".to_string(),
                node_id: String::new(),
            });
            
            match client.sync(request).await {
                Ok(response) => {
                    let resp = response.into_inner();
                    println!("✅ Sync 成功！");
                    println!("   Peers 数量: {}", resp.peers.len());
                    println!("   ACL 规则数量: {}", resp.acl_rules.len());
                    println!("   分配的 IP: {}", resp.assigned_ip);
                    
                    if !resp.peers.is_empty() {
                        println!("\n   Peers 列表:");
                        for (i, peer) in resp.peers.iter().enumerate() {
                            println!("   {}. {} ({})", i+1, peer.hostname, peer.assigned_ip);
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
