# Platform Backup And Certificate Lifecycle Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the next product-critical closure after the v0.1.0 onboarding/control/operations loops: make Settings/Backup safe enough for production use, then make certificate lifecycle complete enough for managed Agent deployment.

**Architecture:** Settings/Backup is handled first because restore is a high-blast-radius super-admin operation and currently lacks dry-run, selective restore, and stronger confirmation. Certificate lifecycle follows because issue/renew primitives already exist, but registration-time issuance, renewal observability, and lifecycle revocation are not fully closed. Control-loop Phase 1 capability/domain-version work is not repeated here because it already exists in current `master`.

**Tech Stack:** Go Controller API, PostgreSQL storage, Vue 3 + Element Plus frontend, Rust Agent certificate client, existing `audit_events`, `alerts`, `node_certificates`, low-bandwidth Controller/frontend deploy flow for frontend/API-only changes, GitHub Actions for Rust Agent or protocol/runtime-sensitive changes.

---

## Scope Boundary

### In Scope

- Settings/Backup dry-run restore.
- Selective restore by known safe table groups.
- Stronger restore confirmation UX and audit details.
- Certificate registration-time issuance path for nodes that do not yet have a client certificate.
- Agent certificate renewal retry/status reporting.
- Node lifecycle certificate revocation or invalidation on delete, suspend, and ban.
- Certificate status visibility in Nodes and Monitoring using existing node detail surfaces.
- Documentation and version/release discipline.

### Not In Scope

- ACE event envelope or signed replaceable desired-state redesign.
- Domain diff sync that skips full snapshots.
- Multi-node transactional policy rollback UI.
- Hermes AI agent integration.
- Frontend full TypeScript migration of every `.vue` view.

## Target Branch And Release Flow

- Work branch: `codex/platform-backup-cert-closure`.
- Batch rule: complete one batch, commit, push, run CI appropriate to touched surface, then continue.
- Controller/frontend-only batches may use the low-bandwidth deploy path for gray validation.
- Any batch touching Rust Agent, eBPF, proto, certificate runtime behavior, or gRPC southbound behavior must use GitHub Actions before gray validation.
- Before any deployed release, bump `VERSION` to a new patch version. Do not reuse a previously deployed version string.
- Final closure: gray validation -> merge to `master` after confirmation -> master Actions -> deploy master artifacts -> smoke test -> cleanup branch.

## Current Code Map

- Settings API: `internal/api/v2/settings.go`
- Settings tests: `internal/api/v2/settings_test.go`
- Settings frontend API: `frontend/src/composables/useSettingsApi.ts`
- Settings page: `frontend/src/views/Settings.vue`
- API endpoints: `frontend/src/config/api.ts`
- Certificate issue/renew HTTP handlers: `internal/cli/controller_serve.go`
- Certificate storage lifecycle helpers: `pkg/controllerstorage/certificate_lifecycle.go`
- Certificate storage tests: `pkg/controllerstorage/certificate_lifecycle_test.go`
- Certificate service: `internal/security/certissuance/service.go`
- Certificate controller tests: `internal/cli/controller_certificates_test.go`, `internal/cli/controller_register_certificate_test.go`
- Agent certificate client: `agent-rust/agent/src/certificate_client.rs`
- Agent runtime: `agent-rust/agent/src/agent_runtime.rs`
- Node detail certificate display: `frontend/src/views/Nodes.vue`, `frontend/src/stores/node.ts`, `frontend/src/types/monitoring.ts`
- Product docs: `docs/v0.1.0-product-blueprint.md`, `docs/deployment.md`, `docs/cert-auto-issuance-design.md`

---

## Batch 1: Backup Restore Dry-Run Contract

**Outcome:** Restore can be previewed without changing data. The API returns exactly which tables would be touched and row counts. Upload validation and restore validation share the same manifest checks.

**Files:**
- Modify: `internal/api/v2/settings.go`
- Modify: `internal/api/v2/settings_test.go`
- Modify: `frontend/src/config/api.ts`
- Modify: `frontend/src/composables/useSettingsApi.ts`

- [ ] **Step 1: Add dry-run request and response types**

Add types near the existing backup types in `internal/api/v2/settings.go`:

```go
type backupRestoreRequest struct {
	DryRun bool     `json:"dry_run"`
	Tables []string `json:"tables,omitempty"`
	Confirm string  `json:"confirm,omitempty"`
}

type backupRestorePlan struct {
	BackupID       string         `json:"backup_id"`
	DryRun         bool           `json:"dry_run"`
	SelectedTables []string       `json:"selected_tables"`
	TableCounts    map[string]int `json:"table_counts"`
	Warnings       []string       `json:"warnings"`
}
```

