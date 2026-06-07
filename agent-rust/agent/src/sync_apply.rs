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
