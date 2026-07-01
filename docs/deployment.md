# Aria Deployment

## Current Production Shape

Controller host: `8.152.163.101`

Production runs Aria in isolated containers under `/root/aria-controller`.
The server also hosts other products, so Aria must keep its own containers,
ports, volumes, and network.

| Service | Container | Image | Ports |
| --- | --- | --- | --- |
| Frontend | `aria-frontend` | `nginx:1.27-alpine` | `80:80`, `443:443` |
| Controller | `aria-controller` | `ghcr.io/chenyongming211-glitch/aria-controller:<VERSION>` | `50051:50051` |
| Postgres | `aria-postgres` | `postgres:16-alpine` | `127.0.0.1:15432:5432` |
| Redis | `aria-redis` | `redis:7-alpine` | `127.0.0.1:16379:6379` |
| VictoriaMetrics | `aria-victoriametrics` | `victoriametrics/victoria-metrics:latest` | `127.0.0.1:18428:8428` |

Public HTTPS is terminated by the host Nginx at `https://aria.yun`.
The frontend container serves files from `/root/aria-controller/frontend/dist`.
For low-bandwidth deployments, build the Controller linux/amd64 binary and the
frontend dist locally on the Mac workstation, upload only those small artifacts
to the x86 Linux Controller host, and let the host assemble the runtime
Controller image locally.
The frontend Nginx config must serve `index.html` with `Cache-Control: no-store`
so browsers do not keep an old Vite entrypoint after a deploy. Hashed assets
under `/assets/` can use long immutable caching.

## Release Flow

Default to the low-bandwidth local artifact flow for Controller and frontend
deployments. Use GitHub Actions only when Rust Agent/eBPF/protobuf compatibility
is affected, or as the final pre-release validation gate.

1. Bump the root `VERSION` file for every shipped change.
2. Run local Go tests and frontend tests/build.
3. Cross-compile the Controller and `ariactl` on macOS for `linux/amd64`.
4. Build the frontend dist locally.
5. Upload only the Controller binary, `ariactl`, frontend dist, and runtime
   Dockerfiles to the x86 Linux host.
6. On the host, build the Controller image from the small
   `/root/aria-controller/runtime-build` context. The runtime base image is
   built only when Alpine or runtime packages change; normal Controller deploys
   do not run `apk add`.
7. Restart `aria-controller` and `aria-frontend` through Docker Compose.
8. Run smoke checks.
9. If any change touches Rust Agent, eBPF, Agent protobuf, gRPC southbound
   contracts, policy snapshot shape, ACL/QoS dataplane payloads, or Agent
   lifecycle/runtime behavior, push the branch and run GitHub Actions for Rust
   Agent verification before production rollout.
10. For formal releases, keep one full GitHub Actions run as the final gate even
    when the deployed Controller/frontend artifacts were produced locally.

Useful commands:

```bash
VERSION=$(cat VERSION)
COMMIT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-X aria/internal/cli.Version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}"

mkdir -p dist/controller dist/frontend

go test ./...

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="${LDFLAGS}" \
  -o dist/controller/aria \
  ./cmd

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -o dist/controller/ariactl \
  ./cmd/ariactl

cd frontend
npm run test:run
npm run build -- --outDir ../dist/frontend --emptyOutDir
```

Use GitHub Actions when Rust Agent/eBPF verification is required:

```bash
gh workflow run Build --repo chenyongming211-glitch/Aria --ref <branch>
```

When a release includes Agent onboarding support, upload the Linux amd64 Agent
artifact produced by GitHub Actions or a Linux builder to the Controller host:

```bash
ssh root@8.152.163.101 'mkdir -p /root/aria-controller/artifacts'
rsync -az --progress aria-agent root@8.152.163.101:/root/aria-controller/artifacts/aria-agent-linux-amd64
ssh root@8.152.163.101 'cd /root/aria-controller/artifacts && sha256sum aria-agent-linux-amd64 > aria-agent-linux-amd64.sha256'
```

Build the published Agent artifact on the pinned Ubuntu 22.04 GitHub runner, not
on `ubuntu-latest`. The installer is intended to work on Ubuntu 22.04 or newer;
using a newer runner can produce a binary that requires a newer glibc than common
LTS nodes provide.

The Controller serves this artifact from
`/api/v2/downloads/aria-agent/linux/amd64` for the Nodes onboarding installer.
If the artifact is missing, the download endpoint returns `404` and the
installer exits before consuming the enrollment token.

The Nodes onboarding command should use the Controller-served installer:

```bash
curl -fsSL https://aria.yun/api/v2/install/agent.sh | sudo bash -s -- \
  --controller-api-url https://aria.yun \
  --server https://aria.yun:50051 \
  --token tk_xxx \
  --ca-url https://aria.yun/api/v2/controller-info/grpc-ca.crt \
  --ca-sha256 <sha256> \
  --tls-server-name aria.yun \
  --region tencent-cloud \
  --interface aria0 \
  --public-ip auto \
  --public-endpoint auto
```

After installation, the expected local checks on the Agent host are:

```bash
sudo aria-agent doctor --config /etc/aria/agent.yaml
sudo systemctl status aria-agent --no-pager
sudo journalctl -u aria-agent -n 120 --no-pager
```

## Server Layout

```text
/root/aria-controller/
├── docker-compose.yml
├── .env
├── bin/
│   └── ariactl
├── artifacts/
│   ├── aria-agent-linux-amd64
│   └── aria-agent-linux-amd64.sha256
├── runtime-build/
│   ├── Dockerfile.controller.runtime
│   ├── Dockerfile.controller.runtime-base
│   └── bin/
│       └── aria
├── config/
│   └── controller.yaml
├── certs/
│   ├── ca.crt
│   ├── ca.key
│   ├── grpc-server.crt
│   └── grpc-server.key
├── data/backups/
├── frontend/
│   ├── dist/
│   └── nginx.conf
└── logs/
```

Controller volumes:

```yaml
- ./config/controller.yaml:/etc/aria/controller.yaml:ro
- ./certs:/etc/aria/certs:ro
- ./logs:/var/log/aria
- ./data/backups:/opt/aria/data/backups
- ./artifacts:/root/aria-controller/artifacts:ro
```

## gRPC TLS

Production Agent configuration uses `https://aria.yun:50051`, the Controller CA,
and `tls_server_name: aria.yun`. The Controller must therefore run in one-way TLS
mode:

```bash
ARIA_GRPC_TLS_MODE=server
ARIA_GRPC_SERVER_CERT=/etc/aria/certs/grpc-server.crt
ARIA_GRPC_SERVER_KEY=/etc/aria/certs/grpc-server.key
ARIA_GRPC_CA_CERT=/etc/aria/certs/ca.crt
ARIA_GRPC_TLS_SERVER_NAME=aria.yun
```

`disabled` is only for local or one-off plaintext testing. Do not use it on the
production Controller while Agents are configured with `https://`.

New Agent nodes must install the same Controller CA before `aria-agent up`.
The preferred path is the Nodes onboarding installer above. Manual init remains
only a fallback for diagnostics:

```bash
sudo install -d -m 0755 /etc/aria/certs
sudo install -m 0600 ca.crt /etc/aria/certs/ca.crt
sudo aria-agent init \
  --server https://aria.yun:50051 \
  --controller-api-url https://aria.yun \
  --token tk_xxx \
  --ca-cert /etc/aria/certs/ca.crt \
  --tls-server-name aria.yun \
  --interface aria0
```

If `aria-agent up` logs `invalid peer certificate: UnknownIssuer`, the CA file is
missing or `--ca-cert` points to the wrong path.

Known onboarding failure modes:

| Failure | Expected operator signal |
| --- | --- |
| Missing Agent artifact | `/api/v2/downloads/aria-agent/linux/amd64` returns `404`; installer exits before init |
| Bad CA URL | installer exits during CA download |
| Wrong CA checksum | installer exits during SHA256 verification |
| Missing CA file in manual mode | `aria-agent init` or `aria-agent doctor` reports `CA certificate file not found` |
| Missing TLS server name with CA | `aria-agent init` or `aria-agent doctor` reports `--tls-server-name is required when --ca-cert is set` |
| systemd start failed | installer prints `systemctl status aria-agent --no-pager` and recent journal lines |

## Super Admin Bootstrap

The Controller uses these environment variables for the initial platform admin:

```bash
ARIA_SUPER_ADMIN=sysadmin
ARIA_SUPER_ADMIN_PASSWORD=<bootstrap-password>
```

`ARIA_SUPER_ADMIN_PASSWORD` is a bootstrap secret. On a fresh database it creates
the initial `super_admin` user and marks the account for first-login password
change.

For an existing `super_admin`, the Controller does not overwrite the database
password on normal restart. This prevents an operator's changed password from
being silently reset back to the bootstrap value in `.env`.

To intentionally force the configured username/password back into the database,
set this only for the maintenance window where the reset is needed:

```bash
ARIA_SUPER_ADMIN_SYNC=true
```

Remove or unset `ARIA_SUPER_ADMIN_SYNC` after the reset so future restarts do not
rewrite the admin password again.

## Controller Deploy

Before deployment:

```bash
ssh root@8.152.163.101
cd /root/aria-controller
docker exec aria-postgres pg_dump -U aria -d aria > backups/pre-controller-deploy-$(date +%Y%m%d-%H%M%S).sql
tar -C /root -czf backups/pre-controller-deploy-config-$(date +%Y%m%d-%H%M%S).tar.gz aria-controller/config aria-controller/certs aria-controller/.env aria-controller/secrets
```

Upload the locally built Controller artifacts:

```bash
ssh root@<controller-host> 'mkdir -p /root/aria-controller/runtime-build/bin /root/aria-controller/bin'
rsync -az --progress dist/controller/aria root@<controller-host>:/root/aria-controller/runtime-build/bin/aria
rsync -az --progress dist/controller/ariactl root@<controller-host>:/root/aria-controller/bin/ariactl
rsync -az --progress Dockerfile.controller.runtime Dockerfile.controller.runtime-base root@<controller-host>:/root/aria-controller/runtime-build/
```

