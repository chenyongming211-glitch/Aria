use anyhow::{Context, Result};
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::Duration;

const DEFAULT_BOOTSTRAP_PATH: &str = "/etc/aria/agent.yaml";
const DEFAULT_STATE_PATH: &str = "/var/lib/aria/agent-state.yaml";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BootstrapConfig {
    #[serde(default)]
    pub controller_url: String,
    #[serde(default)]
    pub controller_api_url: String,
    #[serde(default)]
    pub ca_cert: String,
    #[serde(default)]
    pub client_cert: String,
    #[serde(default)]
    pub client_key: String,
    #[serde(default)]
    pub tls_server_name: Option<String>,
    #[serde(default)]
    pub enrollment_token: Option<String>,
    #[serde(default)]
    pub public_ip: Option<String>,
    #[serde(default)]
    pub public_endpoint: Option<String>,
    #[serde(default = "default_interface")]
    pub interface_name: String,
    #[serde(default = "default_listen_port")]
    pub listen_port: u16,
    #[serde(default = "default_mtu")]
    pub mtu: u32,
    #[serde(default)]
    pub region: Option<String>,
    #[serde(default)]
    pub customer_id: Option<String>,
    #[serde(default)]
    pub advertised_routes: Option<Vec<String>>,
    #[serde(default)]
    pub hostname: Option<String>,
    #[serde(default = "default_sync_interval", with = "serde_duration")]
    pub sync_interval: Duration,
    #[serde(default = "default_certificate_renew_before", with = "serde_duration")]
    pub certificate_renew_before: Duration,
    #[serde(default = "default_multi_tunnel")]
    pub multi_tunnel: bool,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AgentState {
    #[serde(default)]
    pub node_id: Option<String>,
    #[serde(default)]
    pub device_id: Option<String>,
    #[serde(default)]
    pub private_key: String,
    #[serde(default)]
    pub public_key: String,
    #[serde(default)]
    pub assigned_ip: Option<String>,
    #[serde(default)]
    pub address: Option<String>,
    #[serde(default)]
    pub current_credential: Option<String>,
    #[serde(default)]
    pub current_credential_expires_at: Option<i64>,
    #[serde(default)]
    pub last_desired_version: Option<String>,
    #[serde(default)]
    pub last_applied_version: Option<String>,
    #[serde(default)]
    pub latest_domain_versions: HashMap<String, String>,
    #[serde(default)]
    pub last_sync_status: Option<String>,
    #[serde(default)]
    pub last_sync_message: Option<String>,
    #[serde(default)]
    pub last_sync_at: Option<i64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentConfig {
    #[serde(default)]
    pub controller_url: String,
    #[serde(default)]
    pub controller_api_url: String,
    #[serde(default)]
    pub ca_cert: String,
    #[serde(default)]
    pub client_cert: String,
    #[serde(default)]
    pub client_key: String,
    #[serde(default)]
    pub tls_server_name: Option<String>,
    #[serde(default)]
    pub enrollment_token: Option<String>,
    #[serde(default)]
    pub public_ip: Option<String>,
    #[serde(default)]
    pub public_endpoint: Option<String>,
    #[serde(default)]
    pub node_id: Option<String>,
    #[serde(default)]
    pub device_id: Option<String>,
    #[serde(default)]
    pub private_key: String,
    #[serde(default)]
    pub public_key: String,
    #[serde(default)]
    pub assigned_ip: Option<String>,
    #[serde(default)]
    pub address: Option<String>,
    #[serde(default = "default_interface")]
    pub interface_name: String,
    #[serde(default = "default_listen_port")]
    pub listen_port: u16,
    #[serde(default = "default_mtu")]
    pub mtu: u32,
    #[serde(default)]
    pub region: Option<String>,
    #[serde(default)]
    pub customer_id: Option<String>,
    #[serde(default)]
    pub advertised_routes: Option<Vec<String>>,
    #[serde(default)]
    pub hostname: Option<String>,
    #[serde(default = "default_sync_interval", with = "serde_duration")]
    pub sync_interval: Duration,
    #[serde(default = "default_certificate_renew_before", with = "serde_duration")]
    pub certificate_renew_before: Duration,
    #[serde(default = "default_multi_tunnel")]
    pub multi_tunnel: bool,
    #[serde(default)]
    pub current_credential: Option<String>,
    #[serde(default)]
    pub current_credential_expires_at: Option<i64>,
    #[serde(default)]
    pub last_desired_version: Option<String>,
    #[serde(default)]
    pub last_applied_version: Option<String>,
    #[serde(default)]
    pub latest_domain_versions: HashMap<String, String>,
    #[serde(default)]
    pub last_sync_status: Option<String>,
    #[serde(default)]
    pub last_sync_message: Option<String>,
    #[serde(default)]
    pub last_sync_at: Option<i64>,
}

fn default_interface() -> String {
    "aria0".to_string()
}

fn default_listen_port() -> u16 {
    51820
}

fn default_mtu() -> u32 {
    1360
}

fn default_sync_interval() -> Duration {
    Duration::from_secs(5)
}

fn default_certificate_renew_before() -> Duration {
    Duration::from_secs(72 * 60 * 60)
}

fn default_multi_tunnel() -> bool {
    true
}

mod serde_duration {
    use serde::{Deserialize, Deserializer, Serialize, Serializer};
    use std::time::Duration;

    pub fn serialize<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        duration.as_secs().serialize(serializer)
    }

    pub fn deserialize<'de, D>(deserializer: D) -> Result<Duration, D::Error>
    where
        D: Deserializer<'de>,
    {
        let secs = u64::deserialize(deserializer)?;
        Ok(Duration::from_secs(secs))
    }
}

impl Default for BootstrapConfig {
    fn default() -> Self {
        Self {
            controller_url: String::new(),
            controller_api_url: String::new(),
            ca_cert: String::new(),
            client_cert: String::new(),
            client_key: String::new(),
            tls_server_name: None,
            enrollment_token: None,
            public_ip: None,
            public_endpoint: None,
            interface_name: default_interface(),
            listen_port: default_listen_port(),
            mtu: default_mtu(),
            region: None,
            customer_id: None,
            advertised_routes: None,
            hostname: None,
            sync_interval: default_sync_interval(),
            certificate_renew_before: default_certificate_renew_before(),
            multi_tunnel: default_multi_tunnel(),
        }
    }
}

impl Default for AgentConfig {
    fn default() -> Self {
        Self::from_parts(BootstrapConfig::default(), AgentState::default())
    }
}

impl AgentState {
    fn has_materialized_runtime(&self) -> bool {
        self.node_id.is_some()
            || self.device_id.is_some()
            || !self.private_key.trim().is_empty()
            || !self.public_key.trim().is_empty()
            || self.assigned_ip.is_some()
            || self.address.is_some()
            || self.current_credential.is_some()
            || self.current_credential_expires_at.is_some()
            || self.last_desired_version.is_some()
            || self.last_applied_version.is_some()
            || !self.latest_domain_versions.is_empty()
            || self.last_sync_status.is_some()
            || self.last_sync_message.is_some()
            || self.last_sync_at.is_some()
    }
}

impl AgentConfig {
    pub fn from_parts(bootstrap: BootstrapConfig, state: AgentState) -> Self {
        Self {
            controller_url: bootstrap.controller_url,
            controller_api_url: bootstrap.controller_api_url,
            ca_cert: bootstrap.ca_cert,
            client_cert: bootstrap.client_cert,
            client_key: bootstrap.client_key,
            tls_server_name: bootstrap.tls_server_name,
            enrollment_token: bootstrap.enrollment_token,
            public_ip: bootstrap.public_ip,
            public_endpoint: bootstrap.public_endpoint,
            node_id: state.node_id,
            device_id: state.device_id,
            private_key: state.private_key,
            public_key: state.public_key,
            assigned_ip: state.assigned_ip,
            address: state.address,
            interface_name: bootstrap.interface_name,
            listen_port: bootstrap.listen_port,
            mtu: bootstrap.mtu,
            region: bootstrap.region,
            customer_id: bootstrap.customer_id,
            advertised_routes: bootstrap.advertised_routes,
            hostname: bootstrap.hostname,
            sync_interval: bootstrap.sync_interval,
            certificate_renew_before: bootstrap.certificate_renew_before,
            multi_tunnel: bootstrap.multi_tunnel,
            current_credential: state.current_credential,
            current_credential_expires_at: state.current_credential_expires_at,
            last_desired_version: state.last_desired_version,
            last_applied_version: state.last_applied_version,
            latest_domain_versions: state.latest_domain_versions,
            last_sync_status: state.last_sync_status,
            last_sync_message: state.last_sync_message,
            last_sync_at: state.last_sync_at,
        }
    }

    pub fn to_bootstrap(&self) -> BootstrapConfig {
        BootstrapConfig {
            controller_url: self.controller_url.clone(),
            controller_api_url: self.controller_api_url.clone(),
            ca_cert: self.ca_cert.clone(),
            client_cert: self.client_cert.clone(),
            client_key: self.client_key.clone(),
            tls_server_name: self.tls_server_name.clone(),
            enrollment_token: self.enrollment_token.clone(),
            public_ip: self.public_ip.clone(),
            public_endpoint: self.public_endpoint.clone(),
            interface_name: self.interface_name.clone(),
            listen_port: self.listen_port,
            mtu: self.mtu,
            region: self.region.clone(),
            customer_id: self.customer_id.clone(),
            advertised_routes: self.advertised_routes.clone(),
            hostname: self.hostname.clone(),
            sync_interval: self.sync_interval,
            certificate_renew_before: self.certificate_renew_before,
            multi_tunnel: self.multi_tunnel,
        }
    }

    pub fn to_state(&self) -> AgentState {
        AgentState {
            node_id: self.node_id.clone(),
            device_id: self.device_id.clone(),
            private_key: self.private_key.clone(),
            public_key: self.public_key.clone(),
            assigned_ip: self.assigned_ip.clone(),
            address: self.address.clone(),
            current_credential: self.current_credential.clone(),
            current_credential_expires_at: self.current_credential_expires_at,
            last_desired_version: self.last_desired_version.clone(),
            last_applied_version: self.last_applied_version.clone(),
            latest_domain_versions: self.latest_domain_versions.clone(),
            last_sync_status: self.last_sync_status.clone(),
            last_sync_message: self.last_sync_message.clone(),
            last_sync_at: self.last_sync_at,
        }
    }
}

pub struct ConfigManager {
    bootstrap_path: String,
    state_path: String,
}

impl ConfigManager {
    pub fn new(bootstrap_path: &str) -> Self {
        Self {
            bootstrap_path: bootstrap_path.to_string(),
            state_path: derive_state_path(bootstrap_path),
        }
    }

    pub fn bootstrap_path(&self) -> &str {
        &self.bootstrap_path
    }

    pub fn state_path(&self) -> &str {
        &self.state_path
    }

    pub fn load_parts_or_init(&self, force: bool) -> Result<Option<(BootstrapConfig, AgentState)>> {
        if force {
            return Ok(None);
        }

        if !Path::new(&self.bootstrap_path).exists() {
            return Ok(None);
        }

        self.load_parts().map(Some)
    }

    pub fn load(&self) -> Result<AgentConfig> {
        let (bootstrap, state) = self.load_parts()?;
        Ok(AgentConfig::from_parts(bootstrap, state))
    }

    pub fn load_parts(&self) -> Result<(BootstrapConfig, AgentState)> {
        let bootstrap = self.load_bootstrap()?;
        let state = self.load_or_migrate_state()?;
        Ok((bootstrap, state))
    }

    pub fn load_bootstrap(&self) -> Result<BootstrapConfig> {
        read_yaml_file(&self.bootstrap_path)
    }

    pub fn load_state_opt(&self) -> Result<Option<AgentState>> {
        if !Path::new(&self.state_path).exists() {
            return Ok(None);
        }

        read_yaml_file(&self.state_path).map(Some)
    }

    pub fn save_bootstrap(&self, bootstrap: &BootstrapConfig) -> Result<()> {
        write_yaml_file(&self.bootstrap_path, bootstrap)
    }

    pub fn save_state(&self, state: &AgentState) -> Result<()> {
        write_yaml_file(&self.state_path, state)
    }

    fn load_or_migrate_state(&self) -> Result<AgentState> {
        match self.load_state_opt() {
            Ok(Some(state)) => return Ok(state),
            Ok(None) => {}
            Err(err) => {
                tracing::warn!(
                    "Failed to load runtime state from {}; falling back to legacy config or fresh state: {:?}",
                    self.state_path,
                    err
                );
            }
        }

        let state = self
            .load_legacy_config_opt()?
            .map(|legacy| {
                let state = legacy.to_state();
                (legacy.to_bootstrap(), state)
            })
            .unwrap_or_default();

        let (bootstrap, state) = state;
        if state.has_materialized_runtime() {
            self.save_bootstrap(&bootstrap)?;
            self.save_state(&state)?;
        }

        Ok(state)
    }

    fn load_legacy_config_opt(&self) -> Result<Option<AgentConfig>> {
        if !Path::new(&self.bootstrap_path).exists() {
            return Ok(None);
        }

        let data = std::fs::read_to_string(&self.bootstrap_path)
            .context(format!("Failed to read config file: {}", self.bootstrap_path))?;

        serde_yaml::from_str::<AgentConfig>(&data)
            .context("Failed to parse legacy config YAML")
            .map(Some)
    }
}

fn derive_state_path(bootstrap_path: &str) -> String {
    if bootstrap_path == DEFAULT_BOOTSTRAP_PATH {
        return DEFAULT_STATE_PATH.to_string();
    }

    let bootstrap = PathBuf::from(bootstrap_path);
    let parent = bootstrap
        .parent()
        .map(|path| path.to_path_buf())
        .unwrap_or_else(|| PathBuf::from("."));
    let stem = bootstrap
        .file_stem()
        .and_then(|value| value.to_str())
        .unwrap_or("agent");

    parent
        .join(format!("{}.state.yaml", stem))
        .to_string_lossy()
        .into_owned()
}

fn read_yaml_file<T>(path: &str) -> Result<T>
where
    T: DeserializeOwned,
{
    let data = std::fs::read_to_string(path)
        .context(format!("Failed to read config file: {}", path))?;

    serde_yaml::from_str(&data).context(format!("Failed to parse YAML from {}", path))
}

fn write_yaml_file<T>(path: &str, payload: &T) -> Result<()>
where
    T: Serialize,
{
    let target_path = Path::new(path);
    if let Some(parent) = target_path.parent() {
        std::fs::create_dir_all(parent)
            .context("Failed to create config directory")?;
    }

    let data = serde_yaml::to_string(payload)
        .context("Failed to serialize config")?;

    let file_name = target_path
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("agent.yaml");
    let tmp_suffix = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let tmp_path = target_path.with_file_name(format!(
        ".{}.tmp.{}.{}",
        file_name,
        std::process::id(),
        tmp_suffix
    ));

    let write_result = (|| -> Result<()> {
        let mut file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&tmp_path)
            .context(format!(
                "Failed to create temporary config file: {}",
                tmp_path.display()
            ))?;

        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;

            let permissions = std::fs::Permissions::from_mode(0o600);
            file.set_permissions(permissions)
                .context("Failed to set config file permissions")?;
        }

        file.write_all(data.as_bytes())
            .context(format!("Failed to write config file: {}", tmp_path.display()))?;
        file.sync_all()
            .context(format!("Failed to sync config file: {}", tmp_path.display()))?;
        drop(file);

        std::fs::rename(&tmp_path, path)
            .context(format!("Failed to replace config file: {}", path))?;

        if let Some(parent) = target_path.parent() {
            if let Ok(directory) = std::fs::File::open(parent) {
                let _ = directory.sync_all();
            }
        }

        Ok(())
    })();

    if write_result.is_err() {
        let _ = std::fs::remove_file(&tmp_path);
    }

    write_result
}

