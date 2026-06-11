# Control Plane Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add low-risk control-plane foundation work: Controller capability discovery, Sync snapshot completeness metadata, domain version reporting, and append-only audit coverage for critical control changes.

**Architecture:** Phase 1 is additive. It does not introduce ACE tables, Edge Relay, new Agent identity keys, or data-plane changes. Controller exposes a public capability document; gRPC Sync appends backwards-compatible fields; Rust Agent parses and stores those fields; critical mutations write structured audit events using the existing `audit_events` table.

**Tech Stack:** Go HTTP API, Go gRPC/protobuf, Rust tonic Agent client, PostgreSQL-backed controller storage, existing Go/Rust test suites.

**Validation policy:** Do not compile or build release artifacts on the local workstation. Local work may run static checks, formatting, and protobuf generation only. Go/Rust test/build verification must run in GitHub Actions after each pushed batch.

---

## File Map

| Area | Files |
|------|-------|
| Controller capability API | `internal/api/v2/setup.go`, `internal/api/v2/platform.go`, `internal/api/v2/platform_test.go` |
| gRPC protocol | `pkg/grpc/agentpb/aria_agent.proto`, `agent-rust/proto/aria_agent.proto`, generated `pkg/grpc/agentpb/*.pb.go` |
| gRPC server | `internal/controller/grpc/server.go`, `internal/controller/grpc/*_test.go` |
| REST southbound sync | `internal/cli/controller_serve.go`, `internal/cli/controller_sync_test.go` |
| Rust Agent | `agent-rust/agent/src/grpc_client.rs`, `agent-rust/agent/src/agent_runtime.rs`, `agent-rust/tests/test_grpc_sync.rs` |
| Audit storage | `pkg/controllerstorage/audit_events.go`, `pkg/controllerstorage/postgres.go`, related controller tests |
| Docs | `docs/aria-control-plane-synthesis.md` |

## Task 0: Close Required Safety Preconditions