The runtime base Dockerfile is tracked at `Dockerfile.controller.runtime-base`
and copied to `/root/aria-controller/runtime-build/Dockerfile.controller.runtime-base`.
Build it once per runtime package change. It uses the Aliyun Alpine mirror by
default to avoid slow or unstable upstream Alpine downloads:

```dockerfile
FROM alpine:3.19

ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine

RUN if [ -n "${ALPINE_MIRROR}" ]; then \
      sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories; \
    fi \
    && apk add --no-cache openssh-client ca-certificates curl \
    && mkdir -p /var/log/aria /etc/aria
```

The per-release runtime Dockerfile is tracked at `Dockerfile.controller.runtime`
and copied to `/root/aria-controller/runtime-build/Dockerfile.controller.runtime`.
It must not install Alpine packages during a normal release:

```dockerfile
ARG ARIA_CONTROLLER_RUNTIME_BASE=aria-controller-runtime-base:alpine3.19
FROM ${ARIA_CONTROLLER_RUNTIME_BASE}

ARG ARIA_CONTROLLER_VERSION=dev
ENV ARIA_CONTROLLER_VERSION=${ARIA_CONTROLLER_VERSION}

COPY bin/aria /usr/local/bin/aria
RUN chmod +x /usr/local/bin/aria

WORKDIR /opt/aria
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/aria"]
CMD ["controller", "serve", "--config=/etc/aria/controller.yaml"]
```

Build the runtime image on the x86 Linux host and restart only the Controller:

```bash
cd /root/aria-controller/runtime-build
VERSION=<VERSION>

# Run this only when Alpine or runtime package dependencies change.
docker build -f Dockerfile.controller.runtime-base \
  --build-arg ALPINE_MIRROR=https://mirrors.aliyun.com/alpine \
  -t aria-controller-runtime-base:alpine3.19 \
  .

# Run this for every Controller release.
docker build -f Dockerfile.controller.runtime \
  --build-arg ARIA_CONTROLLER_RUNTIME_BASE=aria-controller-runtime-base:alpine3.19 \
  --build-arg ARIA_CONTROLLER_VERSION=${VERSION} \
  -t aria-controller:${VERSION} \
  .
docker tag aria-controller:${VERSION} aria-controller:local

cd /root/aria-controller
if grep -q '^ARIA_VERSION=' .env; then
  sed -i "s/^ARIA_VERSION=.*/ARIA_VERSION=${VERSION}/" .env
else
  printf '\nARIA_VERSION=%s\n' "${VERSION}" >> .env
fi
if grep -q '^ARIA_CONTROLLER_VERSION=' .env; then
  sed -i "s/^ARIA_CONTROLLER_VERSION=.*/ARIA_CONTROLLER_VERSION=${VERSION}/" .env
else
  printf 'ARIA_CONTROLLER_VERSION=%s\n' "${VERSION}" >> .env
fi
grep -q 'image: aria-controller:local' docker-compose.yml || sed -i.bak 's#image: .*aria-controller.*#image: aria-controller:local#' docker-compose.yml
docker compose up -d aria-controller
docker compose ps aria-controller
docker logs --since 2m aria-controller
```

Expected Controller log lines:

```text
Controller ready
gRPC one-way TLS enabled
gRPC server listening on :50051 (TLS: server)
```

## Frontend Deploy

Upload the locally built frontend dist and restart the frontend container. The
frontend is a Docker bind mount (`./frontend/dist:/usr/share/nginx/html:ro`), so
after replacing the `dist` directory do not run only `docker compose up -d`
against an already-running `aria-frontend` container. Restart or force-recreate
the frontend container so Docker remounts the new directory.

```bash
ssh root@<controller-host> 'rm -rf /root/aria-controller/frontend/dist.new && mkdir -p /root/aria-controller/frontend/dist.new'
rsync -az --delete dist/frontend/ root@<controller-host>:/root/aria-controller/frontend/dist.new/
ssh root@<controller-host> '
  set -e
  cd /root/aria-controller/frontend
  rm -rf dist.previous
  [ -d dist ] && mv dist dist.previous
  mv dist.new dist
  docker restart aria-frontend
  # If the container was not restarted after the dist swap, use:
  # cd /root/aria-controller && docker compose up -d --force-recreate aria-frontend
'
```

## Smoke Checks

```bash
ssh root@8.152.163.101 'docker ps --format "{{.Names}}\t{{.Image}}\t{{.Status}}" | grep aria'
ssh root@8.152.163.101 'curl -k --resolve aria.yun:443:127.0.0.1 -fsS https://aria.yun/api/version'
curl -fsS https://aria.yun/api/version
```

## Production Deployment Records

Keep each production deployment record in this section so operators can answer
"is production current?" from the repository and the server state.

Record these fields for every deployment:

| Field | Required value |
| --- | --- |
| Date | UTC timestamp of the production deployment |
| Git commit | `master` commit deployed to Controller and frontend |
| Push CI run | `Build` run that passed after merge to `master` |
| Publish run | `workflow_dispatch` run that pushed the Controller image and uploaded artifacts |
| Controller image | Exact GHCR tag or digest used by `aria-controller` |
| Frontend backup | Server path for the previous frontend `dist` backup |
| Config backup | Server path for the pre-deploy config archive |
| DB backup | Server path for the pre-deploy Postgres dump |
| Agent artifact | `rust-agent-binary` artifact run id and deployed host |
| Verification | Login, menu permissions, Nodes, Monitoring, backup download, Agent sync |

### 2026-06-07 Public IP Correction

Status: deployed.

Purpose:

- Deploy the node public IP correction so SaaS inventory records the true public
  IP and VPN IP only.
- Expected node identity after Agent sync:
  - `public_ip = 82.156.48.111`
  - `assigned_ip = 100.64.0.2`
  - `private_ip = ''`
  - `endpoint = 82.156.48.111:51820`

| Field | Value |
| --- | --- |
| Date | 2026-06-07T02:20Z |
| Git commit | `60e964b59b003fdc4f2e78c1a60362ec091e02ce` |
| Push CI run | `27079569595` |
| Publish run | `27079625892` |
| Controller image | `ghcr.io/chenyongming211-glitch/aria-controller:0.2.35-test@sha256:409d404e991ec8a4ff4fbe50a81cf2360050f6917f34559521f97863bf9af1b4` |
| Frontend backup | `/root/aria-controller/deploy-backups/frontend-dist-20260607T015817Z` |
| Config backup | `/root/aria-controller/backups/pre-public-ip-deploy-config-20260607T015817Z.tar.gz` |
| DB backup | `/root/aria-controller/backups/pre-public-ip-deploy-20260607T015817Z.sql` |
| Agent artifact | `rust-agent-binary` from workflow run `27079625892`, deployed to `82.156.48.111` |
| Verification | Controller container healthy on image `sha256:409d404e991e`; frontend container healthy; `/api/version` returned `0.2.35-test`; `sysadmin` login returned 200; `/api/v2/tenants`, tenant nodes, and tenant roles returned 200; Agent command stream connected and sync completed; node row shows `public_ip=82.156.48.111`, `assigned_ip=100.64.0.2`, empty `private_ip`, and `endpoint=82.156.48.111:51820`. |

Agent gray verification:

```bash
ssh ubuntu@82.156.48.111 'sudo journalctl -u aria-agent --since "5 min ago" --no-pager | tail -120'
```

Healthy Agent evidence should include:

```text
Controller command stream connected
Immediate sync completed
QoS sync completed
```

### 2026-06-07 Nodes API Public IP Response Hotfix

Status: deployed.

Purpose:

- Expose `public_ip`, `private_ip`, and `endpoint` in the tenant-scoped Nodes
  list/detail API responses so the existing frontend `Public IP` column can
  render the real public node address.
- This was a Controller-only hotfix. The frontend artifact and Agent binary were
  not changed for this deployment.

| Field | Value |
| --- | --- |
| Date | 2026-06-07T02:42Z |
| Git commit | `44d6e88a4ed38306227e11f833f5e5682823c72e` |
| Push CI run | `27080519182` |
| Publish run | `27080569906` |
| Controller image | `ghcr.io/chenyongming211-glitch/aria-controller:0.2.35-test@sha256:c1e38f42d144bb5ec8f354f2a7c39de996e801c32963ef8b069fb658691b14f5` |
| Frontend backup | Not changed; frontend remained from the previous deployment. |
| Config backup | Not changed; no Controller config update in this hotfix. |
| DB backup | Not taken; no schema or data migration in this response-only hotfix. |
| Agent artifact | Not changed; Agent remained from workflow run `27079625892`. |
| Verification | Controller container healthy on image `sha256:c1e38f42d144`; `/api/version` returned `0.2.35-test`; `sysadmin` login returned 200; tenant node list and detail both returned `public_ip=82.156.48.111`, empty `private_ip`, `assigned_ip=100.64.0.2`, and `endpoint=82.156.48.111:51820`; browser confirmed the Public IP column is visible. |

### 2026-06-07 Nodes Workbench Endpoint Frontend Hotfix

Status: deployed.

Purpose:

- Keep the Nodes workbench network identity view aligned with the API hotfix by
  preserving `endpoint` in the frontend node store and showing it in the node
  detail dialog.
- This was a frontend-only deployment from the push-triggered GitHub Actions
  `frontend-dist` artifact. The Controller image and Agent binary were not
  changed for this deployment.

