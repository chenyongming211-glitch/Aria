use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::Duration;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentConfig {
    pub controller_url: String,
    pub ca_cert: String,
    pub client_cert: String,
    pub client_key: String,
    pub device_id: Option<String>,
    pub private_key: String,
    pub public_key: String,
    pub assigned_ip: Option<String>,
    pub address: Option<String>,
    #[serde(default = "default_interface")]
    pub interface_name: String,
    pub listen_port: u16,
    pub mtu: u32,
    pub region: Option<String>,
    pub customer_id: Option<String>,
    pub advertised_routes: Option<Vec<String>>,
    pub hostname: Option<String>,
    #[serde(default = "default_sync_interval", with = "serde_duration")]
    pub sync_interval: Duration,
    #[serde(default = "default_multi_tunnel")]
    pub multi_tunnel: bool,
}

fn default_interface() -> String {
    "aria0".to_string()
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

impl Default for AgentConfig {
    fn default() -> Self {
        Self {
            controller_url: String::new(),
            ca_cert: String::new(),
            client_cert: String::new(),
            client_key: String::new(),
            device_id: None,
            private_key: String::new(),
            public_key: String::new(),
            assigned_ip: None,
            address: None,
            interface_name: "aria0".to_string(),
            listen_port: 51820,
            mtu: 1360,
            region: None,
            customer_id: None,
            advertised_routes: None,
            hostname: None,
            sync_interval: Duration::from_secs(5),
            multi_tunnel: true,
        }
    }
}

pub struct ConfigManager {
    config_path: String,
}

impl ConfigManager {
    pub fn new(config_path: &str) -> Self {
        Self {
            config_path: config_path.to_string(),
        }
    }

    pub fn load_or_init(&self, force: bool) -> Result<Option<AgentConfig>> {
        if !Path::new(&self.config_path).exists() {
            return Ok(None);
        }

        if force {
            return Ok(None);
        }

        let config = self.load()?;

        if config.device_id.is_none() {
            return Ok(None);
        }

        Ok(Some(config))
    }

    pub fn load(&self) -> Result<AgentConfig> {
        let data = std::fs::read_to_string(&self.config_path)
            .context(format!("Failed to read config file: {}", self.config_path))?;

        let config: AgentConfig = serde_yaml::from_str(&data)
            .context("Failed to parse config YAML")?;

        Ok(config)
    }

    pub fn save(&self, config: &AgentConfig) -> Result<()> {
        if let Some(parent) = Path::new(&self.config_path).parent() {
            std::fs::create_dir_all(parent)
                .context("Failed to create config directory")?;
        }

        let data = serde_yaml::to_string(config)
            .context("Failed to serialize config")?;

        std::fs::write(&self.config_path, data)
            .context("Failed to write config file")?;

        Ok(())
    }
}
