use anyhow::{Context, Result};
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use std::time::Duration;

const DEFAULT_BOOTSTRAP_PATH: &str = "/etc/aria/agent.yaml";
const DEFAULT_STATE_PATH: &str = "/var/lib/aria/agent-state.yaml";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BootstrapConfig {
    #[serde(default)]
    pub controller_url: String,
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
    pub last_applied_version: Option<String>,
    #[serde(default)]
    pub last_sync_status: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentConfig {
    #[serde(default)]
    pub controller_url: String,
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
    #[serde(default = "default_multi_tunnel")]
    pub multi_tunnel: bool,
    #[serde(default)]
    pub current_credential: Option<String>,
    #[serde(default)]
    pub last_applied_version: Option<String>,
    #[serde(default)]
    pub last_sync_status: Option<String>,
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
            ca_cert: String::new(),
            client_cert: String::new(),
            client_key: String::new(),
            tls_server_name: None,
            enrollment_token: None,
            interface_name: default_interface(),
            listen_port: default_listen_port(),
            mtu: default_mtu(),
            region: None,
            customer_id: None,
            advertised_routes: None,
            hostname: None,
            sync_interval: default_sync_interval(),
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
            || self.last_applied_version.is_some()
            || self.last_sync_status.is_some()
    }
}

impl AgentConfig {
    pub fn from_parts(bootstrap: BootstrapConfig, state: AgentState) -> Self {
        Self {
            controller_url: bootstrap.controller_url,
            ca_cert: bootstrap.ca_cert,
            client_cert: bootstrap.client_cert,
            client_key: bootstrap.client_key,
            tls_server_name: bootstrap.tls_server_name,
            enrollment_token: bootstrap.enrollment_token,
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
            multi_tunnel: bootstrap.multi_tunnel,
            current_credential: state.current_credential,
            last_applied_version: state.last_applied_version,
            last_sync_status: state.last_sync_status,
        }
    }

    pub fn to_bootstrap(&self) -> BootstrapConfig {
        BootstrapConfig {
            controller_url: self.controller_url.clone(),
            ca_cert: self.ca_cert.clone(),
            client_cert: self.client_cert.clone(),
            client_key: self.client_key.clone(),
            tls_server_name: self.tls_server_name.clone(),
            enrollment_token: self.enrollment_token.clone(),
            interface_name: self.interface_name.clone(),
            listen_port: self.listen_port,
            mtu: self.mtu,
            region: self.region.clone(),
            customer_id: self.customer_id.clone(),
            advertised_routes: self.advertised_routes.clone(),
            hostname: self.hostname.clone(),
            sync_interval: self.sync_interval,
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
            last_applied_version: self.last_applied_version.clone(),
            last_sync_status: self.last_sync_status.clone(),
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

    pub fn load_or_init(&self, force: bool) -> Result<Option<AgentConfig>> {
        if force {
            return Ok(None);
        }

        if !Path::new(&self.bootstrap_path).exists() {
            return Ok(None);
        }

        self.load().map(Some)
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

    pub fn load_state(&self) -> Result<AgentState> {
        self.load_or_migrate_state()
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

    pub fn save(&self, config: &AgentConfig) -> Result<()> {
        self.save_bootstrap(&config.to_bootstrap())?;
        self.save_state(&config.to_state())
    }

    fn load_or_migrate_state(&self) -> Result<AgentState> {
        if let Some(state) = self.load_state_opt()? {
            return Ok(state);
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

        Ok(serde_yaml::from_str::<AgentConfig>(&data).ok())
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
    if let Some(parent) = Path::new(path).parent() {
        std::fs::create_dir_all(parent)
            .context("Failed to create config directory")?;
    }

    let data = serde_yaml::to_string(payload)
        .context("Failed to serialize config")?;

    std::fs::write(path, data)
        .context(format!("Failed to write config file: {}", path))?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;

        let permissions = std::fs::Permissions::from_mode(0o600);
        std::fs::set_permissions(path, permissions)
            .context("Failed to set config file permissions")?;
    }

    Ok(())
}