| Field | Value |
| --- | --- |
| Date | 2026-06-07T03:00Z |
| Git commit | `deccfd8a4e85f13687d3372a07c52b42e3fd80b1` |
| Push CI run | `27080906919` |
| Publish run | Not used; deployed `frontend-dist` from the push CI run. |
| Controller image | Unchanged from the previous deployment: `ghcr.io/chenyongming211-glitch/aria-controller:0.2.35-test@sha256:c1e38f42d144bb5ec8f354f2a7c39de996e801c32963ef8b069fb658691b14f5` |
| Frontend backup | `/root/aria-controller/frontend/dist.prev-20260607110004` |
| Config backup | Not changed; no Controller config update in this frontend hotfix. |
| DB backup | Not taken; no schema or data migration in this frontend hotfix. |
| Agent artifact | Not changed; Agent remained from workflow run `27079625892`. |
| Verification | `aria-frontend` container healthy; `http://127.0.0.1:18080/` returned 200 with `Cache-Control: no-store`; deployed bundle includes `assets/Nodes-c28a4f4a.js`; the Nodes bundle contains the `Endpoint` detail label. |

### 2026-06-07 Monitoring Node Network Identity Deployment

Status: deployed.

Purpose:

- Align Monitoring node detail with the Nodes workbench network identity model.
- Return `region`, `public_ip`, `assigned_ip`, and `endpoint` from
  `/api/v2/tenants/{tenant_id}/monitoring/nodes/{node_id}`.
- Show the same network identity fields on the Monitoring node detail page so
  operators can verify public IP, VPN IP, and WireGuard endpoint from either
  workflow.

| Field | Value |
| --- | --- |
| Date | 2026-06-07T03:18Z |
| Git commit | `98cd438ddeb60bf7df5ff767b87666918374ac83` |
| Push CI run | `27081201040` |
| Publish run | `27081255342` |
| Controller image | `ghcr.io/chenyongming211-glitch/aria-controller:0.2.35-test@sha256:12134a1682b6197f68db0609fe24771ed094bd739ff9037c7c7b0945fdf5be03` |
| Frontend backup | `/root/aria-controller/frontend/dist.prev-20260607T031755Z` |
| Config backup | `/root/aria-controller/backups/pre-monitor-network-identity-config-20260607T031755Z.tar.gz` |
| DB backup | `/root/aria-controller/backups/pre-monitor-network-identity-20260607T031755Z.sql` |
| Agent artifact | Not changed; Agent remained from workflow run `27079625892`. |
| Verification | `aria-controller` and `aria-frontend` containers healthy; Controller container image id is `sha256:12134a1682b6197`; frontend bundle includes `assets/NodeMonitorDetail-12051133.js` and `assets/Nodes-aed6c53e.js`; `sysadmin` login through `http://127.0.0.1:18080/api/v2/auth/login` succeeded; tenant node list returned `public_ip=82.156.48.111`, `assigned_ip=100.64.0.2`, `endpoint=82.156.48.111:51820`; Monitoring node detail returned matching `monitor_public_ip=82.156.48.111`, `monitor_assigned_ip=100.64.0.2`, `monitor_endpoint=82.156.48.111:51820`, `monitor_region=tencent-cloud`, `recent_commands=10`, `policy_deliveries=2`, and `active_alerts=0`. |

### 2026-06-25 v0.1.0 E2E Closure Gray Deployment

Status: gray deployed.

Purpose:

- Close the first-stage onboarding, control, and operations loops for v0.1.0.
- Add the Nodes onboarding dialog and generated `aria-agent init` command.
- Preserve Monitoring/Node Detail alert context into the AI assistant without
  auto-executing write actions.
- Verify the Controller to Agent control path for command dispatch plus
  ACL/QoS/Route policy delivery on a real online Agent.

| Field | Value |
| --- | --- |
| Date | 2026-06-25T00:44Z |
| Git commit | `290e04b2efb6255766f27b9fee002c795ab34b13` |
| Branch CI run | `28114344395` |
| Version | `0.2.45` |
| Controller image | Local runtime image `aria-controller:0.2.45`, also tagged `aria-controller:local` on `8.152.163.101`. |
| Frontend backup | `/root/aria-controller/frontend/dist.prev-20260625-004439` |
| Config backup | `/root/aria-controller/backups/pre-v010-closure-config-20260625-004439.tar.gz` |
| DB backup | `/root/aria-controller/backups/pre-v010-closure-20260625-004439.sql` |
| Agent artifact | Not changed; this deployment changed Controller/frontend/docs only. |
| Verification | Branch Actions run `28114344395` passed Go Build, Frontend Build, and Rust Agent Build. Local `go test ./...`, frontend unit tests, `git diff --check`, and frontend build passed before deployment. Server-side `/api/version` returned `0.2.45`; `aria-controller` and `aria-frontend` were healthy; `https://aria.yun/` returned HTTP 200 from the server; deployed frontend bundles include the onboarding command inputs (`--controller-api-url`) and the AI diagnostic prompt. Online smoke with `sysadmin` verified active tenant/node discovery, Monitoring stats/health/events/alerts, node detail desired/applied/observed state, enrollment token create/delete, `health_check` command queued and completed, temporary ACL/QoS/Route create, Agent desired/applied convergence after create, temporary ACL/QoS/Route delete, and Agent desired/applied convergence after delete. |
| Smoke detail | Temporary policy CIDRs were `acl=10.253.53.28/32`, `qos=10.252.53.28/32`, and `route=10.251.53.0/24`. Create convergence reached `desired=dsv-1782320009-6984d50f`; delete convergence reached `desired=dsv-1782320013-2ed519fc`. The temporary token and policies were deleted after validation. |

### 2026-06-26 v0.1.0 Closure Master Deployment and Final Validation

Status: deployed and validated.

Purpose:

- Record the master deployment used for the final onboarding and control-loop
  closure validation.
- Verify one clean VM can self-onboard through the installer flow and that a
  real Agent applies ACL, QoS, Route, and command-loop changes end to end.

| Field | Value |
| --- | --- |
| Date | 2026-06-26T09:24+08:00 deployment; 2026-06-26T09:40+08:00 to 2026-06-26T09:48+08:00 final validation |
| Git commit | `8939283ffe49eba0290bdd9b88096ef9334f33e3` |
| Push CI run | `28210865082` |
| Publish run | Not used; deployed low-bandwidth local payload from master run `28210865082`. |
| Version | `0.2.62` |
| Controller image | Local runtime image `aria-controller:local@sha256:136206400398fe52fa3f43ac7391ca6ed2bf475d6a2af2c1ba4dbc791d74267a`. |
| Frontend backup | `/root/aria-controller/deploy-backups/20260626092400-0.2.62-28210865082-master/frontend-dist` |
| Config backup | `/root/aria-controller/deploy-backups/20260626092400-0.2.62-28210865082-master/config` |
| DB backup | `/root/aria-controller/deploy-backups/20260626092400-0.2.62-28210865082-master/postgres.sql` |
| Release payload | `/root/aria-controller/aria-0.2.62-28210865082-master-payload.tgz`; unpacked release path `/root/aria-controller/releases/0.2.62-28210865082-master` |
| Agent artifact | Existing Agent on `82.156.137.42`, `82.156.48.111`, `43.143.245.123`, and `43.140.246.115`; no Rust/eBPF rebuild in this validation. |
| Verification | Server-side `/api/version` returned `0.2.62`; Controller and frontend containers were healthy. New VM `82.156.137.42` joined as `node-82-156-137-42` with node id `d5c7723c-3d86-48ce-a9fc-4695cd170b1c`, VPN IP `100.64.0.40`, endpoint `82.156.137.42:51820`, and `aria0` through `aria3` all active with three peers. `100.64.0.40 -> 100.64.0.2` VPN ping succeeded. Node detail returned `configuration_status=applied`, `convergence_status=converged`, `observed_state=applied`, and `pending_cmds=0`. |
| Control-loop validation | ACL egress ICMP allow on `100.64.0.2/32` applied and ping succeeded; updating the same ACL to deny applied, ping was blocked, and stats reported `dropped_packets=4`, `dropped_bytes=336`; deleting the ACL restored ping. QoS egress `100.64.0.27/32` create at `1 Mbps` applied and stats reported `passed_bytes=20560`; update to `2 Mbps` applied and stats reported `passed_bytes=19532`; delete removed the rule. Route create `10.255.240.40/32` applied and appeared in the peer WireGuard allowed ips for `100.64.0.40`; update to `10.255.240.41/32` updated the peer allowed ips; delete removed it. |
| Command-loop validation | `health_check` completed with `agent healthy`. A controlled failure test stopped `aria-agent`, queued short-timeout `health_check`, and confirmed command status `failed` with message `command timed out waiting for agent result`; restarting Agent restored `online/applied/converged`, and a follow-up `health_check` completed. |

### 2026-06-27 Confirmed Bugfix Closure Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Deploy the BUG-25 to BUG-35 closure branch with a patch-version bump.
- Keep the deployment on the low-bandwidth Controller/frontend artifact path
  because this patch does not change Rust Agent runtime code.
- Preserve rollback state before replacing the Controller runtime image and
  frontend bundle.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T02:48Z deployment; 2026-06-27T02:49Z to 2026-06-27T02:51Z smoke validation |
