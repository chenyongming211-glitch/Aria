# B6 CI/CD and Deployment Guardrails Bugfix

**Date:** 2026-07-04
**Branch:** `codex/bugfix-b6-delivery-guardrails`
**Scope:** GitHub Actions artifact/publish gates and Ansible deployment guardrails only; no runtime code changes and no online deployment in this batch.

## Bugs

- BUG-107: required GitHub artifact uploads could be hidden by `continue-on-error: true`.
- BUG-108: Docker publish only depended on Go build and allowed manual publish from arbitrary branches.
- BUG-109: Controller Ansible defaults pinned a stale `0.2.35-test` image.
- BUG-110: Agent Ansible used stale source path, binary name, and service name.

## Fix Summary

- Removed `continue-on-error: true` from Go, Rust Agent, and frontend artifact uploads.
- Changed Docker publish to wait for Go, Rust Agent, and frontend jobs.
- Restricted Docker publish to manual `workflow_dispatch` runs from `master` or `v*` release tags.
- Changed Controller Ansible defaults to use `ARIA_CONTROLLER_VERSION`, `ARIA_CONTROLLER_IMAGE`, or the repository `VERSION` instead of a stale hard-coded tag.
- Changed Agent Ansible to deploy a prebuilt `aria-agent` artifact to `/usr/local/bin/aria-agent` and restart the `aria-agent` service.
- Added Go static guardrail tests so CI fails if these delivery protections regress.

## Validation

```bash
go test ./internal/delivery -count=1
ruby -e "require 'yaml'; YAML.load_file('.github/workflows/build.yml'); YAML.load_file('deployments/ansible/playbooks/deploy-controller.yml'); YAML.load_file('deployments/ansible/playbooks/deploy-agent.yml')"
ansible-playbook --syntax-check -i localhost, deployments/ansible/playbooks/deploy-controller.yml
ansible-playbook --syntax-check -i localhost, deployments/ansible/playbooks/deploy-agent.yml
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/build.yml
go test ./... -count=1
git diff --check -- .github/workflows/build.yml deployments/ansible docs/confirmed-bugs.md docs/superpowers/plans/2026-07-03-open-bugfix-batches.md docs/superpowers/plans/2026-07-04-b6-delivery-guardrails.md internal/delivery/ci_cd_guardrails_test.go
```

Branch CI must still pass before merge because this batch intentionally changes GitHub Actions behavior.
