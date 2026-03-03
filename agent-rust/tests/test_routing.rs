use aria_agent::routing::{RoutingManager, RouteEntry};

fn main() {
    println!("=== 路由管理器测试 ===\n");

    // 测试 1: 创建路由管理器
    println!("测试 1: 创建路由管理器...");
    let manager = RoutingManager::new("test-route0");
    println!("✅ 路由管理器创建成功\n");

    // 测试 2: 初始化（创建路由表和策略规则）
    println!("测试 2: 初始化路由表和策略规则...");
    match manager.init() {
        Ok(_) => {
            println!("✅ 初始化成功");
            println!("   - VPN 路由表 (table 100)");
            println!("   - Direct 路由表 (table 200)");
            println!("   - 策略规则已创建\n");
        }
        Err(e) => {
            println!("❌ 初始化失败: {:?}\n", e);
            return;
        }
    }

    // 测试 3: 添加普通路由
    println!("测试 3: 添加普通路由...");
    let route = RouteEntry {
        destination: "10.100.0.0/24".to_string(),
        interface: "test-route0".to_string(),
        gateway: None,
        metric: Some(100),
        table: None,
    };

    match manager.add_route(&route) {
        Ok(_) => {
            println!("✅ 路由添加成功: {}\n", route.destination);
        }
        Err(e) => {
            println!("❌ 路由添加失败: {:?}\n", e);
        }
    }

    // 测试 4: 添加 VPN 路由（到 table 100）
    println!("测试 4: 添加 VPN 路由...");
    match manager.add_vpn_route("192.168.1.0/24") {
        Ok(_) => {
            println!("✅ VPN 路由添加成功 (table 100)\n");
        }
        Err(e) => {
            println!("❌ VPN 路由添加失败: {:?}\n", e);
        }
    }

    // 测试 5: 添加直连路由（到 table 200）
    println!("测试 5: 添加直连路由...");
    match manager.add_direct_route("172.16.0.0/16", "eth0") {
        Ok(_) => {
            println!("✅ 直连路由添加成功 (table 200)\n");
        }
        Err(e) => {
            println!("❌ 直连路由添加失败: {:?}\n", e);
        }
    }

    // 测试 6: 列出路由
    println!("测试 6: 列出路由...");
    match manager.list_routes() {
        Ok(routes) => {
            println!("✅ 找到 {} 个路由", routes.len());
            for (i, r) in routes.iter().enumerate() {
                println!("   {}. {} via {}", i + 1, r.destination, r.interface);
            }
            println!();
        }
        Err(e) => {
            println!("❌ 列出路由失败: {:?}\n", e);
        }
    }

    // 测试 7: 删除路由
    println!("测试 7: 删除路由...");
    match manager.remove_route("10.100.0.0/24") {
        Ok(_) => {
            println!("✅ 路由删除成功\n");
        }
        Err(e) => {
            println!("❌ 路由删除失败: {:?}\n", e);
        }
    }

    // 测试 8: 清理策略规则
    println!("测试 8: 清理策略规则...");
    match manager.cleanup() {
        Ok(_) => {
            println!("✅ 策略规则清理成功\n");
        }
        Err(e) => {
            println!("❌ 清理失败: {:?}\n", e);
        }
    }

    println!("=== 测试完成 ===");
}