- [ ] **Step 2: Write dry-run tests**

Add tests to `internal/api/v2/settings_test.go`:

```go
func TestBackupRestoreDryRunDoesNotModifyDatabase(t *testing.T) {
	// Arrange a backup manifest with tenants and users.
	// POST /api/v2/settings/backups/{id}/restore with {"dry_run":true}.
	// Assert response data.dry_run == true.
	// Assert response data.table_counts contains tenants/users counts.
	// Assert the database row counts are unchanged after the request.
}

func TestBackupRestoreDryRunRejectsUnknownTable(t *testing.T) {
	// POST restore dry-run with {"dry_run":true,"tables":["tenants","unknown_table"]}.
	// Assert HTTP 400 and message includes "unknown restore table".
}
```

Run:

```bash
go test ./internal/api/v2 -run 'TestBackupRestoreDryRun' -count=1
```

Expected: both tests fail before implementation, pass after implementation.

- [ ] **Step 3: Implement restore preview**

Add helpers in `internal/api/v2/settings.go`:

```go
func selectedRestoreTables(requested []string) ([]backupTableSpec, error) {
	if len(requested) == 0 {
		return backupRestoreTables, nil
	}
	allowed := map[string]backupTableSpec{}
	for _, spec := range backupRestoreTables {
		allowed[spec.Name] = spec
	}
	selected := make([]backupTableSpec, 0, len(requested))
	seen := map[string]bool{}
	for _, name := range requested {
		name = strings.TrimSpace(name)
		spec, ok := allowed[name]
		if !ok {
			return nil, fmt.Errorf("unknown restore table: %s", name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		selected = append(selected, spec)
	}
	return selected, nil
}

func buildRestorePlan(manifest *backupManifest, backupID string, selected []backupTableSpec) backupRestorePlan {
	counts := make(map[string]int, len(selected))
	names := make([]string, 0, len(selected))
	for _, spec := range selected {
		names = append(names, spec.Name)
		counts[spec.Name] = len(manifest.Tables[spec.Name])
	}
	warnings := []string{
		"restore replaces selected control-plane configuration tables",
		"active Agent runtime state may need a sync after restore",
	}
	return backupRestorePlan{
		BackupID: backupID,
		DryRun: true,
		SelectedTables: names,
		TableCounts: counts,
		Warnings: warnings,
	}
}
```

- [ ] **Step 4: Wire dry-run into `restoreBackup`**

Parse the request body before applying restore:

```go
var restoreReq backupRestoreRequest
if req.Body != nil {
	_ = json.NewDecoder(req.Body).Decode(&restoreReq)
}
selected, err := selectedRestoreTables(restoreReq.Tables)
if err != nil {
	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, err.Error(), nil)
	return
}
plan := buildRestorePlan(manifest, backupID, selected)
if restoreReq.DryRun {
	apibase.WriteSuccess(w, plan, "Backup restore dry-run completed")
	return
}
```

Do not call `restoreBackupManifest` on dry-run.

- [ ] **Step 5: Add frontend API method**

In `frontend/src/config/api.ts`, add:

```ts
BACKUP_RESTORE_DRY_RUN: (id: string) => `/v2/settings/backups/${id}/restore`
```

In `frontend/src/composables/useSettingsApi.ts`, add:

```ts
restoreBackupDryRun: async (backupId: string, tables: string[] = []) => {
  const response = await api.post(API_ENDPOINTS.SETTINGS.BACKUP_RESTORE_DRY_RUN(backupId), {
    dry_run: true,
    tables
  })
  return response.data?.data || response.data
}
```

- [ ] **Step 6: Verify Batch 1**

Run:

```bash
go test ./internal/api/v2 -run 'TestBackupRestoreDryRun|TestSettings' -count=1
git diff --check
```

Expected: Go tests pass and no whitespace errors.

- [ ] **Step 7: Commit Batch 1**

```bash
git add internal/api/v2/settings.go internal/api/v2/settings_test.go frontend/src/config/api.ts frontend/src/composables/useSettingsApi.ts
git commit -m "feat: add backup restore dry-run"
```

---

## Batch 2: Backup Selective Restore, Confirmation, And UI

