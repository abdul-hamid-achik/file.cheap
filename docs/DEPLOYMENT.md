# Deployment Guide

This guide documents how to deploy file.cheap to production.

## Infrastructure Overview

The production environment runs on Hetzner Cloud with the following architecture:

```
                    ┌─────────────────────────────────────────┐
                    │            Load Balancer                │
                    │            91.98.4.86                   │
                    └─────────────┬───────────────────────────┘
                                  │
           ┌──────────────────────┼──────────────────────┐
           │                      │                      │
           ▼                      ▼                      ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│     Master      │   │    Worker 1     │   │    Worker 2     │
│  46.224.17.195  │   │ 167.235.128.153 │   │ 188.245.44.175  │
│    (cax11)      │   │    (cax21)      │   │    (cax21)      │
│   K3s Server    │   │   K3s Agent     │   │   K3s Agent     │
└─────────────────┘   └─────────────────┘   └─────────────────┘
```

### Kubernetes Resources

| Resource | Replicas | Description |
|----------|----------|-------------|
| `api` | 2 | HTTP API server |
| `worker` | 1 | Background job processor |
| `postgres-0` | 1 | PostgreSQL database |
| `redis-0` | 1 | Redis for job queue |
| `minio-0` | 1 | S3-compatible object storage |

## Prerequisites

### Required Tools

```bash
# Install via Homebrew (macOS)
task infra:setup

# Or manually:
brew install terraform ansible kubectl helm k9s tailscale
```

### Required Credentials

1. **Hetzner Cloud API Token** - From https://console.hetzner.cloud
2. **SSH Key** - For server access
3. **GitHub PAT** - With `write:packages` scope for container registry

### Configuration Files

1. `infrastructure/terraform/terraform.tfvars` - Hetzner credentials
2. `~/.kube/file-cheap` - Kubernetes config (auto-generated)
3. `.env` - Application secrets

## Quick Deploy (Ship Command)

If infrastructure is already set up:

```bash
# Full pipeline: test, build, push, deploy
task ship
```

This command:
1. Runs all tests
2. Builds Docker images for API and worker
3. Pushes images to ghcr.io
4. Applies Kubernetes manifests
5. Restarts deployments
6. Waits for rollout to complete

## Manual Deployment Steps

### 1. Fetch Kubeconfig (First Time Only)

```bash
# SSH to master and fetch kubeconfig
ssh root@46.224.17.195 "cat /etc/rancher/k3s/k3s.yaml" | \
  sed 's/127.0.0.1/46.224.17.195/g' > ~/.kube/file-cheap

chmod 600 ~/.kube/file-cheap
```

### 2. Verify Cluster Connection

```bash
kubectl get nodes --kubeconfig ~/.kube/file-cheap
```

Expected output:
```
NAME                  STATUS   ROLES                  AGE   VERSION
file-cheap-master     Ready    control-plane,master   Xh    v1.30.0+k3s1
file-cheap-worker-1   Ready    <none>                 Xh    v1.30.0+k3s1
file-cheap-worker-2   Ready    <none>                 Xh    v1.30.0+k3s1
```

### 3. Login to Container Registry

```bash
# Using GitHub CLI token
echo $(gh auth token) | docker login ghcr.io -u YOUR_USERNAME --password-stdin

# Or using PAT from cluster secret
kubectl get secret ghcr-secret -n file-processor --kubeconfig ~/.kube/file-cheap \
  -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d | \
  jq -r '.auths["ghcr.io"].auth' | base64 -d
# Output: username:token
```

### 4. Build Docker Images

```bash
# Build API
docker build -t ghcr.io/abdul-hamid-achik/file.cheap/api:latest --target api .

# Build Worker
docker build -t ghcr.io/abdul-hamid-achik/file.cheap/worker:latest --target worker .
```

### 5. Push Images

```bash
docker push ghcr.io/abdul-hamid-achik/file.cheap/api:latest
docker push ghcr.io/abdul-hamid-achik/file.cheap/worker:latest
```

### 6. Deploy to Kubernetes

```bash
# Apply manifests
kubectl apply -k infrastructure/k8s/overlays/production --kubeconfig ~/.kube/file-cheap

# Restart deployments to pull new images
kubectl rollout restart deploy/api deploy/worker -n file-processor --kubeconfig ~/.kube/file-cheap

# Wait for rollout
kubectl rollout status deploy/api -n file-processor --kubeconfig ~/.kube/file-cheap --timeout=180s
kubectl rollout status deploy/worker -n file-processor --kubeconfig ~/.kube/file-cheap --timeout=180s
```

### 7. Verify Deployment

```bash
# Check pods
kubectl get pods -n file-processor --kubeconfig ~/.kube/file-cheap

# Check site
curl -I https://file.cheap
```

## Database Migrations

Migrations are automatically run during `task ship`. You can also run them manually.

### Adding New Migrations

**IMPORTANT:** When creating new migrations, update BOTH:

1. Create the migration file: `migrations/NNN_description.sql`
2. Add to `Taskfile.yml` `migrate` task for local development

Production's `prod:migrate` task automatically runs all `migrations/*.sql` files.

### Run Migrations on Production

