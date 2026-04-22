use anyhow::{Context, Result};
use rcgen::{CertificateParams, DistinguishedName, DnType, KeyPair, PKCS_ECDSA_P256_SHA256};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use url::Url;
use x509_parser::parse_x509_certificate;
use x509_parser::pem::parse_x509_pem;

#[derive(Debug, Clone)]
pub struct RenewedCertificate {
    pub cert_pem: String,
    pub ca_pem: String,
    pub private_key_pem: String,
    pub not_after: SystemTime,
}

#[derive(Debug, Deserialize)]
struct RenewResponse {
    cert_pem: String,
    ca_pem: String,
    not_after: i64,
}

#[derive(Debug, Serialize)]
struct RenewRequest<'a> {
    runtime_token: &'a str,
    csr_pem: &'a str,
}

pub fn resolve_controller_api_url(controller_url: &str, override_url: Option<&str>) -> Result<String> {
    if let Some(url) = override_url.map(str::trim).filter(|value| !value.is_empty()) {
        return Ok(url.trim_end_matches('/').to_string());
    }

    let mut api_url = Url::parse(controller_url)
        .with_context(|| format!("invalid controller_url: {}", controller_url))?;
    if api_url.host_str().is_none() {
        anyhow::bail!("controller_url missing host");
    }
    api_url
        .set_port(Some(8080))
        .map_err(|_| anyhow::anyhow!("failed to set controller_api_url port"))?;
    api_url.set_path("");
    api_url.set_query(None);
    api_url.set_fragment(None);
    Ok(api_url.to_string().trim_end_matches('/').to_string())
}

pub fn should_renew_certificate(cert_path: &str, renew_before: Duration) -> Result<bool> {
    let not_after = load_certificate_not_after(cert_path)?;
    let now = SystemTime::now();
    if now >= not_after {
        return Ok(true);
    }

    let remaining = not_after
        .duration_since(now)
        .unwrap_or_else(|_| Duration::from_secs(0));
    Ok(remaining <= renew_before)
}

pub fn load_certificate_not_after(cert_path: &str) -> Result<SystemTime> {
    let pem = fs::read(cert_path)
        .with_context(|| format!("failed to read certificate file: {}", cert_path))?;
    let (_, pem_block) = parse_x509_pem(&pem)
        .map_err(|err| anyhow::anyhow!("failed to parse certificate PEM {}: {:?}", cert_path, err))?;
    let (_, cert) = parse_x509_certificate(&pem_block.contents)
        .map_err(|err| anyhow::anyhow!("failed to parse certificate DER {}: {:?}", cert_path, err))?;

    let timestamp = cert.validity().not_after.timestamp();
    if timestamp <= 0 {
        anyhow::bail!("certificate has invalid not_after timestamp: {}", timestamp);
    }

    Ok(UNIX_EPOCH + Duration::from_secs(timestamp as u64))
}

pub async fn renew_certificate(
    controller_api_url: &str,
    runtime_token: &str,
    ca_cert_path: &str,
    common_name: &str,
) -> Result<RenewedCertificate> {
    let (csr_pem, private_key_pem) = generate_renewal_request(common_name)?;
    let client = build_http_client(controller_api_url, ca_cert_path)?;
    let endpoint = format!("{}/api/v2/agents/certificates/renew", controller_api_url.trim_end_matches('/'));
    let response = client
        .post(endpoint)
        .json(&RenewRequest {
            runtime_token,
            csr_pem: &csr_pem,
        })
        .send()
        .await
        .context("failed to send certificate renew request")?
        .error_for_status()
        .context("certificate renew request failed")?;

    let payload: RenewResponse = response
        .json()
        .await
        .context("failed to decode certificate renew response")?;
    if payload.not_after <= 0 {
        anyhow::bail!("controller returned invalid not_after for renewed certificate");
    }

    Ok(RenewedCertificate {
        cert_pem: payload.cert_pem,
        ca_pem: payload.ca_pem,
        private_key_pem,
        not_after: UNIX_EPOCH + Duration::from_secs(payload.not_after as u64),
    })
}

pub fn write_renewed_certificate_files(
    ca_cert_path: &str,
    client_cert_path: &str,
    client_key_path: &str,
    renewed: &RenewedCertificate,
) -> Result<()> {
    write_atomic_file(ca_cert_path, renewed.ca_pem.as_bytes(), 0o600)?;
    write_atomic_file(client_cert_path, renewed.cert_pem.as_bytes(), 0o600)?;
    write_atomic_file(client_key_path, renewed.private_key_pem.as_bytes(), 0o600)?;
    Ok(())
}

fn generate_renewal_request(common_name: &str) -> Result<(String, String)> {
    let key_pair = KeyPair::generate_for(&PKCS_ECDSA_P256_SHA256)
        .context("failed to generate private key for certificate renewal")?;
    let mut params = CertificateParams::new(Vec::new())
        .context("failed to create CSR parameters")?;
    let mut dn = DistinguishedName::new();
    dn.push(DnType::CommonName, common_name);
    params.distinguished_name = dn;
    let csr = params
        .serialize_request(&key_pair)
        .context("failed to generate CSR for certificate renewal")?;
    let csr_pem = csr.pem().context("failed to serialize CSR to PEM")?;
    let private_key_pem = key_pair.serialize_pem();
    Ok((csr_pem, private_key_pem))
}

fn build_http_client(controller_api_url: &str, ca_cert_path: &str) -> Result<Client> {
    let mut builder = Client::builder();
    if controller_api_url.starts_with("https://") && !ca_cert_path.trim().is_empty() {
        let ca_pem = fs::read(ca_cert_path)
            .with_context(|| format!("failed to read CA certificate file: {}", ca_cert_path))?;
        let ca = reqwest::Certificate::from_pem(&ca_pem)
            .context("failed to parse CA certificate for HTTPS renew client")?;
        builder = builder.add_root_certificate(ca);
    }
    builder.build().context("failed to build certificate renew HTTP client")
}

fn write_atomic_file(path: &str, data: &[u8], mode: u32) -> Result<()> {
    let file_path = Path::new(path);
    let parent = file_path
        .parent()
        .with_context(|| format!("path has no parent directory: {}", path))?;
    fs::create_dir_all(parent)
        .with_context(|| format!("failed to create directory: {}", parent.display()))?;

    let file_name = file_path
        .file_name()
        .and_then(|value| value.to_str())
        .with_context(|| format!("path has no file name: {}", path))?;
    let tmp_path: PathBuf = parent.join(format!(".{}.tmp", file_name));
    fs::write(&tmp_path, data)
        .with_context(|| format!("failed to write temp certificate file: {}", tmp_path.display()))?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&tmp_path, fs::Permissions::from_mode(mode))
            .with_context(|| format!("failed to set permissions on {}", tmp_path.display()))?;
    }

    fs::rename(&tmp_path, file_path)
        .with_context(|| format!("failed to move renewed certificate file into place: {}", path))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::resolve_controller_api_url;

    #[test]
    fn uses_override_when_present() {
        let resolved = resolve_controller_api_url(
            "https://controller.example.com:50051",
            Some("https://api.example.com:8443"),
        )
        .expect("resolve_controller_api_url should succeed");
        assert_eq!(resolved, "https://api.example.com:8443");
    }

    #[test]
    fn derives_http_api_port_from_grpc_url() {
        let resolved = resolve_controller_api_url(
            "https://controller.example.com:50051",
            None,
        )
        .expect("resolve_controller_api_url should succeed");
        assert_eq!(resolved, "https://controller.example.com:8080");
    }
}
