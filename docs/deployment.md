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

mkdir -p dist/controller dist/frontend

go test ./...

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-X aria/internal/cli.Version=${VERSION} -X aria/internal/cli.commit=${COMMIT}" \
  -o dist/controller/aria \
  ./cmd

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-X main.version=${VERSION}" \
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
