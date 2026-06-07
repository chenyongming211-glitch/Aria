# Aria Deployment

## Current Production Shape

Controller host: `8.152.163.101`

Production runs Aria in isolated containers under `/root/aria-controller`.
The server also hosts other products, so Aria must keep its own containers,
ports, volumes, and network.

| Service | Container | Image | Ports |
| --- | --- | --- | --- |
| Frontend | `aria-frontend` | `nginx:1.27-alpine` | `18080:80` |
| Controller | `aria-controller` | `ghcr.io/chenyongming211-glitch/aria-controller:0.2.35-test` | `50051:50051` |
| Postgres | `aria-postgres` | `postgres:16-alpine` | `127.0.0.1:15432:5432` |
| Redis | `aria-redis` | `redis:7-alpine` | `127.0.0.1:16379:6379` |
| VictoriaMetrics | `aria-victoriametrics` | `victoriametrics/victoria-metrics:latest` | `127.0.0.1:18428:8428` |

Public HTTPS is terminated by the host Nginx at `https://aria.yun`.
The frontend container serves the GitHub Actions `frontend-dist` artifact.
The Controller image is built and pushed by GitHub Actions `workflow_dispatch`.
The frontend Nginx config must serve `index.html` with `Cache-Control: no-store`
so browsers do not keep an old Vite entrypoint after a deploy. Hashed assets
under `/assets/` can use long immutable caching.

## Release Flow

Do not build release binaries or Docker images locally. Use GitHub Actions.

1. Merge the release branch into `master`.
2. Confirm the push-triggered `Build` workflow passes Go, Rust Agent, and Frontend jobs.
3. Trigger `workflow_dispatch` for the `Build` workflow on `master`.
4. Confirm `Docker Build & Push` succeeds and publishes:
   - `ghcr.io/chenyongming211-glitch/aria-controller:latest`
   - `ghcr.io/chenyongming211-glitch/aria-controller:<VERSION>`
5. Download the `frontend-dist` workflow artifact from the same run.
6. Deploy the Controller image and frontend artifact to `/root/aria-controller`.

Useful commands:

```bash
gh workflow run Build --repo chenyongming211-glitch/Aria --ref master
gh run download <run-id> --repo chenyongming211-glitch/Aria -n frontend-dist -D /tmp/aria-frontend-dist
```

## Server Layout

```text
/root/aria-controller/
├── docker-compose.yml
├── .env
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
```

`disabled` is only for local or one-off plaintext testing. Do not use it on the
production Controller while Agents are configured with `https://`.

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

Pull and apply the Controller image:

```bash
cd /root/aria-controller
docker pull ghcr.io/chenyongming211-glitch/aria-controller:0.2.35-test
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

Upload the downloaded Actions artifact and restart the frontend container:

```bash
ssh root@8.152.163.101 'rm -rf /root/aria-controller/frontend/dist.new && mkdir -p /root/aria-controller/frontend/dist.new'
rsync -az --delete /tmp/aria-frontend-dist/ root@8.152.163.101:/root/aria-controller/frontend/dist.new/
ssh root@8.152.163.101 '
  set -e
  cd /root/aria-controller/frontend
  rm -rf dist.previous
  [ -d dist ] && mv dist dist.previous
  mv dist.new dist
  docker restart aria-frontend
'
```

## Smoke Checks

```bash
ssh root@8.152.163.101 'docker ps --format "{{.Names}}\t{{.Image}}\t{{.Status}}" | grep aria'
ssh root@8.152.163.101 'curl -fsS http://127.0.0.1:18080/api/version'
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

Status: pending deployment.

Purpose:

- Deploy the node public IP correction so SaaS inventory records the true public
  IP and VPN IP only.
- Expected node identity after Agent sync:
  - `public_ip = 82.156.48.111`
  - `assigned_ip = 100.64.0.2`
  - `private_ip = ''`
  - `endpoint = 82.156.48.111:51820`

Fill these after deployment:

| Field | Value |
| --- | --- |
| Date | TBD |
| Git commit | TBD |
| Push CI run | TBD |
| Publish run | TBD |
| Controller image | TBD |
| Frontend backup | TBD |
| Config backup | TBD |
| DB backup | TBD |
| Agent artifact | TBD |
| Verification | TBD |

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

## Notes

- `deployments/ansible/roles/controller/templates/docker-compose.yml.j2` mirrors
  the production compose shape.
- `deployments/ansible/playbooks/deploy-frontend.yml` expects a prebuilt
  `frontend-dist` artifact. It must not run `npm run build` locally for release
  deploys.
