# B5 Frontend Stability and Policy Navigation Bugfix

**Date:** 2026-07-04
**Branch:** `codex/bugfix-b5-frontend-stability`
**Scope:** Frontend stability, input validation, startup failure handling, polling backoff, and policy navigation only; no backend/runtime changes and no deployment in this batch.

## Bugs

- BUG-58: `Routing.vue` used bare `error.message` in catch blocks.
- BUG-59: `IPGroups.vue` used bare `error.message` in catch blocks.
- BUG-60: `Tokens.vue` had an existing `errorMessage()` helper but did not use it in create/revoke failures.
- BUG-61: `main.ts` discarded `bootstrap()` rejections.
- BUG-77: node edit save could partially persist metadata while route sync failed, leaving the edit baseline stale.
- BUG-78: focused polling retried failed requests at a fixed interval with no backoff or stop condition.
- BUG-79: route CIDR regex accepted invalid octets/prefixes.
- BUG-106: Policy Center top IP Groups button passed a `MouseEvent` as the policy argument.

## Fix Summary

- Added a shared `isValidCidrOrIp()` helper and reused it in Routing and Nodes route input validation.
- Replaced unsafe catch-block message extraction in Routing, IPGroups, and Tokens with guarded helpers.
- Changed startup bootstrap to catch rejections, log a deterministic error, and render a startup failure state into `#app`.
- Changed focused polling from fixed `setInterval` to a timeout loop with exponential backoff and a consecutive-failure stop condition.
- When node metadata save succeeds but route sync fails, refresh the node list/detail, reset the edit baseline to backend state, keep the dialog open, and show a partial-save message.
- Changed the Policy Center top IP Groups button to call `goToIpGroups()` without passing the click event and guarded the function against `Event` payloads.

## Validation

```bash
cd frontend
npm run type-check
npm run test:run
npm run build
```

Additional targeted RED/GREEN tests were added for startup rejection handling, polling backoff, CIDR/IP validation, non-`Error` catch values, node partial-save baseline refresh, and Policy Center IP Groups navigation.