| Git commit | `b3c0fb60d94b` |
| Branch CI run | `28276174524` |
| Version | `0.2.64` |
| Controller image | Local runtime image `aria-controller:local@sha256:33509d6fd855f04152ffc4de137e999049233c805914e8fc8a57557a8f5de212`. |
| Frontend backup | `/root/aria-controller/deploy-backups/20260627T024846Z-0.2.64-28276174524-b3c0fb60d94b/frontend-dist` |
| Config backup | `/root/aria-controller/deploy-backups/20260627T024846Z-0.2.64-28276174524-b3c0fb60d94b/config` |
| DB backup | `/root/aria-controller/deploy-backups/20260627T024846Z-0.2.64-28276174524-b3c0fb60d94b/postgres.sql` |
| Release payload | `/root/aria-controller/aria-0.2.64-28276174524-b3c0fb60d94b-payload.tgz`; unpacked release path `/root/aria-controller/releases/0.2.64-28276174524-b3c0fb60d94b` |
| Agent artifact | Not changed; Rust Agent CI still passed in branch Actions run `28276174524`. |
| Verification | Local focused Go tests, frontend unit tests, frontend build, linux/amd64 Controller build, and `git diff --check` passed before deployment. Branch Actions run `28276174524` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.64`, `https://aria.yun/` returned HTTP 200, Controller info returned version `0.2.64`, and `aria-controller` plus `aria-frontend` were healthy. |
| Local network note | The local macOS environment could open TCP to `8.152.163.101:443`, but TLS was reset before reaching the frontend container. The environment also had proxy variables pointing to `127.0.0.1:7897`. Nginx logs did not receive those failed local TLS attempts, while server-side public-domain smoke succeeded. |

### 2026-06-27 Non-AI Operations Loop Audit Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Deploy the non-AI operations-loop audit closure.
- Record `command.queued` audit evidence for manual Agent commands and carry
  `reason/source/command_id` into alert resolve audit details.
- Keep Controller/frontend deployment on the low-bandwidth local artifact path;
  Rust Agent artifacts were built by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T11:56+08:00 deployment; 2026-06-27T11:57+08:00 smoke validation |
| Git commit | `89ddb72724ea` |
| Branch CI runs | `28277074037`, `28277430166`, `28277591501` |
| Master CI run | `28277723272` |
| Version | `0.2.65` |
| Controller image | Local runtime image `aria-controller:local@sha256:6583c400158f735a0a2f69857ee87db95c0ad7a179fb7bdc95860d486dd8d74b`. |
| Frontend backup | `/root/aria-controller/deploy-backups/20260627T035624Z-0.2.65-28277723272-89ddb72724ea/frontend-dist` |
| Runtime binary backup | `/root/aria-controller/deploy-backups/20260627T035624Z-0.2.65-28277723272-89ddb72724ea/runtime-bin` |
| Config backup | `/root/aria-controller/deploy-backups/20260627T035624Z-0.2.65-28277723272-89ddb72724ea/.env` and `docker-compose.yml` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Branch Actions run `28277591501` and master Actions run `28277723272` passed Go Build, Frontend Build, and Rust Agent Build. Local linux/amd64 Controller/ariactl build and frontend build passed before deployment. Server-side `https://aria.yun/` returned HTTP 200. `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.65`; `aria-controller` and `aria-frontend` were healthy. |
| Operations smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants. Smoke selected tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db` and online node `node-82-156-137-42` (`d5c7723c-3d86-48ce-a9fc-4695cd170b1c`). Node detail, Monitoring stats, Monitoring alerts, and Monitoring topology all returned HTTP 200. A `health_check` command with `source=master-smoke` and `run_id=28277723272` queued as `2b67f001-9ddb-4ffe-9446-1a0a5f6d9f13`, then reached `completed`; Agent status returned `last_command_status=completed` and `configuration_status=applied`. |

### 2026-06-27 Frontend Workflow Context Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Preserve node, policy, and command context when moving from Monitoring or
  Nodes into Policy Center and then into ACL, QoS, or Route pages.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T12:23+08:00 gray deployment; 2026-06-27T12:33+08:00 master deployment; 2026-06-27T12:35+08:00 smoke validation |
| Git commit | `1860acf5ad99ce29d44cd6c846accd8f47300ebc` |
| Branch CI run | `28278318460` |
| Master CI run | `28278563853` |
| Version | `0.2.66` |
| Controller image | Local runtime image `aria-controller:local@sha256:8907411cc170902597455d5f708a338a59e55275efb5042c3be2ccbe504ddfd3`. |
| Frontend backup | `/root/aria-controller/deploy-backups/20260627T043354Z-0.2.66-28278563853-1860acf5ad99/frontend-dist` |
| Runtime binary backup | `/root/aria-controller/deploy-backups/20260627T043354Z-0.2.66-28278563853-1860acf5ad99/runtime-bin` |
| Config backup | `/root/aria-controller/deploy-backups/20260627T043354Z-0.2.66-28278563853-1860acf5ad99/.env`, `docker-compose.yml`, and `config` |
| DB backup | `/root/aria-controller/deploy-backups/20260627T043354Z-0.2.66-28278563853-1860acf5ad99/postgres.sql` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`142` tests), plus focused policy-context tests. Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28278318460` and master Actions run `28278563853` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` returned `0.2.66`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`; deployed bundles included `Policies-110ed234.js`, `ACLRules-933b5618.js`, `BandwidthControl-4fe5effa.js`, and `Routing-fa19cc25.js`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected tenant `Aria Default`; tenant Nodes returned 4 items; tenant Monitoring stats returned HTTP 200; tenant Policies returned 11 items. |

### 2026-06-27 Frontend Alert Command Trace Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Close the Monitoring action handoff gap: after an operator runs `sync` or
  `health_check` from an active alert, the console now opens the node detail
  command section with the newly queued command id.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T12:56+08:00 gray deployment; 2026-06-27T13:10+08:00 master deployment; 2026-06-27T13:12+08:00 smoke validation |
| Git commit | `83428ab07e3eed7136f3a4d85e667bdc313f88a5` |
| Branch CI run | `28279026090` |
| Master CI run | `28279360441` |
| Version | `0.2.67` |
| Controller image | Local runtime image `aria-controller:local@sha256:9323a11126374990bc568c55685bb7f3b37347fcf63e3a4ae159e0ce4c2fa201`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T045535Z-0.2.67-28279026090-83428ab07e3e` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T051016Z-0.2.67-28279360441-83428ab07e3e` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`142` tests), plus focused Monitoring/Nodes/Policy/ACL/QoS tests (`58` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28279026090` and master Actions run `28279360441` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.67`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring stats returned `ok`; tenant Policies returned 11 items. |
| Deployment note | During gray validation, `/api/version` returned `0.2.67` while `/api/v2/controller-info` initially returned the old `.env` value. The server `.env` was updated with `ARIA_VERSION=0.2.67` and `ARIA_CONTROLLER_VERSION=0.2.67` per this document before final master deployment. |

### 2026-06-27 Frontend Policy Retry Trace Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Close the Policy Center retry handoff gap: after an operator retries a
  failed policy delivery, the console now opens the node detail command section
  with the newly queued retry command id.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T13:33+08:00 gray deployment; 2026-06-27T13:42+08:00 master deployment; 2026-06-27T13:43+08:00 smoke validation |
| Git commit | `1096282cb27f635de58f051f25b87e5f80a2079e` |
| Branch CI run | `28279869486` |
| Master CI run | `28280050514` |
| Version | `0.2.68` |
| Controller image | Local runtime image `aria-controller:local@sha256:7cbcda418a160cedbe735c2710e132bf804de98b21b4c4f12e50709c7c805f3e`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T053324Z-0.2.68-28279869486-1096282cb27f` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T054225Z-0.2.68-28280050514-1096282cb27f` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`142` tests), plus focused Monitoring/Nodes/Policy/ACL/QoS tests (`58` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28279869486` and master Actions run `28280050514` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.68`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring stats returned `ok`; tenant Policies returned 11 items. |
| Deployment note | The first master upload attempt hit a transient SSH timeout before replacement. A fresh deploy id was used for the final master deployment, and the final smoke validated the replacement. |

### 2026-06-27 Frontend Node Status Display Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Keep node detail command and policy-delivery status wording aligned with
  Policy Center by reusing the shared control-loop status mapper.
- Treat `error`, `timeout`, and `timed_out` command states as failed terminal
  states so quick-command polling and node summary counts do not drift.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T14:02+08:00 gray deployment; 2026-06-27T14:09+08:00 master deployment; 2026-06-27T14:10+08:00 smoke validation |
| Git commit | `796f38bab5dad060b5d69080b73a376d7285d0c7` |
| Branch CI run | `28280508971` |
| Master CI run | `28280657776` |
| Version | `0.2.69` |
| Controller image | Local runtime image `aria-controller:local@sha256:dd511df19b324b33952d0e3575603df7931d44c4a4ac0cc1aa3b93340ce82de9`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T060202Z-0.2.69-28280508971-796f38bab5da` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T060931Z-0.2.69-28280657776-796f38bab5da` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`143` tests), including focused `controlLoopStatus` and `nodesWorkbench` tests. Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28280508971` and master Actions run `28280657776` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.69`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring stats returned `ok`; tenant Policies returned 11 items. |

### 2026-06-27 Frontend Rule Retry Trace Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Close the ACL and QoS page retry handoff gap: after an operator retries a
  failed rule delivery from the dedicated ACL or QoS page, the console now opens
  the node detail command section with the newly queued retry command id.
- Keep this aligned with the Policy Center retry behavior introduced in
  `0.2.68`.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T14:29+08:00 gray deployment; 2026-06-27T14:37+08:00 master deployment; 2026-06-27T14:38+08:00 smoke validation |
| Git commit | `e1b94d1a810149a1216a4307414b4c709f5c749b` |
| Branch CI run | `28281076556` |
| Master CI run | `28281268428` |
| Version | `0.2.70` |
| Controller image | Local runtime image `aria-controller:local@sha256:ace467eac064143dbbfe852d02ed4edac6710cf0e0d82a0275c6be08ad3e00d1`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T062919Z-0.2.70-28281076556-e1b94d1a8101` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T063748Z-0.2.70-28281268428-e1b94d1a8101` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`145` tests), including focused `policyPageContext`, `useAclApi`, and `useQosApi` tests. Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28281076556` and master Actions run `28281268428` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.70`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Policies returned 11 items. |

### 2026-06-27 Frontend Rule Mutation Trace Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Close the ACL and QoS mutation handoff gap: after an operator creates,
  updates, toggles, or deletes a dedicated ACL or QoS rule, the console opens
  the node detail command section with the newly queued mutation command id.