**Outcome:** Restore requires a deliberate confirmation phrase, supports selected table groups, and shows a preview before applying. This turns restore from a dangerous one-click operation into a predictable super-admin workflow.

**Files:**
- Modify: `internal/api/v2/settings.go`
- Modify: `internal/api/v2/settings_test.go`
- Modify: `frontend/src/views/Settings.vue`
- Modify: `frontend/src/composables/useSettingsApi.ts`

- [ ] **Step 1: Define confirmation phrase**

Add a constant in `internal/api/v2/settings.go`:

```go
const backupRestoreConfirmPhrase = "RESTORE ARIA CONFIG"
```

- [ ] **Step 2: Write confirmation tests**

Add tests:

```go
func TestBackupRestoreRequiresConfirmationPhrase(t *testing.T) {
	// POST restore with dry_run=false and missing confirm.
	// Assert HTTP 400 and no database rows are changed.
}

func TestBackupRestoreSelectedTablesOnly(t *testing.T) {
	// POST restore with {"confirm":"RESTORE ARIA CONFIG","tables":["ip_groups","ip_group_members"]}.
	// Assert only selected tables are cleaned/restored.
}
```

Run:

```bash
go test ./internal/api/v2 -run 'TestBackupRestoreRequiresConfirmationPhrase|TestBackupRestoreSelectedTablesOnly' -count=1
```

Expected: fail before implementation.

- [ ] **Step 3: Implement selected restore**

Change `restoreBackupManifest` signature:

```go
func (r *Router) restoreBackupManifest(manifest *backupManifest, backupID, actor string, selected []backupTableSpec) (map[string]int, error)
```

Compute cleanup tables from selected restore specs instead of always clearing every table. Keep full restore as the default when no specific tables are requested.

- [ ] **Step 4: Enforce confirmation**

Before applying restore:

```go
if strings.TrimSpace(restoreReq.Confirm) != backupRestoreConfirmPhrase {
	apibase.WriteError(w, http.StatusBadRequest, apibase.CodeBadRequest, "Restore confirmation phrase is required", map[string]string{
		"required_confirm": backupRestoreConfirmPhrase,
	})
	return
}
```

- [ ] **Step 5: Improve audit details**

When restore succeeds, write `settings_backup_restored` audit detail:

```go
Detail: map[string]interface{}{
	"backup_id": backupID,
	"selected_tables": tableNamesFromSpecs(selected),
	"restored_tables": restoredTables,
}
```

- [ ] **Step 6: Add UI preview dialog**

In `frontend/src/views/Settings.vue`, replace direct popconfirm restore with a dialog flow:

```vue
<el-dialog v-model="restoreDialogVisible" title="Restore Backup" width="640px">
  <el-alert type="warning" :closable="false" show-icon>
    <template #default>
      Restore replaces selected control-plane configuration tables. Run dry-run first and type the confirmation phrase before applying.
    </template>
  </el-alert>
  <el-checkbox-group v-model="restoreTables">
    <el-checkbox label="tenants" />
    <el-checkbox label="users" />
    <el-checkbox label="roles" />
    <el-checkbox label="tokens" />
    <el-checkbox label="nodes" />
    <el-checkbox label="ip_groups" />
    <el-checkbox label="ip_group_members" />
    <el-checkbox label="acl_rules" />
    <el-checkbox label="qos_rules" />
    <el-checkbox label="blacklist_rules" />
  </el-checkbox-group>
  <el-input v-model="restoreConfirm" placeholder="RESTORE ARIA CONFIG" />
</el-dialog>
```

Add handlers:

```ts
const openRestoreDialog = async (backup) => {
  selectedBackup.value = backup
  restoreDialogVisible.value = true
  restorePlan.value = await useSettingsApi.restoreBackupDryRun(backup.id, restoreTables.value)
}

const applyRestore = async () => {
  await useSettingsApi.restoreBackup(selectedBackup.value.id, {
    confirm: restoreConfirm.value,
    tables: restoreTables.value
  })
  await loadBackups()
}
```

- [ ] **Step 7: Verify Batch 2**

Run:

```bash
go test ./internal/api/v2 -run 'TestBackupRestore' -count=1
cd frontend && npm run build
git diff --check
```

Expected: Go restore tests pass; frontend build succeeds.

- [ ] **Step 8: Commit Batch 2**

```bash
git add internal/api/v2/settings.go internal/api/v2/settings_test.go frontend/src/views/Settings.vue frontend/src/composables/useSettingsApi.ts
git commit -m "feat: harden backup restore workflow"
```

