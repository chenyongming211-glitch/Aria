#[cfg(test)]
mod tests {
    use super::RuntimeCredentialStore;

    #[tokio::test]
    async fn snapshot_reflects_latest_runtime_token_after_sync_update() {
        let store = RuntimeCredentialStore::new(Some("old-token".to_string()));

        assert_eq!(store.snapshot().await.as_deref(), Some("old-token"));

        store.update(Some("new-token".to_string())).await;

        assert_eq!(store.snapshot().await.as_deref(), Some("new-token"));
    }
}
