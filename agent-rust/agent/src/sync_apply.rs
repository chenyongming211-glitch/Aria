use anyhow::{Context, Result};

use crate::acl::{parse_ports, ACTION_DROP, ACTION_PASS};
use crate::grpc_client::{
    acl_policy_from_sync_rule, qos_policy_from_sync_rule, AclRule, QoSRule,
};

const MAX_EXPANDED_ACL_PORTS: u32 = 4096;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AclApplyOperation {
    pub src_net: String,
    pub dst_net: String,
    pub dst_port: u16,
    pub protocol: u8,
    pub action: u32,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum QosApplyTarget {
    Source { cidr: String },
    Pair { src_cidr: String, dst_cidr: String },
    Service {
        src_cidr: String,
        dst_cidr: String,
        dst_port: u16,
        protocol: u8,
    },
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct QosApplyOperation {
    pub target: QosApplyTarget,
    pub rate_bps: u64,
    pub rate_bytes_per_sec: u64,
    pub burst_bytes: u64,
    pub priority: u8,
    pub mode: u8,
}

pub fn acl_apply_operations_from_sync_rule(rule: &AclRule) -> Result<Vec<AclApplyOperation>> {
    let policy = acl_policy_from_sync_rule(rule)?;
    if policy.direction != 0 {
        return Err(anyhow::anyhow!(
            "egress ACL is not supported by current dataplane"
        ));
    }

    let default_action = match policy.action {
        0 => ACTION_PASS,
        1 => ACTION_DROP,
        other => {
            return Err(anyhow::anyhow!(
                "invalid ACL action {} after sync policy normalization",
                other
            ));
        }
    };

    let Some(ports) = policy.ports.as_deref() else {
        return Ok(vec![AclApplyOperation {
            src_net: dataplane_cidr(policy.src_group),
            dst_net: dataplane_cidr(policy.dst_group),
            dst_port: 0,
            protocol: policy.proto,
            action: default_action,
        }]);
    };

    let mut operations = Vec::new();
    operations.push(AclApplyOperation {
        src_net: dataplane_cidr(policy.src_group.clone()),
        dst_net: dataplane_cidr(policy.dst_group.clone()),
        dst_port: 0,
        protocol: policy.proto,
        action: inverted_acl_action(default_action)?,
    });

    for (start, end, encoded_action) in parse_ports(ports, default_action)? {
        let span = u32::from(end) - u32::from(start) + 1;
        if operations.len() as u32 + span > MAX_EXPANDED_ACL_PORTS {
            return Err(anyhow::anyhow!(
                "ACL port expansion exceeds current dataplane limit of {} entries",
                MAX_EXPANDED_ACL_PORTS
            ));
        }

        let action = acl_action_from_port_encoding(encoded_action)?;
        for port in start..=end {
            operations.push(AclApplyOperation {
                src_net: dataplane_cidr(policy.src_group.clone()),
                dst_net: dataplane_cidr(policy.dst_group.clone()),
                dst_port: port,
                protocol: policy.proto,
                action,
            });
        }
    }

    if operations.is_empty() {
        operations.push(AclApplyOperation {
            src_net: dataplane_cidr(policy.src_group),
            dst_net: dataplane_cidr(policy.dst_group),
            dst_port: 0,
            protocol: policy.proto,
            action: default_action,
        });
    }

    Ok(operations)
}

pub fn qos_apply_operation_from_sync_rule(rule: &QoSRule) -> Result<QosApplyOperation> {
    let policy = qos_policy_from_sync_rule(rule)?;
    if policy.direction != 1 {
        return Err(anyhow::anyhow!(
            "ingress QoS is not supported by current dataplane"
        ));
    }
    if policy.mode != 0 {
        return Err(anyhow::anyhow!(
            "QoS shaping mode is not supported by current dataplane"
        ));
    }
    if policy.rate_bps == 0 {
        return Err(anyhow::anyhow!("QoS rate_bps must be greater than zero"));
    }

    let target = qos_target_from_sync_rule(rule)
        .with_context(|| format!("invalid QoS sync rule target: {:?}", rule))?;
    let rate_bytes_per_sec = (policy.rate_bps / 8).max(1);

    Ok(QosApplyOperation {
        target,
        rate_bps: policy.rate_bps,
        rate_bytes_per_sec,
        burst_bytes: policy.burst_bytes,
        priority: policy.priority,
        mode: policy.mode,
    })
}

fn acl_action_from_port_encoding(action: u8) -> Result<u32> {
    match action {
        1 => Ok(ACTION_DROP),
        2 => Ok(ACTION_PASS),
        other => Err(anyhow::anyhow!("invalid encoded ACL port action {}", other)),
    }
}

fn inverted_acl_action(action: u32) -> Result<u32> {
    match action {
        ACTION_PASS => Ok(ACTION_DROP),
        ACTION_DROP => Ok(ACTION_PASS),
        other => Err(anyhow::anyhow!("invalid ACL action {}", other)),
    }
}

fn dataplane_cidr(cidr: String) -> String {
    if cidr == "any" {
        String::new()
    } else {
        cidr
    }
}

fn qos_target_from_sync_rule(rule: &QoSRule) -> Result<QosApplyTarget> {
    let src = rule.src_ip.trim();
    let dst = rule.dst_ip.trim();

    if src.is_empty() && dst.is_empty() {
        return Err(anyhow::anyhow!(
            "port-only QoS rules are not supported by current dataplane"
        ));
    }

    if src.is_empty() || dst.is_empty() {
        return Ok(QosApplyTarget::Source {
            cidr: if src.is_empty() {
                dst.to_string()
            } else {
                src.to_string()
            },
        });
    }

    if rule.src_port == 0 && rule.dst_port == 0 {
        return Ok(QosApplyTarget::Pair {
            src_cidr: src.to_string(),
            dst_cidr: dst.to_string(),
        });
    }

    let dst_port = u16::try_from(rule.dst_port).context("QoS dst_port must fit in u16")?;
    let protocol = u8::try_from(rule.protocol).context("QoS protocol must fit in u8")?;
    Ok(QosApplyTarget::Service {
        src_cidr: src.to_string(),
        dst_cidr: dst.to_string(),
        dst_port,
        protocol,
    })
}

#[cfg(test)]
mod tests {
    use super::{acl_apply_operations_from_sync_rule, qos_apply_operation_from_sync_rule};
    use crate::acl::{ACTION_DROP, ACTION_PASS};
    use crate::grpc_client::{AclRule, QoSRule};

    #[test]
    fn acl_sync_rule_expands_reference_ports_before_apply() {
        let operations = acl_apply_operations_from_sync_rule(&AclRule {
            src_net: "10.10.0.0/24".to_string(),
            dst_net: "10.20.0.0/24".to_string(),
            protocol: 6,
            min_port: 0,
            max_port: 0,
            action: "deny".to_string(),
            direction: "ingress".to_string(),
            ports: "80-82,443:0".to_string(),
        })
        .expect("valid ACL sync rule");

        let ports_and_actions: Vec<(u16, u32)> = operations
            .iter()
            .map(|op| (op.dst_port, op.action))
            .collect();

        assert_eq!(
            ports_and_actions,
            vec![
                (0, ACTION_PASS),
                (80, ACTION_DROP),
                (81, ACTION_DROP),
                (82, ACTION_DROP),
                (443, ACTION_PASS),
            ]
        );
    }

    #[test]
    fn acl_sync_rule_rejects_unsupported_egress_direction() {
        let err = acl_apply_operations_from_sync_rule(&AclRule {
            src_net: "10.10.0.0/24".to_string(),
            dst_net: "10.20.0.0/24".to_string(),
            protocol: 6,
            min_port: 443,
            max_port: 443,
            action: "allow".to_string(),
            direction: "egress".to_string(),
            ports: String::new(),
        })
        .expect_err("current dataplane has ingress ACL only");

        assert!(err.to_string().contains("egress ACL is not supported"));
    }

    #[test]
    fn acl_allow_with_ports_adds_drop_fallback() {
        let operations = acl_apply_operations_from_sync_rule(&AclRule {
            src_net: "10.10.0.0/24".to_string(),
            dst_net: "10.20.0.0/24".to_string(),
            protocol: 6,
            min_port: 0,
            max_port: 0,
            action: "allow".to_string(),
            direction: "ingress".to_string(),
            ports: "443".to_string(),
        })
        .expect("valid ACL sync rule");

        let ports_and_actions: Vec<(u16, u32)> = operations
            .iter()
            .map(|op| (op.dst_port, op.action))
            .collect();

        assert_eq!(ports_and_actions, vec![(0, ACTION_DROP), (443, ACTION_PASS)]);
    }

    #[test]
    fn qos_sync_rule_keeps_rate_and_burst_before_apply() {
        let operation = qos_apply_operation_from_sync_rule(&QoSRule {
            src_ip: "10.10.0.0/24".to_string(),
            dst_ip: "10.20.0.0/24".to_string(),
            src_port: 0,
            dst_port: 443,
            protocol: 6,
            bandwidth_mbps: 0,
            direction: "egress".to_string(),
            rate_bps: 250_000_000,
            burst_bytes: 4_000_000,
            priority: 7,
            mode: "policing".to_string(),
        })
        .expect("valid QoS sync rule");

        assert_eq!(operation.rate_bytes_per_sec, 31_250_000);
        assert_eq!(operation.burst_bytes, 4_000_000);
        assert_eq!(operation.priority, 7);
    }
}