- Keep this aligned with the Policy Center and retry handoff behavior introduced
  in `0.2.68` through `0.2.70`.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T14:58+08:00 gray deployment; 2026-06-27T15:10+08:00 master deployment; 2026-06-27T15:11+08:00 smoke validation |
| Git commit | `99f788944096917bab6a5fc010a8112a1edcb804` |
| Branch CI run | `28281728879` |
| Master CI run | `28281925156` |
| Version | `0.2.71` |
| Controller image | Local runtime image `aria-controller:local@sha256:cd3093fba02691b81371729bd9acfdfce390395e79faeab9f1715fe6e2acb274`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T065824Z-0.2.71-28281728879-99f788944096` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T071025Z-0.2.71-28281925156-99f788944096-ldflags` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`149` tests), including focused `policyPageContext`, `useAclApi`, and `useQosApi` tests (`36` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28281728879` and master Actions run `28281925156` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.71`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Policies returned 11 items. |
| Deployment note | The first master local-artifact deploy built the Controller without the same `aria/internal/cli.Version` ldflags used by GitHub Actions, so `/api/version` returned `dev` while `/api/v2/controller-info` returned `0.2.71` from the environment. The Controller was rebuilt with the documented ldflags and redeployed under the final master backup above. |

### 2026-06-27 Frontend Route Mutation Trace Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Close the Route mutation handoff gap: after an operator creates, updates, or
  deletes a route from the Routing page, the console opens the node detail
  command section with the newly queued route command id.
- Keep Route behavior aligned with the ACL/QoS mutation trace behavior
  introduced in `0.2.71`.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T15:30+08:00 gray deployment; 2026-06-27T15:38+08:00 master deployment; 2026-06-27T15:39+08:00 smoke validation |
| Git commit | `461daa981fdb6fc75fdd53156824aec91c48d8af` |
| Branch CI run | `28282433907` |
| Master CI run | `28282606252` |
| Version | `0.2.72` |
| Controller image | Local runtime image `aria-controller:local@sha256:918fa49e27db95f7e57981888b8f017ec7d96b2be79f28bab37203d8e2bd7791`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T073019Z-0.2.72-28282433907-461daa981fdb` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T073832Z-0.2.72-28282606252-461daa981fdb` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`152` tests), including focused `policyPageContext` tests (`15` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28282433907` and master Actions run `28282606252` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.72`; `aria-controller` and `aria-frontend` were healthy; frontend `index.html` returned `Cache-Control: no-store`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Policies returned 11 items. |

### 2026-06-27 Frontend I18n Foundation Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Replace fixed route titles with `titleKey` metadata so the page header follows
  the active UI language instead of hardcoded Chinese labels.
- Add shared translation support for command and policy delivery status labels
  while preserving existing Chinese fallbacks for non-UI callers.
- Remove stale visible AI/product wording from the shell for this foundation
  pass; broader AI/Hermes work remains deferred.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T16:16+08:00 gray deployment; 2026-06-27T16:28+08:00 master deployment; 2026-06-27T16:30+08:00 smoke validation |
| Git commit | `39409fd655c219af3e092ed2aa46155cd0ba16d0` |
| Branch CI run | `28283473397` |
| Master CI run | `28283739150` |
| Version | `0.2.73` |
| Controller image | Local runtime image `aria-controller:local@sha256:d95a25972840bd3be5bba30c00e2982beb35c0018be8db0303191862a4329117`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T081644Z-0.2.73-28283473397-39409fd655c2` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T082848Z-0.2.73-28283739150-39409fd655c2` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`155` tests), including focused router permission, page permission visibility, and control-loop status tests. Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28283473397` and master Actions run `28283739150` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.73`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Monitoring alerts returned 0 items; tenant Monitoring events returned 5 items; tenant Policies returned 11 items. |
| Frontend smoke | Deployed bundle no longer contains `AI Copilot`, `智能副驾`, `System Online`, or `Node Monitor Detail` fixed strings. |
| Deployment note | After the gray deploy, `/api/version` returned `0.2.73` but `/api/v2/controller-info` still returned `0.2.72` because production `.env` had stale `ARIA_VERSION` and `ARIA_CONTROLLER_VERSION` values. The master deploy updated both environment variables to `0.2.73` before rebuilding the runtime image and restarting `aria-controller`. |

### 2026-06-27 Frontend Design Token Cleanup Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Remove the unused Plus Jakarta webfont dependency and use a single system font
  stack that supports Chinese well.
- Reduce operational surface decoration by removing default card shadows, glow
  tokens, stat-card gradient bars, button gradients, and default pulsing status
  dots from the shared frontend foundation.
- Align Element Plus card, button, input, and table radius/shadow treatment with
  the 6px/8px operational token rules.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T16:52+08:00 gray deployment; 2026-06-27T17:01+08:00 master deployment; 2026-06-27T17:02+08:00 smoke validation |
| Git commit | `966b3ce2ebd3a0ca4bd44b50b15567d4920dacfa` |
| Branch CI run | `28284209602` |
| Master CI run | `28284433543` |
| Version | `0.2.74` |
| Controller image | Local runtime image `aria-controller:local@sha256:64bbd2e63ef12c46efb72725c2f8d5bb7d6023bc012921f8632b4c76b5dd86c4`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T085239Z-0.2.74-28284209602-966b3ce2ebd3` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T090134Z-0.2.74-28284433543-966b3ce2ebd3` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`155` tests) and local frontend build passed. Local linux/amd64 Controller/ariactl build passed. Branch Actions run `28284209602` and master Actions run `28284433543` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.74`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Policies returned 11 items. |
| Frontend smoke | Deployed bundle no longer contains Plus Jakarta font assets or `Avenir` references. |

### 2026-06-27 Frontend UI Foundation Components Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Add internal reusable frontend UI foundation components for later product
  workflow cleanup: page headers, metric strips, data panels, status badges,
  icon action buttons, and filter bars.
- Keep the components covered by focused unit tests before wiring them into
  larger pages.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T17:31+08:00 gray deployment; 2026-06-27T17:39+08:00 master deployment; 2026-06-27T17:40+08:00 smoke validation |
| Git commit | `b8d33be7cef635b407f8c6a09cc53bd7d20efd00` |
| Branch CI run | `28284981185` |
| Master CI run | `28285299904` |
| Version | `0.2.75` |
| Controller image | Local runtime image `aria-controller:local@sha256:c408f25185afed23aeb63d3e4c8f1e785ee44742c6777ca7ad1bbed85b79f7f4`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T093133Z-0.2.75-28284981185-b8d33be7cef6` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T093957Z-0.2.75-28285299904-b8d33be7cef6-ldflags` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`161` tests), including focused UI foundation component tests (`6` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28284981185` and master Actions run `28285299904` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.75`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Monitoring alerts returned 0 items; tenant Policies returned 11 items. |
| Deployment note | An intermediate master local-artifact deploy used the wrong manual ldflags package path and `/api/version` returned `dev`. The Controller was rebuilt with the documented `aria/internal/cli.Version` ldflags and redeployed under the final master backup above. |

### 2026-06-27 Frontend Nodes Workbench Foundation Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Wire the shared UI foundation components into the Nodes workbench list shell:
  page header, metric strip, filter bar, data panel, status badges, and icon
  action buttons.
- Keep this pass low risk by preserving existing Nodes business logic, backend
  APIs, onboarding flow, node detail dialog, and command/policy behavior.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T18:01+08:00 gray deployment; 2026-06-27T18:09+08:00 master deployment; 2026-06-27T18:10+08:00 smoke validation |
| Git commit | `b8381bbf318add4bc0ee92422387b8c32decdc25` |
| Branch CI run | `28285807895` |
| Master CI run | `28285970922` |
| Version | `0.2.76` |
| Controller image | Local runtime image `aria-controller:local@sha256:3865aec2e9be869806f12178b7fbb33fbd0192860c8dfbc97d893dc26f785495`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T100132Z-0.2.76-28285807895-b8381bbf318a` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T100917Z-0.2.76-28285970922-b8381bbf318a` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`162` tests), including focused Nodes workbench tests (`6` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28285807895` and master Actions run `28285970922` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.76`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Monitoring alerts returned 0 items; tenant Policies returned 11 items. |
| Frontend smoke | Deployed frontend assets contain the new Nodes workbench foundation classes `ui-page-header` and `ui-metric-strip`. |

### 2026-06-27 Frontend Monitoring Workflow Foundation Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Wire the shared UI foundation components into the Monitoring operations shell:
  page header, metric strip, filter bar, data panels, and status badges.
- Preserve the existing Monitoring -> Node Detail / Policy Center action
  context, including Run Sync, Health Check, Resolve, and event filtering.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T18:32+08:00 gray deployment; 2026-06-27T18:40+08:00 master deployment; 2026-06-27T18:41+08:00 smoke validation |
| Git commit | `3c21a4c7b5f22bfaa0a98b1ff312cbb0ad230162` |
| Branch CI run | `28286460178` |
| Master CI run | `28286672665` |
| Version | `0.2.77` |
| Controller image | Local runtime image `aria-controller:local@sha256:203957898310a6204672cfe3d31b8c5883124ebb1605e7fb959db2c5e46373b0`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T103228Z-0.2.77-28286460178-3c21a4c7b5f2` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T104036Z-0.2.77-28286672665-3c21a4c7b5f2` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`164` tests), including focused Monitoring workflow tests (`20` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28286460178` and master Actions run `28286672665` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.77`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Monitoring health returned `ok`; tenant Monitoring alerts returned 0 items; tenant Monitoring events returned 5 items; tenant Policies returned 11 items. |
| Frontend smoke | Deployed frontend assets contain the Monitoring workflow foundation markers `ui-page-header`, `ui-metric-strip`, and `Monitoring Center`. |

### 2026-06-27 Frontend Policy Center Foundation Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Wire the shared UI foundation components into the Policy Center shell: page
  header, metric strip, filter bar, and data panel.
- Keep existing Policy Center context handoff behavior intact for ACL, QoS,
  Route, node detail, retry delivery, and delivery history.
- Add clickable policy metric shortcuts so operators can quickly focus failed,
  pending, applied, or policy-domain-specific inventory.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T19:05+08:00 gray deployment; 2026-06-27T19:13+08:00 master deployment; 2026-06-27T19:14+08:00 smoke validation |
| Git commit | `ca19e1c2c3aac52df1a5e184513239e021fe4532` |
| Branch CI run | `28287186363` |
| Master CI run | `28287377802` |
| Version | `0.2.78` |
| Controller image | Local runtime image `aria-controller:local@sha256:d7b1e327d06e2f09d9be6a25d7a4ff2dceda9b1711560dc1d3ce18c8bdb2844d`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T110504Z-0.2.78-28287186363-ca19e1c2c3aa` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T111301Z-0.2.78-28287377802-ca19e1c2c3aa` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`166` tests), including focused Policy Center context tests (`17` tests). Local linux/amd64 Controller/ariactl build and frontend build passed. Branch Actions run `28287186363` and master Actions run `28287377802` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.78`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Policies returned 11 items; tenant Monitoring health returned `ok`; tenant Monitoring events returned 5 items. |
| Frontend smoke | Deployed frontend assets contain the Policy Center foundation markers `ui-page-header`, `ui-metric-strip`, and `Policy Center`. |

