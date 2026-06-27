# C5 Backend — Kubernetes (TKE) Deploy

Manifests for the C5 backend on Tencent Kubernetes Engine. Two workloads share one
image base and the same config/secret:

| Workload     | Image        | Purpose                                              | Health |
|--------------|--------------|------------------------------------------------------|--------|
| `c5-api`     | `c5-api`     | Gin HTTP API (`:8080`)                               | liveness `/api/v1/healthz`, readiness `/readyz` |
| `c5-worker`  | `c5-worker`  | asynq processor: derive media tiers, Excel export, UPLOADING reaper | `:9091/healthz`, `/readyz` |
| `c5-migrate` | `c5-api`     | one-shot `c5-api migrate` (golang-migrate to latest) | Job completion |

## Configuration contract

All config comes from env (`C5_` prefix, nested keys joined by `_`):

- **Non-secret** → `configmap.yaml` (`c5-config`)
- **Secret** → `c5-secret` (DB DSN, Redis password, JWT secret ≥32 B, COS SecretId/Key)

`secret.example.yaml` is a template with placeholders — never commit a filled copy.

## Apply order

The migrate Job **must complete before** the Deployments roll (so a new schema is
in place before any pod serves traffic). golang-migrate takes a Postgres advisory
lock, so the Job is safe to re-run.

```bash
kubectl apply -f namespace.yaml
kubectl -n c5 apply -f configmap.yaml

# create the real secret out-of-band (do NOT apply secret.example.yaml as-is)
kubectl -n c5 create secret generic c5-secret \
  --from-literal=C5_DB_DSN='postgres://USER:PASS@HOST:5432/c5?sslmode=require' \
  --from-literal=C5_REDIS_PASSWORD='...' \
  --from-literal=C5_JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=C5_COS_SECRET_ID='...' \
  --from-literal=C5_COS_SECRET_KEY='...'

kubectl -n c5 apply -f migrate-job.yaml
kubectl -n c5 wait --for=condition=complete job/c5-migrate --timeout=300s

kubectl apply -k .   # api + worker + services + HPA + ingress
```

## Notes

- **ICP 备案**: the public Ingress host must be ICP-registered before the CLB will
  serve it in China mainland — see `../cloud-resources-checklist.md`.
- **Image registry**: placeholder `ccr.ccs.tencentyun.com/c5/*` (Tencent TCR). Set
  real tags via `kustomize edit set image` or an overlay.
- **Scaling**: CPU/memory HPA included. For asynq queue-depth scaling, add a KEDA
  `ScaledObject` against the Redis queue length.
- **Probes depend on P7 code**: `/readyz` (api + worker) and the worker `:9091`
  health/metrics server are wired in the P7 observability pass.
