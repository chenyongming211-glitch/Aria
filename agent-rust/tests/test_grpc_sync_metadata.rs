use std::collections::HashMap;

use aria_agent::grpc_client::{aria, SyncResult};

#[test]
fn sync_result_preserves_phase1_snapshot_metadata_shape() {
    let mut domain_versions = HashMap::new();
    domain_versions.insert("acl".to_string(), "dsv-phase1".to_string());

    let result = SyncResult {
        peers: vec![],
        assigned_ip: "100.64.0.2".to_string(),
        desired_state_version: "dsv-phase1".to_string(),
        ip_groups: vec![],
        acl_rules: vec![],
        qos_rules: vec![],
        blacklist_rules: vec![],
        runtime_token: None,
        runtime_token_expires_at: None,
        snapshot_complete: true,
        domain_versions: domain_versions.clone(),
    };

    assert!(result.snapshot_complete);
    assert_eq!(result.domain_versions.get("acl"), Some(&"dsv-phase1".to_string()));

    let response = aria::SyncResponse {
        snapshot_complete: true,
        domain_versions,
        ..Default::default()
    };
    assert!(response.snapshot_complete);
    assert_eq!(response.domain_versions.get("acl"), Some(&"dsv-phase1".to_string()));
}