#[cfg(test)]
mod tests {
    use super::{default_certificate_renew_before, AgentConfig, BootstrapConfig, ConfigManager};
    use std::fs;
    use std::time::Duration;

    #[test]
    fn default_agent_config_sets_certificate_renew_window() {
        let config = AgentConfig::default();
        assert_eq!(config.certificate_renew_before, default_certificate_renew_before());
    }

    #[test]
    fn from_parts_preserves_certificate_renew_window() {
        let bootstrap = BootstrapConfig {
            certificate_renew_before: Duration::from_secs(6 * 60 * 60),
            ..BootstrapConfig::default()
        };
        let config = AgentConfig::from_parts(bootstrap.clone(), Default::default());
        assert_eq!(config.certificate_renew_before, Duration::from_secs(6 * 60 * 60));
        assert_eq!(config.to_bootstrap().certificate_renew_before, bootstrap.certificate_renew_before);
    }

    #[test]
    fn load_parts_falls_back_to_legacy_config_when_state_yaml_is_corrupt() {
        let base = std::env::temp_dir().join(format!(
            "aria-agent-corrupt-state-{}",
            std::process::id()
        ));
        fs::create_dir_all(&base).expect("create temp config dir");
        let bootstrap_path = base.join("agent.yaml");
        let manager = ConfigManager::new(bootstrap_path.to_str().unwrap());
        let legacy = AgentConfig {
            controller_url: "https://controller.example.com:50051".to_string(),
            private_key: "legacy-private".to_string(),
            public_key: "legacy-public".to_string(),
            assigned_ip: Some("100.64.0.2".to_string()),
            ..AgentConfig::default()
        };
        fs::write(
            &bootstrap_path,
            serde_yaml::to_string(&legacy).expect("serialize legacy config"),
        )
        .expect("write legacy config");
        fs::write(manager.state_path(), "node_id: [unterminated").expect("write corrupt state");

        let (_bootstrap, state) = manager
            .load_parts()
            .expect("corrupt state should fall back to legacy runtime config");

        assert_eq!(state.public_key, "legacy-public");
        assert_eq!(state.private_key, "legacy-private");
        assert_eq!(state.assigned_ip.as_deref(), Some("100.64.0.2"));

        let _ = fs::remove_dir_all(base);
    }
}