### 2026-06-27 Frontend Policy Rule Pages Foundation Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Wire the shared UI foundation components into the ACL and QoS policy rule
  pages: page header, filter bar, data panel, and consistent icon action
  buttons.
- Preserve existing ACL/QoS create, edit, delete, retry, command-trace handoff,
  stats display, and node-scoped API behavior.
- Keep this as a Controller/frontend-only deployment; Rust Agent artifacts were
  validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T19:36+08:00 gray deployment; 2026-06-27T19:44+08:00 master deployment; 2026-06-27T19:45+08:00 smoke validation |
| Git commit | `ae92781d86eb1e93c68dd7e2751d6b97284f79c7` |
| Branch CI run | `28287874281` |
| Master CI run | `28288074140` |
| Version | `0.2.79` |
| Controller image | Local runtime image `aria-controller:local@sha256:f83c98988fda30610025263d05b02041b68443ede4f5ad1d4a65f042aa589ebb`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260627T113628Z-0.2.79-28287874281-ae92781d86eb` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T114445Z-0.2.79-28288074140-ae92781d86eb` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local frontend unit tests passed (`168` tests), including focused Policy Center context tests (`19` tests). Local linux/amd64 Controller/ariactl build and frontend build passed from a clean worktree. Branch Actions run `28287874281` and master Actions run `28288074140` both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` both returned `0.2.79`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; tenant Policies returned 11 items; tenant Monitoring health returned `ok`; tenant Monitoring events returned 5 items. Node-scoped ACL and QoS list APIs for node `d5c7723c-3d86-48ce-a9fc-4695cd170b1c` both returned HTTP 200. |
| Frontend smoke | Deployed frontend assets contain the ACL/QoS rule page foundation markers `ui-page-header`, `ui-filter-bar`, and `ui-data-panel`; assets also contain `ACL 规则管理` and `带宽控制`. |

### 2026-06-27 Frontend TypeScript First-Stage Deployment

Status: deployed and server-side smoke validated.

Purpose:

- Complete the first-stage frontend TypeScript migration for shared API
  response helpers, monitoring/agent/policy/ACL/QoS composables, core stores,
  low-risk config/utilities, and selected high-risk Vue page scripts.
- Keep this as a behavior-preserving Controller/frontend deployment; Rust Agent
  artifacts were validated by CI but not redeployed.
- Publish the already-validated frontend TypeScript migration under a unique
  release version instead of reusing `0.2.79`.

| Field | Value |
| --- | --- |
| Date | 2026-06-27T23:08+08:00 master deployment; 2026-06-27T23:16+08:00 smoke validation |
| Git commit | `896272b8305caee6d882a98fa54136e071bf2e6a` |
| Branch CI runs | `28292425865` for `codex/frontend-utils-b15`; `28292846921` for `codex/release-0.2.80` |
| Master CI runs | `28292651167` for the TypeScript migration merge; `28293000439` for the `0.2.80` release commit |
| Version | `0.2.80` |
| Controller image | Local runtime image `aria-controller:local` built from uploaded linux/amd64 Controller binary. |
| Master backup | `/root/aria-controller/deploy-backups/20260627231554` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch and master Actions. |
| Verification | Local `go test ./...` passed, local linux/amd64 Controller/ariactl build passed, local frontend unit tests passed (`173` tests), and local frontend build passed. Master Actions run `28293000439` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.80`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200 and referenced `assets/index-c80505ff.js`. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned a valid tenant `e779e1f9-1e74-4fc1-915d-f8b432abd421`; tenant Nodes, Monitoring stats, Monitoring events, and Monitoring health all returned HTTP 200. |
| Deployment note | This was a Controller/frontend-only low-bandwidth deployment. `/api/v2/health` is not a valid smoke endpoint in this deployment path and returned 404; tenant-scoped Monitoring health is the validated health API. |

### 2026-06-28 Frontend Workflow Context and TypeScript Gray Deployment

Status: deployed from `master` and server-side smoke validated.

Purpose:

- Validate the `codex/frontend-workflow-closure` branch after the cross-page
  context fixes for Nodes, Monitoring, Policy Center, ACL, QoS, Route, and IP
  Group pages, then keep validating the same branch as the non-AI frontend
  TypeScript migration continues.
- Preserve command, policy, alert, and node context across page jumps, including
  Node Detail focus for command and policy evidence.
- Confirm the non-AI router, API client, app bootstrap, i18n, store, permission,
  tenant, settings, token, and policy/monitoring composable migrations still
  build and serve correctly. `useAiApi.js` intentionally remains JavaScript
  because AI/Hermes work is deferred.
- Keep this as a Controller/frontend-only low-bandwidth deployment; Rust Agent
  artifacts were validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-28T03:01+08:00 initial gray deployment; 2026-06-28T04:32+08:00 frontend-only gray update; 2026-06-28T04:35+08:00 gray smoke validation; 2026-06-28T04:51+08:00 master deployment; 2026-06-28T04:53+08:00 master smoke validation |
| Git commit | Runtime deployed from `60e48c7c65691a2344330eb3e9b4828b7c431c70`; this deployment record was committed afterward as a docs-only follow-up. |
| Branch | `codex/frontend-workflow-closure` |
| Branch CI runs | `28298596624`, `28299146597`, `28299393566`, `28299643997`, `28299834095`, `28300149077`, `28300475309`, `28300620823`, `28300816666` |
| Master CI run | `28301243315` |
| Version | `0.2.81` |
| Controller image | Master runtime image `aria-controller:local@sha256:7c32188e5c754d0020bacb59a7ac3721091bb193b679140f6956be6cc1bfe43c`. |
| Gray backup | Initial gray backup: `/root/aria-controller/deploy-backups/20260627T190103Z-0.2.81-28298596624-1563729`; latest frontend-only backup: `/root/aria-controller/deploy-backups/frontend-dist-before-e84c9a1-20260628043253.tar.gz` |
| Master backup | `/root/aria-controller/deploy-backups/20260627T205119Z-0.2.81-28301243315-60e48c7` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch Actions. |
| Verification | Branch Actions runs through `28300816666` passed Go Build, Frontend Build, and Rust Agent Build. Master Actions run `28301243315` passed Go Build, Frontend Build, and Rust Agent Build. For the latest frontend-only update, local `npm run type-check`, focused router/session tests, full frontend unit tests (`192` tests), `npm run build`, and `git diff --check` passed. For the master deployment, local linux/amd64 Controller/ariactl build and frontend build passed. Server-side `https://aria.yun/` returned HTTP 200 with `Cache-Control: no-store`; the deployed entry referenced `assets/index-bc990307.js`, that asset returned HTTP 200 with 99,909 bytes; `https://aria.yun/api/version` returned `0.2.81`; `/health` and `/api/v2/controller-info` returned HTTP 200; `aria-controller` and `aria-frontend` were healthy; Controller logs showed HTTP and gRPC TLS listeners ready. Browser automation from the local workstation timed out on `https://aria.yun`, consistent with the known local 443/proxy path issue; server-side HTTPS validation remained healthy. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes, Policies, Monitoring health, Monitoring events, and Node Detail for `d5c7723c-3d86-48ce-a9fc-4695cd170b1c` all returned HTTP 200. |
| Frontend smoke | Deployed frontend assets contain `PolicyContextBanner` and command-focus markers for the cross-page context workflow. |
| Deployment note | The feature branch was fast-forwarded into `master`, master CI passed, and master Controller/frontend artifacts were deployed through the low-bandwidth runtime image path. Rust Agent artifacts were validated by CI but not redeployed. This docs-only deployment record commit does not require a runtime redeploy. |

### 2026-06-28 Node Location Response Deployment

Status: deployed from `master` and server-side smoke validated.

Purpose:

- Close the non-AI node response bug batch by returning `region` and `vpc_id`
  from tenant-scoped node list/detail responses.
- Publish the fix under a unique release version instead of reusing `0.2.81`.
- Keep this as a Controller/frontend low-bandwidth deployment; Rust Agent
  artifacts were validated by CI but not redeployed.

