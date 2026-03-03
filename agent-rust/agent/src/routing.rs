use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use std::collections::HashSet;

const VPN_TABLE: u32 = 100;
const DIRECT_TABLE: u32 = 200;
const MAIN_TABLE: u32 = 254;
const VPN_PRIORITY: u32 = 100;

#[derive(Error, Debug)]
pub enum RouteError {
    #[error("Failed to add route: {0}")]
    AddRoute(String),
    #[error("Failed to remove route: {0}")]
    RemoveRoute(String),
    #[error("Route not found: {0}")]
    RouteNotFound(String),
    #[error("Failed to initialize routing: {0}")]
    InitError(String),
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct RouteEntry {
    pub destination: String,
    pub interface: String,
    pub gateway: Option<String>,
    pub metric: Option<u32>,
    pub table: Option<u32>,
}

#[derive(Clone)]
pub struct RoutingManager {
    interface_name: String,
}

impl RoutingManager {
    pub fn new(interface_name: &str) -> Self {
        Self {
            interface_name: interface_name.to_string(),
        }
    }

    /// 初始化路由表和策略规则
    pub fn init(&self) -> Result<()> {
        tracing::info!("Routing Manager: initializing with interface {}", self.interface_name);

        self.ensure_tables()
            .context("Failed to ensure routing tables")?;

        self.ensure_rules()
            .context("Failed to ensure policy rules")?;

        tracing::info!("Routing Manager: initialized successfully");
        Ok(())
    }

    /// 确保路由表存在
    fn ensure_tables(&self) -> Result<()> {
        let tables = vec![VPN_TABLE, DIRECT_TABLE];
        
        for table in tables {
            let output = std::process::Command::new("ip")
                .args(&["route", "show", "table", &table.to_string()])
                .output()
                .context("Failed to check routing table")?;

            if !output.status.success() {
                tracing::info!("Routing Manager: table {} will be created on first use", table);
            }
        }

        Ok(())
    }

    /// 确保策略规则存在
    fn ensure_rules(&self) -> Result<()> {
        let rules = vec![
            (VPN_TABLE, VPN_PRIORITY),
            (DIRECT_TABLE, VPN_PRIORITY + 1),
        ];

        for (table, priority) in rules {
            // 检查规则是否已存在
            let output = std::process::Command::new("ip")
                .args(&["rule", "show", "priority", &priority.to_string()])
                .output()
                .context("Failed to check policy rule")?;

            let output_str = String::from_utf8_lossy(&output.stdout);
            
            if output_str.contains(&priority.to_string()) {
                continue; // 规则已存在
            }

            // 创建规则
            let add_output = std::process::Command::new("ip")
                .args(&["rule", "add", "priority", &priority.to_string(), "table", &table.to_string()])
                .output()
                .context("Failed to create policy rule")?;

            if !add_output.status.success() {
                tracing::warn!(
                    "Routing Manager: failed to create rule for table {}: {}",
                    table,
                    String::from_utf8_lossy(&add_output.stderr)
                );
            } else {
                tracing::info!("Routing Manager: created rule priority {} -> table {}", priority, table);
            }
        }

        Ok(())
    }

    /// 添加路由
    pub fn add_route(&self, route: &RouteEntry) -> Result<()> {
        let iface_name = if route.interface.is_empty() {
            &self.interface_name
        } else {
            &route.interface
        };

        let table = route.table.unwrap_or(MAIN_TABLE);

        let mut args = vec!["route", "replace", &route.destination, "dev", iface_name];

        let gateway_arg;
        if let Some(gw) = &route.gateway {
            gateway_arg = gw.clone();
            args.extend_from_slice(&["via", &gateway_arg]);
        }

        let metric_arg;
        if let Some(metric) = route.metric {
            metric_arg = metric.to_string();
            args.extend_from_slice(&["metric", &metric_arg]);
        }

        let table_arg;
        if table != MAIN_TABLE {
            table_arg = table.to_string();
            args.extend_from_slice(&["table", &table_arg]);
        }

        let output = std::process::Command::new("ip")
            .args(&args)
            .output()
            .context("Failed to execute ip route")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            // 路由已存在，使用 replace 命令时通常不会报错，但以防万一
            if stderr.contains("File exists") {
                tracing::debug!("Route {} already exists, updating", route.destination);
                return Ok(());
            }
            
            // 网络不可达等非致命错误
            if stderr.contains("Network is unreachable") {
                tracing::warn!("Route {} network is unreachable: {}", route.destination, stderr);
                return Ok(()); // 允许添加失败，后续可能网络恢复
            }

            return Err(RouteError::AddRoute(stderr).into());
        }