---

## Batch 3: Certificate Registration-Time Issuance

**Outcome:** A registered Agent can bootstrap a client certificate without manual out-of-band certificate provisioning, using the runtime token identity already bound to that node.

**Files:**
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/cli/controller_register_certificate_test.go`
- Modify: `internal/cli/controller_certificates_test.go`
- Modify: `agent-rust/agent/src/certificate_client.rs`
- Modify: `agent-rust/agent/src/agent_runtime.rs`

- [ ] **Step 1: Write Controller tests**

Add tests:

```go
func TestIssueCertificateRequiresRuntimeToken(t *testing.T) {
	// POST /api/v2/agents/certificates/issue without runtime_token.
	// Assert HTTP 401 and no node_certificates row is inserted.
}

func TestIssueCertificateBindsRuntimeTokenNode(t *testing.T) {
	// Runtime token belongs to node A; request body tries node B.
	// Assert certificate is issued only for node A or request is rejected.
}

func TestIssueCertificateRejectsInactiveNode(t *testing.T) {
	// Node status suspended.
	// Assert HTTP 403 and no certificate row.
}
```

Run:

```bash
go test ./internal/cli -run 'TestIssueCertificate' -count=1
```

Expected: tests fail before implementation if any path is incomplete.

- [ ] **Step 2: Normalize issue and renew response**

Make issue and renew response include:

```json
{
  "status": "success",
  "node_id": "...",
  "serial_number": "...",
  "cert_pem": "...",
  "ca_pem": "...",
  "not_before": 1710000000,
  "not_after": 1712592000,
  "renew_before": 259200
}
```

Do not return private key material from Controller.

- [ ] **Step 3: Add Agent issue client**

In `agent-rust/agent/src/certificate_client.rs`, add:

```rust
pub async fn issue_certificate(
    controller_api_url: &str,
    runtime_token: &str,
    ca_cert_path: &str,
    common_name: &str,
) -> Result<RenewedCertificate> {
    let (csr_pem, private_key_pem) = generate_renewal_request(common_name)?;
    let client = build_http_client(controller_api_url, ca_cert_path)?;
    let endpoint = format!("{}/api/v2/agents/certificates/issue", controller_api_url.trim_end_matches('/'));
    let response = client
        .post(endpoint)
        .json(&RenewRequest {
            runtime_token,
            csr_pem: &csr_pem,
        })
        .send()
        .await
        .context("failed to send certificate issue request")?
        .error_for_status()
        .context("certificate issue request failed")?;
    let payload: RenewResponse = response.json().await.context("failed to decode certificate issue response")?;
    Ok(RenewedCertificate {
        cert_pem: payload.cert_pem,
        ca_pem: payload.ca_pem,
        private_key_pem,
        not_after: UNIX_EPOCH + Duration::from_secs(payload.not_after as u64),
    })
}
```

- [ ] **Step 4: Call issue when cert files are absent**

In Agent startup path, after runtime token is available and before requiring mTLS-only operation:

```rust
if client_cert_path_is_configured && certificate_file_missing {
    let issued = issue_certificate(...).await?;
    write_renewed_certificate_files(ca_cert_path, client_cert_path, client_key_path, &issued)?;
}
```

Failure message must name the missing path and the HTTP endpoint. Do not silently fall back to insecure mode.

- [ ] **Step 5: Verify Batch 3**

Run:

```bash
go test ./internal/cli -run 'TestIssueCertificate|TestRegisterCertificate' -count=1
cd agent-rust && cargo test -p aria-agent certificate_client
git diff --check
```

Expected: Controller tests pass; Rust certificate client tests pass.

- [ ] **Step 6: Commit Batch 3**

```bash
git add internal/cli/controller_serve.go internal/cli/controller_register_certificate_test.go internal/cli/controller_certificates_test.go agent-rust/agent/src/certificate_client.rs agent-rust/agent/src/agent_runtime.rs
git commit -m "feat: issue agent certificates during bootstrap"
```

---

## Batch 4: Certificate Renewal And Failure Visibility

**Outcome:** Agent renews before expiry, Controller records success/failure, Nodes and Monitoring show the same certificate lifecycle state.

**Files:**
- Modify: `agent-rust/agent/src/agent_runtime.rs`
- Modify: `agent-rust/agent/src/certificate_client.rs`
- Modify: `pkg/controllerstorage/certificate_lifecycle.go`
- Modify: `pkg/controllerstorage/certificate_lifecycle_test.go`
- Modify: `internal/api/v2/monitoring.go`
- Modify: `internal/api/v2/nodes_monitoring_behavior_test.go`
- Modify: `frontend/src/stores/node.ts`
- Modify: `frontend/src/views/Nodes.vue`

- [ ] **Step 1: Add Agent renewal state tests**

Add Rust tests:

```rust
#[test]
fn should_renew_when_certificate_is_inside_renew_window() {
    // Create a temporary certificate expiring inside renew_before.
    // Assert should_renew_certificate returns true.
}