**Files:**
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/controller/grpc/server.go`
- Modify: `internal/cli/controller_certificates_test.go`
- Modify: `internal/controller/grpc/lifecycle_hardening_test.go`

**Status:** completed before Phase 1 implementation. See `docs/code-review-findings.md`: `ENROLL-002`, `GRPC-001/GRPC-002`, and `GRPC-003` are fixed in the current branch history.

- [x] **Step 1: Add or update failing tests for ENROLL-002**

In `internal/cli/controller_certificates_test.go`, replace the current token-consumption expectation with a test that proves enrollment token usage is rolled back or delayed when `SaveNode` fails.

Expected behavior: after a fresh registration fails while saving the node, the enrollment token remains usable or the transaction rolls back both token consumption and node write.

- [x] **Step 2: Run the targeted test in GitHub Actions**

Run:

```bash
go test ./internal/cli -run 'TestHandleRegister.*Token|TestHandleRegister.*Enrollment' -count=1
```

Expected before fix: at least one test fails because token consumption happens before node persistence.

- [x] **Step 3: Fix enrollment token atomicity**

Update registration flow so enrollment token consumption and `SaveNode` are in one transaction or token consumption happens only after node persistence succeeds. Keep existing re-registration checks intact.

- [x] **Step 4: Add or update failing tests for gRPC lifecycle gates**

In `internal/controller/grpc/lifecycle_hardening_test.go`, assert that deleted, suspended, and banned nodes cannot resolve through legacy public-key fallback for metrics or command stream.

- [x] **Step 5: Fix gRPC legacy lifecycle fallback**

Update `internal/controller/grpc/server.go` so `resolveLegacyAgentIdentity` applies the same deleted/suspended/banned status gate as runtime-token resolution, or remove fallback for lifecycle-sensitive paths.

- [x] **Step 6: Verify safety preconditions in GitHub Actions**

Actions validation:

```bash
gh run watch <run-id> --exit-status
```

Expected: branch checks are green before Phase 1 implementation continues.

## Task 1: Add Controller Capability API

**Files:**
- Modify: `internal/api/v2/setup.go`
- Modify: `internal/api/v2/platform.go`
- Test: `internal/api/v2/platform_test.go`

- [ ] **Step 1: Write capability contract test**

Add a test for `GET /api/v2/controller-info` that does not require JWT and returns a stable JSON contract.

Expected JSON shape:

```json
{
  "name": "aria-controller",
  "version": "0.2.x",
  "supported_features": ["grpc_sync", "runtime_token_refresh", "cert_renew"],
  "limits": {
    "max_peers_per_sync": 500,
    "max_acl_rules": 1000
  },
  "auth": {
    "enrollment": true,
    "runtime_token_ttl_sec": 86400,
    "challenge_auth": false
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/api/v2 -run TestControllerInfo -count=1
```

Expected: FAIL because route is not registered.

- [ ] **Step 3: Implement handler**

Add a small handler in `internal/api/v2/platform.go`:

```go
func (r *Router) HandleControllerInfo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		apibase.WriteError(w, http.StatusMethodNotAllowed, apibase.CodeMethodNotAllowed, "Method not allowed", nil)
		return
	}
	apibase.WriteSuccess(w, map[string]interface{}{
		"name":    "aria-controller",
		"version": currentControllerVersion(),
		"supported_features": []string{
			"grpc_sync",
			"runtime_token_refresh",
			"cert_renew",
		},
		"limits": map[string]int{
			"max_peers_per_sync": 500,
			"max_acl_rules":     1000,
		},
		"auth": map[string]interface{}{
			"enrollment":            true,
			"runtime_token_ttl_sec": 86400,
			"challenge_auth":        false,
		},
	}, "Controller capabilities retrieved")
}
```

Use the repo's existing version source if available; otherwise keep a helper that falls back to `0.2.x` until a stable version package exists.

- [ ] **Step 4: Register route**

In `internal/api/v2/setup.go`, add the public route before JWT-protected routes:

```go
mux.HandleFunc("/api/v2/controller-info", router.HandleControllerInfo)
```

- [ ] **Step 5: Verify capability API**

Run:

```bash
go test ./internal/api/v2 -run TestControllerInfo -count=1
```

Expected: PASS.

## Task 2: Extend SyncResponse Protocol

**Files:**
- Modify: `pkg/grpc/agentpb/aria_agent.proto`
- Modify: `agent-rust/proto/aria_agent.proto`
- Regenerate: `pkg/grpc/agentpb/aria_agent.pb.go`
- Regenerate: `pkg/grpc/agentpb/aria_agent_grpc.pb.go` if generator output changes

- [ ] **Step 1: Add backwards-compatible fields**

Append fields to both proto files:

```proto
message SyncResponse {
  repeated PeerInfo peers = 1;
  string assigned_ip = 2;
  int64 last_update = 3;
  repeated ACLRule acl_rules = 4;
  string metrics_push_gateway = 5;
  repeated QoSRule qos_rules = 6;
  repeated BlacklistRule blacklist_rules = 7;
  string desired_state_version = 8;
  string runtime_token = 9;
  int64 runtime_token_expires_at = 10;
  bool snapshot_complete = 11;
  map<string, string> domain_versions = 12;
}
```

- [ ] **Step 2: Regenerate Go protobuf stubs**

Run the repo's established protobuf generation command. If no script exists, use:

```bash
protoc --go_out=. --go-grpc_out=. pkg/grpc/agentpb/aria_agent.proto
```

Expected: Go stubs expose `GetSnapshotComplete()` and `GetDomainVersions()`.

- [ ] **Step 3: Verify generated Go code compiles**

Run:

```bash
go test ./pkg/grpc/agentpb ./internal/controller/grpc -count=1
```

Expected: PASS or only expected test failures unrelated to field generation.

## Task 3: Populate Snapshot and Domain Versions in Controller Sync

**Files:**
- Modify: `internal/controller/grpc/server.go`
- Modify: `internal/cli/controller_serve.go`
- Test: `internal/controller/grpc/runtime_token_sync_test.go`
- Test: `internal/cli/controller_sync_test.go`

- [ ] **Step 1: Add failing gRPC Sync test**

Assert that successful Sync returns:

```go
if !resp.GetSnapshotComplete() {
	t.Fatal("expected snapshot_complete=true")
}
if resp.GetDomainVersions()["acl"] == "" {
	t.Fatal("expected acl domain version")
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./internal/controller/grpc -run Test.*Sync.*Domain -count=1
```

Expected: FAIL because fields are not populated.

- [ ] **Step 3: Implement domain version helper**

Add a helper near Sync response construction:

```go
func domainVersionsFromDesiredVersion(version string) map[string]string {
	if strings.TrimSpace(version) == "" {
		return map[string]string{}
	}
	return map[string]string{
		"peer":        version,
		"acl":         version,
		"qos":         version,
		"route":       version,
		"blacklist":   version,
		"certificate": version,
	}
}
```

This is conservative Phase 1 behavior. Phase 2 replaces it with real per-domain revision.

- [ ] **Step 4: Set fields on successful Sync**

In `internal/controller/grpc/server.go`, set:

```go
SnapshotComplete: true,
DomainVersions:  domainVersionsFromDesiredVersion(desiredVersion),
```

Do not set `snapshot_complete=true` on DB/query failure paths.

- [ ] **Step 5: Mirror REST southbound Sync response**

In `internal/cli/controller_serve.go`, extend `SyncResponse` JSON with:

```go
SnapshotComplete bool              `json:"snapshot_complete"`
DomainVersions  map[string]string `json:"domain_versions,omitempty"`
```

Populate it on successful `syncNode`.

- [ ] **Step 6: Verify controller Sync**

Run:

```bash
go test ./internal/controller/grpc ./internal/cli -run 'Sync|RuntimeToken' -count=1
```

Expected: PASS.

## Task 4: Parse Sync Metadata in Rust Agent

**Files:**
- Modify: `agent-rust/agent/src/grpc_client.rs`
- Modify: `agent-rust/agent/src/agent_runtime.rs`
- Test: `agent-rust/tests/test_grpc_sync.rs`

- [ ] **Step 1: Extend `SyncResult`**

Add fields:

```rust
pub snapshot_complete: bool,
pub domain_versions: std::collections::HashMap<String, String>,
```

- [ ] **Step 2: Map response fields**

In `GrpcClient::sync`, map:

```rust
snapshot_complete: resp.snapshot_complete,
domain_versions: resp.domain_versions,
```

- [ ] **Step 3: Store latest domain versions**

In `agent_runtime`, add a small in-memory field or config-compatible storage for latest domain versions. Phase 1 only records the map; it does not skip full Sync yet.

- [ ] **Step 4: Add Rust regression test**

Create or update a test that builds a `SyncResponse` with `snapshot_complete=true` and a non-empty `domain_versions`, then asserts the parsed `SyncResult` preserves both.

- [ ] **Step 5: Verify Rust Agent**

Run:

```bash
cd agent-rust
cargo test -p aria-agent --lib
```

Expected: PASS.

## Task 5: Expand Append-Only Audit Coverage

**Files:**
- Modify: `pkg/controllerstorage/audit_events.go`
- Modify: `internal/cli/controller_serve.go`
- Modify: `internal/api/v2/setup.go`
- Test: `internal/cli/controller_certificates_test.go`
- Test: `internal/api/v2/policy_dispatch_test.go`

- [ ] **Step 1: Define audit event constants**

In `pkg/controllerstorage/audit_events.go`, add constants:

```go
const (
	AuditNodeRegistered   = "node.registered"
	AuditNodeReregistered = "node.reregistered"
	AuditNodeSuspended    = "node.suspended"
	AuditNodeDeleted      = "node.deleted"
	AuditCertIssued       = "cert.issued"
	AuditCertRevoked      = "cert.revoked"
	AuditPolicyChanged    = "policy.changed"
	AuditCommandQueued    = "command.queued"
	AuditCommandResult    = "command.result"
)
```

- [ ] **Step 2: Add audit assertions for registration and certificate lifecycle**

Extend existing tests to assert `audit_events.event_type` uses the new constants for registration, certificate issue, and certificate revoke.

- [ ] **Step 3: Add policy mutation audit assertion**

In policy dispatch tests, assert ACL/QoS/route mutations write `policy.changed` with `tenant_id`, `node_id`, `policy_domain`, and `desired_state_version` in `detail`.

- [ ] **Step 4: Implement missing audit writes**

Use existing `CreateAuditEvent` and include enough detail for timeline display:

```go
&controllerstorage.AuditEvent{
	TenantID:  tenantID,
	NodeID:    &nodeID,
	EventType: controllerstorage.AuditPolicyChanged,
	Actor:     actor,
	Summary:   "Policy changed",
	Detail: map[string]interface{}{
		"policy_domain":         domain,
		"desired_state_version": desiredVersion,
		"source":                "api.v2",
	},
}
```

- [ ] **Step 5: Verify audit coverage**

Run:

```bash
go test ./internal/cli ./internal/api/v2 ./pkg/controllerstorage -run 'Audit|Policy|Certificate|Register' -count=1
```

Expected: PASS.

## Task 6: Final Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run allowed local static checks**

Run:

```bash
gofmt -w <touched-go-files>
git diff --check
rg -n "controller-info|snapshot_complete|domain_versions|Phase 1" docs/aria-control-plane-synthesis.md docs/superpowers/plans/2026-06-03-control-plane-phase1.md
```

Expected: formatting and whitespace checks pass; docs mention the Phase 1 contract.

- [ ] **Step 2: Commit and push Phase 1 implementation**

Run:

```bash
git add docs/aria-control-plane-synthesis.md docs/superpowers/plans/2026-06-03-control-plane-phase1.md \
  internal/api/v2 internal/cli internal/controller/grpc pkg/controllerstorage pkg/grpc/agentpb agent-rust
git commit -m "feat: add control plane phase 1 foundation"
git push origin codex/control-plane-phase1-prep
```

Expected: branch push starts GitHub Actions.

- [ ] **Step 3: Wait for GitHub Actions**

Run:

```bash
gh run list --branch codex/control-plane-phase1-prep --limit 1
gh run watch <run-id> --exit-status
```

Expected: Go, Rust, frontend, and any repository CI checks are green.

- [ ] **Step 4: Merge and deploy only after green**

Run the repository's existing merge/release path:

```bash
git checkout master
git merge --no-ff codex/control-plane-phase1-prep
git push origin master
gh workflow run <release-or-build-workflow>
```

Expected: master Actions produce deployable Controller/frontend/Agent artifacts before staging or production validation.

## Plan Self-Review

- Spec coverage: Phase 1 covers capability discovery, Sync completeness, domain versions, audit enhancement, and required safety preconditions from the synthesis doc.
- Scope kept out: ACE tables, replaceable desired state migration, challenge auth, signed approval, and Edge Relay remain out of Phase 1.
- Compatibility: gRPC fields are appended only; old Agents ignore new fields, new Agents tolerate absent fields from old Controllers.