        tracing::info!("Routing Manager: added route {} via {}", route.destination, iface_name);
        Ok(())
    }

    /// 删除路由
    pub fn remove_route(&self, destination: &str) -> Result<()> {
        let output = std::process::Command::new("ip")
            .args(&["route", "del", destination, "dev", &self.interface_name])
            .output()
            .context("Failed to execute ip route del")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            // 路由不存在，静默成功
            if stderr.contains("No such process") || stderr.contains("Cannot find device") {
                tracing::debug!("Route {} does not exist, ignoring", destination);
                return Ok(());
            }

            return Err(RouteError::RemoveRoute(stderr).into());
        }

        tracing::info!("Routing Manager: removed route {}", destination);
        Ok(())
    }

    /// 添加 VPN 路由（到 table 100）
    pub fn add_vpn_route(&self, destination: &str) -> Result<()> {
        let output = std::process::Command::new("ip")
            .args(&["route", "add", destination, "dev", &self.interface_name, "table", &VPN_TABLE.to_string()])
            .output()
            .context("Failed to add VPN route")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            if stderr.contains("File exists") {
                return Ok(()); // 路由已存在，不报错
            }

            return Err(RouteError::AddRoute(format!("VPN route: {}", stderr)).into());
        }

        tracing::info!("Routing Manager: added VPN route {} via {}", destination, self.interface_name);
        Ok(())
    }

    /// 删除 VPN 路由
    pub fn remove_vpn_route(&self, destination: &str) -> Result<()> {
        let output = std::process::Command::new("ip")
            .args(&["route", "del", destination, "table", &VPN_TABLE.to_string()])
            .output()
            .context("Failed to remove VPN route")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            if stderr.contains("No such process") {
                return Ok(()); // 路由不存在，不报错
            }

            return Err(RouteError::RemoveRoute(format!("VPN route: {}", stderr)).into());
        }

        tracing::info!("Routing Manager: removed VPN route {}", destination);
        Ok(())
    }

    pub fn add_ecmp_route(&self, destination: &str, interfaces: &[&str]) -> Result<()> {
        if interfaces.is_empty() {
            return Err(RouteError::AddRoute("No interfaces for ECMP route".to_string()).into());
        }

        let mut args = vec!["route", "replace", destination, "proto", "static"];
        
        for iface in interfaces {
            args.extend_from_slice(&["nexthop", "dev", iface, "weight", "1"]);
        }

        let output = std::process::Command::new("ip")
            .args(&args)
            .output()
            .context("Failed to add ECMP route")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            // 路由已存在
            if stderr.contains("File exists") {
                tracing::debug!("ECMP route {} already exists, ignoring", destination);
                return Ok(());
            }

            return Err(RouteError::AddRoute(stderr).into());
        }

        tracing::info!("Routing Manager: added ECMP route {} via {} interfaces", 
            destination, interfaces.len());
        Ok(())
    }

    pub fn remove_ecmp_route(&self, destination: &str) -> Result<()> {
        let output = std::process::Command::new("ip")
            .args(&["route", "del", destination])
            .output()
            .context("Failed to remove ECMP route")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            if stderr.contains("No such process") || stderr.contains("not found") {
                return Ok(()); // 路由不存在，不报错
            }

            return Err(RouteError::RemoveRoute(format!("ECMP route: {}", stderr)).into());
        }

        tracing::info!("Routing Manager: removed ECMP route {}", destination);
        Ok(())
    }

    /// 添加直连路由（到 table 200）
    pub fn add_direct_route(&self, destination: &str, dev: &str) -> Result<()> {
        let output = std::process::Command::new("ip")
            .args(&["route", "add", destination, "dev", dev, "table", &DIRECT_TABLE.to_string()])
            .output()
            .context("Failed to add direct route")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            if stderr.contains("File exists") {
                return Ok(()); // 路由已存在
            }

            return Err(RouteError::AddRoute(format!("Direct route: {}", stderr)).into());
        }

        tracing::info!("Routing Manager: added direct route {} via {}", destination, dev);
        Ok(())
    }

    /// 清理策略规则
    pub fn cleanup(&self) -> Result<()> {
        let priorities = vec![VPN_PRIORITY, VPN_PRIORITY + 1];

        for priority in priorities {
            let _ = std::process::Command::new("ip")
                .args(&["rule", "del", "priority", &priority.to_string()])
                .output();
            // 忽略错误
        }

        tracing::info!("Routing Manager: cleaned up");
        Ok(())
    }

    /// 列出指定接口的路由
    pub fn list_routes(&self) -> Result<Vec<RouteEntry>> {
        let output = std::process::Command::new("ip")
            .args(&["route", "show", "dev", &self.interface_name])
            .output()
            .context("Failed to list routes")?;

        if !output.status.success() {
            return Err(RouteError::InitError(
                String::from_utf8_lossy(&output.stderr).to_string()
            ).into());
        }

        let routes_str = String::from_utf8_lossy(&output.stdout);
        let mut entries = Vec::new();

        for line in routes_str.lines() {
            if line.is_empty() {
                continue;
            }

            // 简单解析：提取目标网段
            let parts: Vec<&str> = line.split_whitespace().collect();
            
            if let Some(dest) = parts.first() {
                entries.push(RouteEntry {
                    destination: dest.to_string(),
                    interface: self.interface_name.clone(),
                    gateway: None,
                    metric: None,
                    table: Some(MAIN_TABLE),
                });
            }
        }

        Ok(entries)
    }
    
    /// 列出 VPN 路由表（table 100）中的所有路由
    pub fn list_vpn_routes(&self) -> Result<HashSet<String>> {
        let output = std::process::Command::new("ip")
            .args(&["route", "show", "table", &VPN_TABLE.to_string()])
            .output()
            .context("Failed to list VPN routes")?;
        
        if !output.status.success() {
            tracing::warn!("Failed to list VPN routes: {}", 
                String::from_utf8_lossy(&output.stderr));
            return Ok(HashSet::new());
        }
        
        let routes_str = String::from_utf8_lossy(&output.stdout);
        let mut routes = HashSet::new();
        
        for line in routes_str.lines() {
            if line.is_empty() {
                continue;
            }
            
            // 提取目标网段（第一个字段）
            if let Some(dest) = line.split_whitespace().next() {
                // 只收集有效的 CIDR 格式路由
                // 过滤掉 "default"、"local" 等特殊路由
                if is_valid_cidr(dest) {
                    routes.insert(dest.to_string());
                } else {
                    tracing::debug!("Skipping non-CIDR route: {}", dest);
                }
            }
        }
        
        Ok(routes)
    }
}

/// 检查是否是有效的 CIDR 格式
fn is_valid_cidr(s: &str) -> bool {
    // 过滤掉特殊路由
    if s == "default" || s == "local" || s == "unreachable" || s == "blackhole" || s == "prohibit" {
        return false;
    }
    
    // 必须包含 / 才是 CIDR 格式
    if !s.contains('/') {
        return false;
    }
    
    // 简单的格式验证：x.x.x.x/x 或 x:x:x:x:x:x:x:x/x
    let parts: Vec<&str> = s.split('/').collect();
    if parts.len() != 2 {
        return false;
    }
    
    // 验证前缀长度是数字
    if parts[1].parse::<u8>().is_err() {
        return false;
    }
    
    // 验证 IP 部分不为空
    if parts[0].is_empty() {
        return false;
    }
    
    // 简单检查：IPv4 应该包含点，IPv6 应该包含冒号
    let ip = parts[0];
    ip.contains('.') || ip.contains(':')
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_routing_manager_creation() {
        let manager = RoutingManager::new("test0");
        assert_eq!(manager.interface_name, "test0");
    }
}