| Field | Value |
| --- | --- |
| Date | 2026-06-28T05:38+08:00 master deployment; 2026-06-28T05:40+08:00 smoke validation |
| Git commit | `42906d0f3ed62c2b76549c63d5061980212984e1` |
| Master CI run | `28302385567` |
| Version | `0.2.82` |
| Controller image | Local runtime image `aria-controller:0.2.82` / `aria-controller:local@sha256:976d20307df3cafac32dbb16509a2f62aacb0c0d2dd864ac2995bdfa2d1950e8`. |
| Backup | `/root/aria-controller/deploy-backups/20260627T213841Z-0.2.82-28302385567-42906d0` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in master Actions. |
| Verification | Local `go test ./...` passed, local linux/amd64 Controller/ariactl build passed, local frontend unit tests passed (`192` tests), and local frontend build passed. Master Actions run `28302385567` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.82`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200 and referenced `/assets/index-bc990307.js`; Controller logs showed HTTP and gRPC TLS listeners ready. |
| API smoke | Login succeeded as `sysadmin`; tenant listing returned 4 tenants; selected active tenant `0bc152e2-5bdc-4b62-9333-3376dacc28db`; tenant Nodes returned 4 items; first node `node-82-156-137-42` returned `region=beijing`, `vpc_id=""`, `public_ip=82.156.137.42`, and `assigned_ip=100.64.0.40`. |
| Deployment note | This docs-only deployment record commit does not require a runtime redeploy. |

### 2026-06-28 Platform Backup and Certificate Lifecycle Deployment

Status: deployed from `master` and server-side smoke validated.

Purpose:

- Close Settings/Backup restore safety: dry-run, selected table restore, and
  mandatory confirmation phrase for destructive restore.
- Close the first certificate lifecycle path: registration-time certificate
  issue, renewal failure visibility, and node lifecycle revocation.
- Deploy Controller, frontend, and Agent artifact together because this release
  changes the Agent gRPC/protobuf certificate contract.

| Field | Value |
| --- | --- |
| Date | 2026-06-28T10:40+08:00 gray deployment; 2026-06-28T11:05+08:00 master deployment; 2026-06-28T11:10+08:00 master smoke validation |
| Git commit | `8f2e45442b51fae62ec33e0763e0c99ec3401423` |
| Branch CI run | `28308920401` |
| Master CI run | `28309407829` |
| Version | `0.2.83` |
| Controller image | Local runtime image `aria-controller:0.2.83` / `aria-controller:local@sha256:7894cd0fc3c26ea8a4d490e2782b2a5654f7e9cbdfd3cd9d33287f0605647223`. |
| Gray backup | `/root/aria-controller/deploy-backups/20260628T024010Z-0.2.83-28308920401-8f2e45442b51-gray` |
| Master backup | `/root/aria-controller/deploy-backups/20260628T030510Z-0.2.83-28309407829-8f2e45442b51-master` |
| Agent artifact | Master Actions artifact `rust-agent-binary` from run `28309407829`, deployed to `/root/aria-controller/artifacts/aria-agent-linux-amd64` and all four online Agents. SHA256: `e9b6d77ba40f9c01ed97396a554e4675467b728923ba4c2d1b8f699282fa9695`. |
| Verification | Branch Actions run `28308920401` and master Actions run `28309407829` both passed Go Build, Frontend Build, and Rust Agent Build. Local linux/amd64 Controller/ariactl build passed, local frontend build passed, and Rust Agent artifact was taken from master Actions. Server-side `https://aria.yun/api/version` and `/api/v2/controller-info` returned `0.2.83`; `aria-controller` and `aria-frontend` were healthy; Controller logs showed `Version: 0.2.83`, HTTP ready, and gRPC one-way TLS listening on `:50051`. |
| Backup smoke | Login succeeded as `sysadmin`; backup create succeeded; dry-run restore with selected tables returned `selected_tables=["tenants","roles"]`; real restore without the confirmation phrase returned HTTP 400; the smoke backup was deleted after validation. |
| Certificate smoke | REST Agent register with CSR returned certificate material and created an `issued` node certificate. Deleting that temporary node through `DELETE /api/v2/tenants/{tenant_id}/nodes/{node_id}` changed the node to `deleted`, changed the certificate to `revoked` with reason `node deleted via API`, and created one `cert.revoked` audit event. Existing node monitoring for `node-82-156-137-42` returned certificate status `issued`. |
| Command and Agent smoke | Node `d5c7723c-3d86-48ce-a9fc-4695cd170b1c` accepted `health_check`, and the command completed. Online nodes `node-43-143-245-123`, `node-82-156-137-42`, `node-82-156-48-111`, and `node-VM-16-6-ubuntu` all reported `online`, last seen within 5 seconds, and `observed_state=applied` after Agent redeploy. |
| Deployment note | This release intentionally redeployed the Agent artifact because the protobuf registration response now carries certificate fields. Historical `deleted` duplicate hostname rows still exist in the database but do not affect the online node records used by the smoke checks. |

### 2026-06-28 Nodes Advertised Route Edit Hotfix

Status: gray deployed from `codex/ip-group-reference-closure` and server-side
smoke validated.

Purpose:

- Fix the Nodes edit dialog so advertised route changes are synchronized through
  the dedicated node route API instead of being dropped by the generic node
  update endpoint.
- Add frontend workflow coverage and route API composable coverage so future
  changes catch this UI-to-API wiring failure.
- Publish the hotfix as `0.2.86` instead of reusing the previous deployed
  version.

| Field | Value |
| --- | --- |
| Date | 2026-06-28T13:53Z deployment; 2026-06-28T13:56Z smoke validation |
| Git commit | `8de994a27a344eb0090fe8ae4ef629898bca805c` |
| Branch CI run | `28324396372` |
| Version | `0.2.86` |
| Controller image | Local runtime image `aria-controller:0.2.86` / `aria-controller:local@sha256:beb2f2774ae4ddc324be0d81b40ad9ea789ee54e885f9623d6e2289f31e0d7bd`. |
| Frontend backup | `/root/aria-controller/frontend/dist.prev-20260628T135217Z` |
| Config backup | `/root/aria-controller/backups/pre-0.2.86-config-20260628T135217Z.tar.gz` |
| DB backup | `/root/aria-controller/backups/pre-0.2.86-20260628T135217Z.sql` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch Actions. |
| Verification | Local `go test ./...`, frontend unit tests (`202` tests), linux/amd64 Controller/ariactl build, frontend build, and `git diff --check` passed. Branch Actions run `28324396372` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.86`; `aria-controller` and `aria-frontend` were healthy; Controller logs showed `Version: 0.2.86`, HTTP ready, and gRPC one-way TLS listening on `:50051`; frontend entry returned HTTP 200 with `Cache-Control: no-store` and served `assets/Nodes-00cbd08b.js` plus `assets/useRouteApi-78f98f60.js`. |
| Route smoke | Login succeeded as `sysadmin`; selected tenant `Aria Default` and node `node-82-156-137-42`; temporary route `10.254.86.0/24` was added through `POST /api/v2/tenants/{tenant_id}/nodes/{node_id}/routes`, appeared in the route list, was deleted through `DELETE /api/v2/tenants/{tenant_id}/nodes/{node_id}/routes/{cidr}`, and no longer appeared afterward. |
| Deployment note | This is a gray deployment from the feature branch, not a `master` deployment. Merge to `master` should still follow the normal confirmation gate. |

### 2026-06-28 Nodes Advertised Route Edit Follow-up

Status: gray deployed from `codex/ip-group-reference-closure` and server-side
smoke validated.

Purpose:

- Fix the remaining Nodes edit edge case where a user typed a new advertised
  route and clicked Save without pressing Enter first. The edit dialog now
  commits the pending route input before synchronizing route diffs.
- Add a frontend version watcher so an already-open console tab reloads after a
  deployed version changes instead of continuing to run stale Vite chunks.
- Publish the follow-up as `0.2.87` because `0.2.86` had the route API wiring
  fix deployed, but already-open browser tabs could still be running old
  JavaScript and never issue the route API request.

| Field | Value |
| --- | --- |
| Date | 2026-06-28T22:31+08:00 deployment; 2026-06-28T22:34+08:00 smoke validation |
| Git commit | `47498f375f2bbbcf5eff0d1c08279c8153e70dae` |
| Branch CI run | `28325437321` |
| Version | `0.2.87` |
| Controller image | Local runtime image `aria-controller:0.2.87` / `aria-controller:local@sha256:5c6c9c7347a9fec3fbab113c7c9f32f7c0ef48da37785eb9a2e3668da39a3296`. |
| Backup | `/root/aria-controller/deploy-backups/20260628223112` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch Actions. |
| Verification | Local focused route-input regression test passed, local version watcher tests passed, local `npm run type-check` passed, full frontend unit tests passed (`204` tests), frontend build passed, linux/amd64 Controller/ariactl build passed, and `git diff --check` passed. Branch Actions run `28325437321` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.87`; `aria-controller` and `aria-frontend` were healthy; frontend entry returned HTTP 200 with `Cache-Control: no-store` and served `assets/index-13a4d431.js`, `assets/Nodes-5b8f9205.js`, and `assets/useRouteApi-fea59ada.js`. The deployed entry bundle contains `startVersionWatcher` and `reloadOnChange`. |
| Route smoke | Login succeeded as `sysadmin`; selected tenant `Aria Default` and node `node-82-156-48-111`; temporary route `10.254.87.0/24` was added through `POST /api/v2/tenants/{tenant_id}/nodes/{node_id}/routes`, appeared in the route list, was deleted through `DELETE /api/v2/tenants/{tenant_id}/nodes/{node_id}/routes/{cidr}`, and no longer appeared afterward. |
| Deployment note | This is a gray deployment from the feature branch, not a `master` deployment. Users with a tab that was opened before `0.2.87` may still need one manual refresh; once the `0.2.87` entry is loaded, later version changes trigger the frontend watcher. |

### 2026-06-28 Focused Status Polling Gray Deployment

Status: gray deployed from `codex/focused-status-polling` and server-side smoke
validated.

Purpose:

