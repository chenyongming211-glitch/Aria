# Nodes Workbench Closure Design

## Scope

This phase turns the existing Nodes detail dialog into the first-stop operations workbench for a single node. The backend already exposes the required monitoring detail payload; this work keeps the scope focused on mapping and presenting that payload, improving quick command feedback, and linking directly into Monitoring and Policy Center.

## User Flow

1. An operator opens `Nodes`, selects a node, and sees online status, desired/applied versions, observed state, last sync, pending commands, and active alerts.
2. The operator runs `sync` or `health_check` from the node detail dialog.
3. The command appears immediately as `pending`, then the detail view refreshes so later states such as `sent`, `acknowledged`, `completed`, or `failed` are visible.
4. Recent policy deliveries show domain, policy reference/name, command id, delivery state, last error, and update time.
5. Active alerts and certificate status are visible in the same node detail context, with buttons to open Monitoring or Policy Center preserving the relevant node context.

## Architecture

The backend stays mostly unchanged because `GET /api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}` already returns recent commands, recent policy deliveries, active alerts, and certificate fields. The frontend `node` Pinia store becomes the normalization boundary for node workbench data. `Nodes.vue` remains the UI surface and uses normalized fields rather than reaching into raw API payloads.

The node detail loader should tolerate partial failures. If the status or command-history endpoint fails while monitoring detail succeeds, the dialog should still open with monitoring, certificate, alert, and policy delivery data.

## UI Design

The detail dialog uses dense operational panels, not marketing cards. Add a compact operations summary above the detailed sections, then show state convergence, certificate status, commands, alerts, and policy deliveries. Keep tables bounded, use tags for status, and keep command ids short in the primary view with tooltips for full ids.

## Testing

Frontend unit coverage should prove that:

- `loadNodeDetail()` maps monitoring detail certificate fields into the normalized node model.
- `loadNodeDetail()` still returns a useful node if auxiliary status or command calls fail.
- `Nodes.vue` quick commands prepend the queued command to recent commands immediately and preserve node context links to Monitoring and Policy Center.

Build and test execution that compiles code must run in GitHub Actions, not locally.
