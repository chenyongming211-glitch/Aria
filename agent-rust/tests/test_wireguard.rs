use aria_agent::wireguard::{WireGuardManager, InterfaceConfig, PeerConfig};

fn main() {
    println!("=== WireGuard 管理器测试 ===\n");
    
    // 测试 1: 生成密钥对
    println!("测试 1: 生成密钥对...");
    match WireGuardManager::generate_keypair() {
        Ok((private, public)) => {
            println!("✅ 私钥: {}", &private[..20]);
            println!("   公钥: {}", &public[..20]);
            
            // 测试 2: 创建接口
            println!("\n测试 2: 创建 WireGuard 接口...");
            let mut manager = WireGuardManager::new("test-wg0");
            
            let config = InterfaceConfig {
                name: "test-wg0".to_string(),
                private_key: private.clone(),
                listen_port: 13300,  // 使用非标准端口避免冲突
                mtu: 1360,
                address: Some("10.0.0.1/24".to_string()),
            };
            
            match manager.create_interface(config) {
                Ok(_) => {
                    println!("✅ 接口创建成功");
                    
                    // 测试 3: 添加 Peer
                    println!("\n测试 3: 添加 Peer...");
                    let (_, peer_public) = WireGuardManager::generate_keypair().unwrap();
                    
                    let peer_config = PeerConfig {
                        public_key: peer_public.clone(),
                        endpoint: Some("1.2.3.4:51820".to_string()),
                        allowed_ips: vec!["10.0.0.2/32".to_string()],
                        persistent_keepalive: 25,
                    };
                    
                    match manager.add_peer(peer_config) {
                        Ok(_) => {
                            println!("✅ Peer 添加成功");
                            
                            // 测试 4: 列出 Peers
                            println!("\n测试 4: 列出 Peers...");
                            match manager.list_peers() {
                                Ok(peers) => {
                                    println!("✅ 找到 {} 个 Peers", peers.len());
                                    for (i, peer) in peers.iter().enumerate() {
                                        println!("   {}. {}... - {:?}", 
                                            i+1, 
                                            &peer.public_key[..16],
                                            peer.endpoint
                                        );
                                    }
                                    
                                    // 测试 5: 获取统计信息
                                    println!("\n测试 5: 获取统计信息...");
                                    match manager.get_stats() {
                                        Ok(stats) => {
                                    println!("✅ 接口: {}", stats.interface_name);
                                    if !stats.public_key.is_empty() {
                                        println!("   公钥: {}...", &stats.public_key[..20.min(stats.public_key.len())]);
                                    }
                                    println!("   监听端口: {}", stats.listen_port);
                                    println!("   Peers 数量: {}", stats.peers.len());
                                        }
                                        Err(e) => {
                                            println!("❌ 获取统计信息失败: {:?}", e);
                                        }
                                    }
                                }
                                Err(e) => {
                                    println!("❌ 列出 Peers 失败: {:?}", e);
                                }
                            }
                            
                            // 测试 6: 删除 Peer
                            println!("\n测试 6: 删除 Peer...");
                            match manager.remove_peer(&peer_public) {
                                Ok(_) => {
                                    println!("✅ Peer 删除成功");
                                }
                                Err(e) => {
                                    println!("❌ 删除 Peer 失败: {:?}", e);
                                }
                            }
                        }
                        Err(e) => {
                            println!("❌ 添加 Peer 失败: {:?}", e);
                        }
                    }
                    
                    // 测试 7: 删除接口
                    println!("\n测试 7: 删除接口...");
                    match manager.delete_interface() {
                        Ok(_) => {
                            println!("✅ 接口删除成功");
                        }
                        Err(e) => {
                            println!("❌ 删除接口失败: {:?}", e);
                        }
                    }
                }
                Err(e) => {
                    println!("❌ 接口创建失败: {:?}", e);
                }
            }
        }
        Err(e) => {
            println!("❌ 密钥生成失败: {:?}", e);
        }
    }
    
    println!("\n=== 测试完成 ===");
}