#[test]
fn should_not_renew_when_certificate_is_outside_renew_window() {
    // Create a temporary certificate expiring later than renew_before.
    // Assert should_renew_certificate returns false.
}
```

Run:

```bash
cd agent-rust && cargo test -p aria-agent certificate_client
```

- [ ] **Step 2: Persist renewal failure as observed state**

When Agent renewal fails, set sync observation:

```rust
self.set_sync_observation("error", format!("certificate renew failed: {}", err));
```

Ensure the next Sync reports this to Controller through `observed_state` and `observed_message`.

- [ ] **Step 3: Add Controller monitoring tests**

Add tests:

```go
func TestNodeMonitoringIncludesCertificateRenewFailure(t *testing.T) {
	// Insert node certificate plus certificate_renew_failed alert/audit.
	// GET node monitoring detail.
	// Assert certificate_activity.last_renew_failure is present.
}
```

- [ ] **Step 4: Add UI display normalization**

In `frontend/src/stores/node.ts`, normalize:

```ts
certificate: monitorDetail?.certificate || detail.certificate || null,
certificateActivity: monitorDetail?.certificate_activity || detail.certificate_activity || null,
```

In `frontend/src/views/Nodes.vue`, show:

- status
- serial number
- not_after
- last_renewed_at
- last_renew_failure

- [ ] **Step 5: Verify Batch 4**

Run:

```bash
go test ./internal/api/v2 -run 'TestNodeMonitoringIncludesCertificate' -count=1
cd agent-rust && cargo test -p aria-agent certificate_client
git diff --check
```

Expected: tests pass.

- [ ] **Step 6: Commit Batch 4**

```bash
git add agent-rust/agent/src/agent_runtime.rs agent-rust/agent/src/certificate_client.rs pkg/controllerstorage/certificate_lifecycle.go pkg/controllerstorage/certificate_lifecycle_test.go internal/api/v2/monitoring.go internal/api/v2/nodes_monitoring_behavior_test.go frontend/src/stores/node.ts frontend/src/views/Nodes.vue
git commit -m "feat: surface certificate renewal state"
```

---

## Batch 5: Certificate Revocation On Node Lifecycle

**Outcome:** Deleted, suspended, or banned nodes cannot keep active issued certificates in Controller state. Revocation is visible through audit and monitoring.

**Files:**
- Modify: `pkg/controllerstorage/node_lifecycle.go`
- Modify: `pkg/controllerstorage/certificates.go`
- Modify: `pkg/controllerstorage/certificate_lifecycle_test.go`
- Modify: `internal/api/v2/nodes_monitoring_behavior_test.go`
- Modify: `frontend/src/views/Nodes.vue`

- [ ] **Step 1: Write lifecycle tests**

Add tests:

```go
func TestSuspendNodeRevokesIssuedCertificate(t *testing.T) {
	// Insert node with issued certificate.
	// Apply lifecycle transition to suspended.
	// Assert certificate status is revoked and revoke_reason contains "node suspended".
}