- Add focused control-loop polling so policy and node status rows update from
  lightweight status endpoints instead of waiting for manual full-page refresh.
- Add tenant-scoped status endpoints for explicit policy delivery refs and node
  IDs.
- Keep the deployment on the low-bandwidth Controller/frontend path because the
  release changes Controller API and frontend code only, not Rust Agent/eBPF
  runtime behavior.

| Field | Value |
| --- | --- |
| Date | 2026-06-28T23:25+08:00 deployment; 2026-06-28T23:30+08:00 smoke validation |
| Git commit | `1fb5d75c8ec787d7857148d76ed6f88cce47046b` |
| Branch CI run | `28326752826` |
| Version | `0.2.88` |
| Controller image | Local runtime image `aria-controller:0.2.88` / `aria-controller:local@sha256:274abced48d991e241d28605f2339b54aeebc3e6b0e33ce728b5adadfe8ba069`. |
| Backup | `/root/aria-controller/deploy-backups/20260628T152550Z-0.2.88-28326752826-1fb5d75c8ec7` |
| Release path | `/root/aria-controller/releases/0.2.88-20260628T152550Z-0.2.88-28326752826-1fb5d75c8ec7` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch Actions. |
| Verification | Local focused backend tests passed, local `go test ./internal/api/v2 ./pkg/controllerstorage` passed, local `go test ./...` passed, local frontend type-check passed, targeted frontend status polling tests passed, full frontend unit tests passed, local frontend build passed, linux/amd64 Controller/ariactl build passed, and `git diff --check` passed. Branch Actions run `28326752826` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.88`; `aria-controller` and `aria-frontend` were healthy; deployed frontend assets contain `policy-deliveries/status` and `nodes/status`. |
| Status endpoint smoke | Login succeeded as `sysadmin`; `POST /api/v2/tenants/{tenant_id}/nodes/status` returned HTTP 200 for live node `554ec635-7267-4771-b3a5-9d174350a954` with `status=online`, `configuration_status=applied`, `convergence_status=converged`, and `pending_cmds=0`; `POST /api/v2/tenants/{tenant_id}/policy-deliveries/status` returned HTTP 200 for latest route delivery `1.1.1.1/32` with `policy_status=applied` and `pending_cmds=0`. |
| Focused polling smoke | Temporary route `10.255.188.128/32` on node `53ac38c9-f8ff-475d-83aa-1ca80cbdbdd9` returned `create_http=200`; the focused delivery status endpoint observed `pending` then `applied/completed`; deleting the route returned `delete_http=200`; the focused delivery status endpoint again observed `pending` then `applied/completed`; final route list confirmed `cleanup_route_exists=false`. |
| Deployment note | This is a gray deployment from the feature branch, not a `master` deployment. Browser DevTools network tracing was not run in this environment; page-level request tracing can be checked manually if needed, while the deployed bundle and API smoke confirm the focused endpoint path is active. |

### 2026-06-29 Focused Status Polling Master Deployment

Status: deployed from `master` and server-side smoke validated.

Purpose:

- Bring `master` back in sync with the already-validated `0.2.88` gray
  deployment.
- Include the i18n follow-up task record and focused status polling deployment
  record in the main branch.
- Rebuild and redeploy Controller/frontend from current `master`; Agent artifact
  remains unchanged because this release does not change Agent runtime behavior.

| Field | Value |
| --- | --- |
| Date | 2026-06-29T23:52+08:00 deployment; 2026-06-29T23:54+08:00 smoke validation |
| Git commit | `56bf947390af12b6061c68455ab9494327f0f6ed` |
| Master CI run | `28384372597` |
| Version | `0.2.88` |
| Controller image | Local runtime image `aria-controller:0.2.88` / `aria-controller:local@sha256:dbe313d38de9871ff9adca2b706d632092b6a2e7707cbf93f91a8bcad8c2e55d`. |
| Backup | `/root/aria-controller/deploy-backups/20260629T155222Z-0.2.88-28384372597-56bf947390af-master` |
| Release path | `/root/aria-controller/releases/0.2.88-20260629T155222Z-0.2.88-28384372597-56bf947390af-master` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in master Actions. |
| Verification | Master Actions run `28384372597` passed Go Build, Frontend Build, and Rust Agent Build. Local linux/amd64 Controller/ariactl build passed, and frontend build passed from current `master`. Server-side `https://aria.yun/api/version` returned `0.2.88`; frontend entry returned HTTP 200; `aria-controller` and `aria-frontend` were healthy; deployed frontend assets contain `policy-deliveries/status` and `nodes/status`. |
| Status endpoint smoke | Login succeeded as `sysadmin`; `POST /api/v2/tenants/{tenant_id}/nodes/status` returned HTTP 200 for live node `53ac38c9-f8ff-475d-83aa-1ca80cbdbdd9` with `status=online`, `configuration_status=applied`, `convergence_status=converged`, and `pending_cmds=0`. |
| Focused polling smoke | Temporary route `10.255.189.150/32` on node `554ec635-7267-4771-b3a5-9d174350a954` returned `create_http=200`; the focused delivery status endpoint observed `pending` then `applied/completed`; deleting the route returned `delete_http=200`; the focused delivery status endpoint again observed `pending` then `applied/completed`; final route list confirmed `cleanup_route_exists=false`. |
| Deployment note | This is the master deployment that supersedes the `codex/focused-status-polling` gray deployment. This docs-only deployment record commit does not require another runtime redeploy. |

### 2026-07-01 Bug 38-57 Hardening Master Deployment

Status: deployed from `master` and server-side smoke validated.

Purpose:

- Close the BUG-38 to BUG-57 hardening batch across Controller, frontend, and
  Rust Agent surfaces.
- Publish the runtime-sensitive fixes under a unique patch version instead of
  reusing `0.2.88`.
- Deploy Controller, frontend, and the online Agent artifact so the running
  system matches `master`.

| Field | Value |
| --- | --- |
| Date | 2026-07-01 master deployment and smoke validation |
| Git commit | `2887daf54f33c97752f13dfac2b9b5f27b9fbd80` |
| Branch CI run | `28523152622` |
| Branch workflow_dispatch | `28523600762` |
| Master CI run | `28524622363` |
| Master workflow_dispatch | `28525088215` |
| Version | `0.2.89` |
| Controller image | Local runtime image `aria-controller:0.2.89` / image id `sha256:dfe3b5191d9e7a6d475314b0c3510a806b26e0e3c4bf847dd4220d5f05b4f7bc`. |
| Agent artifact | Master Actions artifact deployed to the online Agent `82.156.48.111`; `/usr/local/bin/aria-agent` SHA256 `f871f43ff247fde04183f6cef40a77da4fe1511200a8472bcea409562e1fd729`. The Controller-hosted Agent artifact was also updated for future installs. |
| Verification | Branch and master Actions both passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.89`; `aria-controller` and `aria-frontend` were healthy; login, tenant listing, Nodes, ACL, QoS, Settings Backup, and `health_check` smoke checks passed. The online Agent service was active and logs showed command stream connected plus immediate sync completed. |
| Deployment note | Other historical Agent records were stale or unreachable during this deployment window, so only the confirmed online Agent was upgraded directly. |

### 2026-07-01 Policy Workbench Reference Gray Deployment

Status: gray deployed from `codex/policy-workbench-unification` and
server-side smoke validated.

Purpose:

- Align IP Group reference links with the current policy workbench routes.
- Keep policy context navigation stable by preferring named frontend routes
  with `rule_id` and `node_id` query parameters.
- Add focused regression coverage around IP Group references, route context,
  focused polling, Settings Backup, Agent command status, and route edits.

| Field | Value |
| --- | --- |
| Date | 2026-07-01T15:38Z deployment; 2026-07-01T15:40Z smoke validation |
| Git commit | `249ba299db1b1832790e177032be13b30bd4b5cb` |
| Branch CI run | `28528858536` |
| Version | `0.2.90` |
| Controller image | Local runtime image `aria-controller:0.2.90` / `aria-controller:local@sha256:cb47251470a66420dc46920774aa28d676ea4ab4cce60950fd083fe649128de8`. |
| Backup | `/root/aria-controller/deploy-backups/20260701T153655Z-0.2.90-28528858536-249ba29` |
| Agent artifact | Not changed on servers; Rust Agent Build passed in branch Actions. |
| Verification | Local targeted Go and frontend regression tests passed, local `go test ./...` passed, full frontend unit tests passed (`217` tests), frontend type-check passed, and `git diff --check` passed before push. Branch Actions run `28528858536` passed Go Build, Frontend Build, and Rust Agent Build. Server-side `https://aria.yun/api/version` returned `0.2.90`; frontend entry returned HTTP 200; `aria-controller` and `aria-frontend` were healthy; login succeeded as `sysadmin`; tenant scan found `Aria Default` with 4 nodes and 21 IP Groups. IP Group reference smoke confirmed ACL links use `/policy-center/acl-rules`, QoS links use `/policy-center/bandwidth-control`, and both include `route.name`, `rule_id`, `node_id`, and latest delivery status. |
| Deployment note | This is a gray deployment from the feature branch, not a `master` deployment. Merge to `master`, run master Actions, and redeploy master artifacts after gray confirmation. |

## Notes

- `deployments/ansible/roles/controller/templates/docker-compose.yml.j2` mirrors
  the production compose shape.
- Do not `docker cp` a Controller binary into a running container for normal
  deploys. It is not durable across container recreation and makes rollback
  unclear. Always rebuild the runtime image from the uploaded binary.
- Build Controller runtime images from `/root/aria-controller/runtime-build`,
  not `/root/aria-controller`. The small context prevents `.env`, certs, logs,
  backups, and other host state from entering the Docker build context.
- `deployments/ansible/playbooks/deploy-frontend.yml` expects prebuilt frontend
  files. Those files can come from local `npm run build` or from GitHub Actions
  when a full CI-gated release is required.
