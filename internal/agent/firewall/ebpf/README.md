# Aria eBPF Firewall & QoS System

## 架构概述

Aria eBPF系统提供了一个高性能、低延迟的网络策略执行引擎，结合XDP和TC框架实现ACL（访问控制）和QoS（服务质量）功能。

## 设计特点

### 1. 双层架构
- **XDP (eXpress Data Path)**: 处理入站ACL，在网络栈最前端快速决策
- **TC (Traffic Control)**: 实现QoS和出站ACL，在网络栈更深处进行精细控制

### 2. 三层QoS策略（优先级递减）
1. **服务级QoS (App QoS)**: 基于五元组(SrcIP, DstIP, SrcPort, DstPort, Proto)的细粒度控制
2. **对等体QoS (Peer QoS)**: 基于IP的中等粒度控制
3. **全局QoS (Global QoS)**: 作为兜底的总体带宽限制

### 3. 数据结构设计

#### 核心结构体
- `acl_5tuple_key`: 五元组键结构，包含SrcIP, DstIP, SrcPort, DstPort, Proto
- `acl_rule_value`: ACL规则值，包含动作、规则ID、统计信息
- `bucket_state`: 令牌桶状态，用于QoS流控
- `flow_detail_key`: 流量详情键，用于可观测性
- `drop_event_t`: 丢包事件，用于监控和告警

#### 并发安全
- 所有共享状态使用 `bpf_spin_lock` 保证并发安全
- 使用 `__sync_fetch_and_add` 等原子操作进行统计更新

## eBPF Maps 设计

### 入站ACL (XDP层)
- `ingress_5tuple_map`: 5元组ACL规则表
- `ingress_port_blk_map`: 入站端口阻断表
- `ingress_ip_blk_map`: 入站IP阻断表

### 出站ACL (TC层)
- `egress_5tuple_map`: 出站5元组ACL规则表
- `egress_ip_blk_map`: 出站IP阻断表

### QoS控制 (TC层)
- `app_qos_map`: 服务级QoS表 (基于5元组)
- `peer_qos_map`: 对等体QoS表 (基于IP)
- `global_qos_map`: 全局QoS表 (数组类型，容量1，作为物理出口兜底)

### 全量可观测
- `rule_flow_table`: LRU per-CPU哈希表，存储流量详情
- `drop_alerts`: 环形缓冲区，实时传递丢包事件

## 编程接口

Go应用程序可通过`eBPF`包提供的高级API与内核模块交互：

### ACL管理
- `Apply5TupleACLRule`: 应用5元组ACL规则
- `BlockPort`: 阻断特定端口
- `BlockIP`: 阻断特定IP

### QoS管理
- `LimitIP`: 设置IP级带宽限制
- `LimitPeerPair`: 设置对等体间带宽限制
- `LimitService`: 设置服务级(5元组)带宽限制
- `LimitPort`: 设置端口级带宽限制

## 性能优化

1. **零拷贝**: eBPF程序直接在网络驱动中执行，避免数据包拷贝
2. **快速查找**: 使用哈希表实现O(1)复杂度的规则查找
3. **高效流控**: 令牌桶算法实现实时带宽控制
4. **内存优化**: LRU表自动回收过期记录，防止内存耗尽
5. **原子操作**: 所有统计更新使用原子操作，避免锁竞争

## 容错与恢复

- **持久化**: 所有eBPF maps支持pinning到bpffs，进程崩溃后可恢复
- **原子更新**: 规则更新使用原子操作，保证一致性
- **健康监控**: 通过ringbuf实时上报丢包事件，便于故障排查

## 编译与部署

```bash
cd internal/agent/firewall/ebpf
mkdir build && cd build
cmake ..
make
```

编译后会生成`xdp_acl_filter.o`和`tc_qos_filter.o`两个eBPF对象文件。

## 安全考虑

- **最小权限**: 程序仅申请必要权限，限制对内核的访问
- **边界检查**: 所有内存访问前进行边界验证
- **输入验证**: 网络包解析进行完整性检查
- **审计日志**: 所有丢包事件记录到ringbuf，支持事后审计