```bash
# Run all pending migrations (preferred)
task prod:migrate

# Run a single migration
task prod:migrate:single -- 012_new_feature.sql

# Or manually
cat migrations/007_image_presets.sql | \
  kubectl exec -i postgres-0 -n file-processor --kubeconfig ~/.kube/file-cheap -- \
  psql -U fileprocessor -d fileprocessor
```

### Connect to Production Database

```bash
# Interactive psql session
task prod:psql

# Or manually
kubectl exec -it postgres-0 -n file-processor --kubeconfig ~/.kube/file-cheap -- \
  psql -U fileprocessor -d fileprocessor
```

## Infrastructure Management

### Initial Setup (First Time)

```bash
# 1. Copy and configure terraform variables
cp infrastructure/terraform/terraform.tfvars.example infrastructure/terraform/terraform.tfvars
# Edit terraform.tfvars with your Hetzner token and SSH key

# 2. Initialize terraform
task infra:init

# 3. Provision infrastructure
task infra:up

# 4. Fetch kubeconfig
task infra:kubeconfig
```

### Scale Deployments

```bash
# Scale API
task scale:api -- 3

# Scale Worker
task scale:worker -- 2
```

### View Logs

```bash
# All production logs
task prod:logs

# API logs only
task prod:logs:api

# Worker logs only
task prod:logs:worker
```

### Rollback

```bash
# Rollback API
task rollback:api

# Rollback Worker
task rollback:worker
```

### SSH to Servers

```bash
# Master node
task ssh

# Worker nodes
task ssh:worker -- 1
task ssh:worker -- 2
```

## Monitoring

### Grafana Dashboard

```bash
# Port forward Grafana
task prod:grafana
# Open http://localhost:3001
```

Or access directly: https://grafana.file.cheap

### Prometheus

```bash
# Check metrics endpoint
task metrics
```

### Cluster Health

```bash
# Quick health check
task status

# Detailed diagnostics
task wtf
```

## Secrets Management

### View Current Secrets

```bash
task secrets:show
```

### Update Secrets

```bash
# Create secrets from .env.production
task secrets:create

# Or manually
kubectl create secret generic app-secrets \
  --from-env-file=.env.production \
  -n file-processor \
  --kubeconfig ~/.kube/file-cheap \
  --dry-run=client -o yaml | kubectl apply -f -
```

## DNS Configuration

The following DNS records should point to the load balancer:

| Record | Type | Value | Purpose |
|--------|------|-------|---------|
| `file.cheap` | A | 91.98.4.86 | Web UI |
| `api.file.cheap` | A | 91.98.4.86 | REST API (`https://api.file.cheap/v1/...`) |
| `grafana.file.cheap` | A | 46.224.17.195 | Monitoring dashboard |

## Troubleshooting

### Pods Not Starting

```bash
# Check pod events
kubectl describe pod <pod-name> -n file-processor --kubeconfig ~/.kube/file-cheap

# Check logs
kubectl logs <pod-name> -n file-processor --kubeconfig ~/.kube/file-cheap
```

### Image Pull Errors

```bash
# Verify registry secret
kubectl get secret ghcr-secret -n file-processor --kubeconfig ~/.kube/file-cheap

# Recreate if needed
kubectl delete secret ghcr-secret -n file-processor --kubeconfig ~/.kube/file-cheap
kubectl create secret docker-registry ghcr-secret \
  --docker-server=ghcr.io \
  --docker-username=YOUR_USERNAME \
  --docker-password=YOUR_PAT \
  -n file-processor \
  --kubeconfig ~/.kube/file-cheap
```

### Database Connection Issues

```bash
# Check postgres is running
kubectl get pods -n file-processor --kubeconfig ~/.kube/file-cheap | grep postgres

# Check postgres logs
kubectl logs postgres-0 -n file-processor --kubeconfig ~/.kube/file-cheap
```

### SSL Certificate Issues

```bash
# Check cert-manager
kubectl get certificates -n file-processor --kubeconfig ~/.kube/file-cheap

# Check certificate status
kubectl describe certificate file-cheap-tls -n file-processor --kubeconfig ~/.kube/file-cheap
```

## Useful Commands Reference

```bash
# Cluster
task status              # Health dashboard
task wtf                 # Debug everything
task k9s                 # Open k9s UI

# Deployment
task ship                # Full deploy pipeline
task deploy              # Deploy only (no build)
task deploy:api          # Deploy API only
task deploy:worker       # Deploy worker only

# Scaling
task scale:api -- N      # Scale API to N replicas
task scale:worker -- N   # Scale worker to N replicas

# Logs
task prod:logs           # All logs
task prod:logs:api       # API logs
task prod:logs:worker    # Worker logs

# Database
task prod:psql           # Connect to postgres
task prod:redis          # Connect to redis

# Monitoring
task prod:grafana        # Port-forward Grafana
task mon                 # Open Grafana in browser

# SSH
task ssh                 # SSH to master
task ssh:worker -- 1     # SSH to worker 1

# Rollback
task rollback:api        # Rollback API
task rollback:worker     # Rollback worker
```

## Cost Estimate

| Resource | Type | Monthly Cost |
|----------|------|--------------|
| Master | cax11 (2 vCPU, 4GB ARM) | ~€4 |
| Worker x2 | cax21 (4 vCPU, 8GB ARM) | ~€12 |
| Load Balancer | lb11 | ~€5 |
| Volumes (80GB total) | - | ~€4 |
| **Total** | | **~€25/month** |
