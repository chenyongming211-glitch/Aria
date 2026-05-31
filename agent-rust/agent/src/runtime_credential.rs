use std::sync::Arc;
use tokio::sync::Mutex;

#[derive(Clone, Debug)]
pub struct RuntimeCredentialStore {
    credential: Arc<Mutex<Option<String>>>,
}

impl RuntimeCredentialStore {
    pub fn new(initial: Option<String>) -> Self {
        Self {
            credential: Arc::new(Mutex::new(normalize_credential(initial))),
        }
    }

    pub async fn snapshot(&self) -> Option<String> {
        self.credential.lock().await.clone()
    }

    pub async fn update(&self, credential: Option<String>) {
        if let Some(credential) = normalize_credential(credential) {
            *self.credential.lock().await = Some(credential);
        }
    }
}

fn normalize_credential(credential: Option<String>) -> Option<String> {
    credential.and_then(|value| {
        let trimmed = value.trim();
        if trimmed.is_empty() {
            None
        } else {
            Some(trimmed.to_string())
        }
    })
}

#[cfg(test)]
mod tests {
    use super::RuntimeCredentialStore;

    #[tokio::test]
    async fn snapshot_reflects_latest_runtime_token_after_sync_update() {
        let store = RuntimeCredentialStore::new(Some("old-token".to_string()));
        let stream_task_store = store.clone();

        assert_eq!(stream_task_store.snapshot().await.as_deref(), Some("old-token"));

        store.update(Some("new-token".to_string())).await;

        assert_eq!(stream_task_store.snapshot().await.as_deref(), Some("new-token"));
    }

    #[tokio::test]
    async fn blank_runtime_token_update_keeps_existing_token() {
        let store = RuntimeCredentialStore::new(Some("old-token".to_string()));

        store.update(Some("   ".to_string())).await;
        store.update(None).await;

        assert_eq!(store.snapshot().await.as_deref(), Some("old-token"));
    }
}