func TestBanNodeRevokesIssuedCertificate(t *testing.T) {
	// Insert node with issued certificate.
	// Apply lifecycle transition to banned.
	// Assert certificate status is revoked and revoke_reason contains "node banned".
}
```

- [ ] **Step 2: Reuse existing certificate revoke helper**

Call the existing certificate revocation storage helper from lifecycle transitions. The revoke reason must be deterministic:

```go
fmt.Sprintf("node %s", targetStatus)
```

- [ ] **Step 3: Add audit event**

For each lifecycle-driven revoke, write:

```go
EventType: controllerstorage.AuditCertRevoked
Summary: "Node certificate revoked due to node lifecycle change"
Detail: map[string]interface{}{
	"node_status": targetStatus,
	"reason": fmt.Sprintf("node %s", targetStatus),
}
```

- [ ] **Step 4: Verify Batch 5**

Run:

```bash
go test ./pkg/controllerstorage -run 'Test.*Certificate|Test.*Lifecycle' -count=1
go test ./internal/api/v2 -run 'TestNodeMonitoring' -count=1
git diff --check
```

Expected: lifecycle tests pass and monitoring still compiles.

- [ ] **Step 5: Commit Batch 5**

```bash
git add pkg/controllerstorage/node_lifecycle.go pkg/controllerstorage/certificates.go pkg/controllerstorage/certificate_lifecycle_test.go internal/api/v2/nodes_monitoring_behavior_test.go frontend/src/views/Nodes.vue
git commit -m "feat: revoke certificates on node lifecycle changes"
```

---

## Batch 6: Docs, Version, CI, Gray Validation, And Merge

**Outcome:** The shipped product state is documented, versioned, validated online, and not confused with older control-loop Phase 1 plans.

**Files:**
- Modify: `VERSION`
- Modify: `docs/v0.1.0-product-blueprint.md`
- Modify: `docs/cert-auto-issuance-design.md`
- Modify: `docs/deployment.md`
- Modify: `docs/superpowers/plans/2026-06-28-platform-backup-certificate-closure.md`

- [ ] **Step 1: Bump version**

Increment `VERSION` by one patch before any deployment-intended artifact is published.

- [ ] **Step 2: Update product blueprint**

In `docs/v0.1.0-product-blueprint.md`, update:

- Settings/Backup status from partial to closed.
- Certificate lifecycle status from partial to closed after gray validation.
- Keep AI/Hermes as deferred.
- Keep ACE/domain diff as deferred.

- [ ] **Step 3: Update certificate design doc**

In `docs/cert-auto-issuance-design.md`, document:

- registration-time issue
- renew-before behavior
- runtime token binding
- lifecycle revoke
- user-visible failure states

- [ ] **Step 4: Run full validation**

Because this plan touches Rust Agent and certificate runtime behavior, run full CI or an equivalent Linux builder:

```bash
go test ./...
cd frontend && npm run build
cd ../agent-rust && cargo test --workspace
git diff --check
```

Expected: all selected checks pass. If using GitHub Actions, wait for Go, frontend, and Rust jobs before declaring green.

- [ ] **Step 5: Gray validation checklist**

Validate on the existing online Controller and at least one Agent:

```text
Settings/Backup:
- super_admin can create, list, download, upload, dry-run restore, restore selected tables, delete.
- non-super_admin receives 403 for every backup endpoint.
- restore requires confirmation phrase.
- audit_events includes create/upload/restore/delete details.

Certificate:
- new Agent can register and obtain certificate without manual client certificate provisioning.
- existing Agent renews certificate before renew window when forced with a short-validity test cert.
- renew failure appears in node detail and Monitoring.
- suspend/delete/banned node revokes certificate in Controller state.

Smoke:
- login
- tenant switch
- Nodes list/detail
- Settings Backup
- command stream health_check
- ACL/QoS/Route existing policy delivery still applied
```

- [ ] **Step 6: Merge and deploy**

Use the established closure routine:

```bash
git push origin codex/platform-backup-cert-closure
# wait for branch CI
git checkout master
git pull --ff-only origin master
git merge --ff-only codex/platform-backup-cert-closure
git push origin master
# wait for master CI
# deploy master artifacts
```

- [ ] **Step 7: Final record**

Append final evidence to this plan:

```text
Completed version:
Branch CI:
Master CI:
Controller deployed commit:
Frontend deployed commit:
Agent deployed commit:
Backup gray result:
Certificate gray result:
Smoke result:
```

---

## Recommended Execution Order

1. Batch 1: Backup dry-run.
2. Batch 2: Backup selective restore and confirmation UI.
3. Batch 3: Certificate registration-time issue.
4. Batch 4: Certificate renewal and failure visibility.
5. Batch 5: Certificate lifecycle revocation.
6. Batch 6: docs, version, CI, gray validation, merge, deploy.

This order keeps the highest-blast-radius operation first, then moves into runtime-sensitive certificate work with stronger tests and CI coverage.

## Self-Review

- Spec coverage: covers Settings/Backup dry-run, selective restore, confirmation, audit, certificate issue, renew, revoke, visibility, docs, and release flow.
- Placeholder scan: no step depends on an unspecified future design; deferred items are explicitly out of scope.
- Type consistency: restore request/plan types are used consistently across backend and frontend API. Certificate issue reuses the existing renewal response shape and `RenewedCertificate` struct to avoid duplicating Agent certificate file-writing logic.
