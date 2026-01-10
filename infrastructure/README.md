# Infrastructure

This directory contains the infrastructure-as-code for deploying **file.cheap** to production on Hetzner Cloud using K3s.

## Architecture

```
                                    ┌─────────────────────────────┐
                                    │     Hetzner Load Balancer   │
                                    │        (file.cheap)         │
                                    └──────────────┬──────────────┘
                                                   │
                    ┌──────────────────────────────┼──────────────────────────────┐
                    │                              │                              │
           ┌────────▼────────┐           ┌────────▼────────┐           ┌─────────▼────────┐
           │   K3s Master    │           │   K3s Worker 1  │           │   K3s Worker 2   │
           │     (CX21)      │           │     (CX31)      │           │     (CX31)       │
           │  Control Plane  │           │   API + Apps    │           │  Workers (jobs)  │
           └─────────────────┘           └─────────────────┘           └──────────────────┘
```

## Prerequisites

1. [Hetzner Cloud account](https://console.hetzner.cloud/)
2. Domain name (file.cheap)
3. Your public IP address (for SSH/API access whitelist)

## Quick Start

```bash
# 1. Install tools
task infra:setup

# 2. Configure
cp terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform.tfvars with your Hetzner token and SSH key

# 3. Provision cluster
task infra:up

# 4. Check status
task status
```

## Directory Structure

```
infrastructure/
├── terraform/           # Infrastructure provisioning
│   ├── modules/
│   │   ├── network/     # VPC, firewall
│   │   ├── servers/     # K3s nodes
│   │   ├── volumes/     # Persistent storage
│   │   └── load-balancer/
│   ├── main.tf
│   ├── variables.tf
│   └── outputs.tf
│
├── ansible/             # Server configuration
│   ├── playbooks/       # Playbook files
│   ├── roles/           # Ansible roles
│   │   ├── common/      # Base OS config
│   │   ├── k3s-master/  # K3s server
│   │   └── k3s-worker/  # K3s agent
│   └── inventory/       # Host configuration
│
├── k8s/                 # Kubernetes manifests
│   ├── base/            # Base resources
│   │   ├── api/         # API deployment
│   │   ├── worker/      # Worker deployment
│   │   ├── postgres/    # Database
│   │   ├── redis/       # Cache/queue
│   │   ├── minio/       # Object storage
│   │   └── backup/      # Backup CronJob
│   └── overlays/
│       └── production/  # Production config
│
└── terraform.tfvars.example
```

## Configuration

### terraform.tfvars

```hcl
# Required
hcloud_token   = "your-hetzner-api-token"
ssh_public_key = "ssh-ed25519 AAAA..."

# Optional - customize as needed
cluster_name   = "file-cheap"
domain         = "file.cheap"
location       = "fsn1"
worker_count   = 2
```

### Secure Access Configuration

SSH and Kubernetes API access are secured via Hetzner Cloud firewall with IP whitelist.

Edit `terraform/terraform.tfvars` to set your allowed IPs:

```hcl
# Your static IP or range for SSH/K8s API access
ssh_allowed_ips = ["YOUR.PUBLIC.IP.ADDRESS/32"]
```

Find your public IP: `curl -s ifconfig.me`

## Commands

### Infrastructure Lifecycle

```bash
task infra:setup      # Install terraform, ansible, kubectl, etc.
task infra:init       # Initialize Terraform
task infra:plan       # Preview changes
task infra:up         # Provision everything
task infra:down       # Destroy cluster (careful!)
```

### Daily Operations

```bash
task status           # Health dashboard
task ship             # Test → Build → Push → Deploy
task deploy           # Deploy to production
task wtf              # Debug mode
task doctor           # Diagnose issues
task fix              # Restart stuck pods
```

### Logs & Access

```bash
task prod:logs        # All logs
task prod:logs:api    # API logs only
task prod:logs:worker # Worker logs only
task prod:shell       # Shell into API pod
task prod:psql        # PostgreSQL CLI
task prod:redis       # Redis CLI
```

### Scaling

```bash
task scale            # Show current scale
task scale:api -- 3   # Scale API to 3 replicas
task scale:worker -- 5 # Scale workers to 5
```

### Backups

```bash
task backup           # Run backup now
task backup:list      # List backups
task backup:restore -- backup-20240106.sql.gz
```

### Monitoring

```bash
task prod:grafana     # Port-forward Grafana to localhost:3001
task prod:minio       # Port-forward MinIO to localhost:9001
task k9s              # Open k9s cluster UI
```

## Cost Estimate

| Resource | Spec | Monthly Cost |
|----------|------|--------------|
| K3s Master | CX21 (2 vCPU, 4GB) | €4.17 |
| K3s Worker x2 | CX31 (2 vCPU, 8GB) | €13.18 |
| Load Balancer | LB11 | €5.39 |
| Volumes | ~80GB | €4.00 |
| **Total** | | **~€26.74** |

## DNS Setup

After provisioning, create these DNS records:

```
A    file.cheap        → <load_balancer_ip>
A    api.file.cheap    → <load_balancer_ip>
```

Get the IP with:
```bash
task infra:output
```

## Secrets

Create the Kubernetes secret with all required values:

```bash
# Copy the example and fill in your values
cp k8s/base/secrets/app-secrets.yaml.example k8s/base/secrets/app-secrets.yaml
# Edit with your actual secrets
vim k8s/base/secrets/app-secrets.yaml
# Apply to cluster
kubectl apply -f k8s/base/secrets/app-secrets.yaml --kubeconfig=$HOME/.kube/file-cheap
```

### Required Secrets

| Key | Description |
|-----|-------------|
| `POSTGRES_PASSWORD` | PostgreSQL password |
| `MINIO_ACCESS_KEY` | MinIO access key |
| `MINIO_SECRET_KEY` | MinIO secret key |
| `JWT_SECRET` | JWT signing secret (min 32 chars) |
| `STRIPE_SECRET_KEY` | Stripe API secret key (`sk_live_...`) |
| `STRIPE_PUBLISHABLE_KEY` | Stripe publishable key (`pk_live_...`) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret (`whsec_...`) |
| `STRIPE_PRICE_ID_PRO` | Stripe Price ID for Pro plan (`price_...`) |

### Optional Secrets

| Key | Description |
|-----|-------------|
| `GOOGLE_CLIENT_ID` | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret |
| `GITHUB_CLIENT_ID` | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth client secret |
| `SMTP_*` | Email configuration |

See `k8s/base/secrets/app-secrets.yaml.example` for the full template.

## SSL/TLS

SSL certificates are automatically provisioned via cert-manager and Let's Encrypt. The cluster issuer is configured in `k8s/base/cert-manager/cluster-issuer.yaml`.

## Backup Strategy

- **Schedule**: Daily at 3:00 AM UTC
- **Retention**: 30 days
- **Storage**: MinIO bucket (`backups/`)
- **Method**: `pg_dump` compressed with gzip

## High Availability (Future)

The current setup uses a single master node. To enable HA:

1. Update `terraform.tfvars`:
   ```hcl
   master_count = 3
   ```

2. Re-run provisioning:
   ```bash
   task infra:up
   ```

K3s will automatically configure embedded etcd for HA.

## Secure Access Options

The infrastructure uses Hetzner Cloud firewall to restrict SSH and Kubernetes API access to whitelisted IPs. Here are alternatives if you need more flexibility:

### Option 1: IP Whitelist (Default - Recommended)

Simplest approach. Set your static IP in `terraform.tfvars`:

```hcl
ssh_allowed_ips = ["YOUR.PUBLIC.IP.ADDRESS/32"]
```

**Pros**: No additional tools, native cloud firewall, zero cost
**Cons**: Requires static IP or manual updates when IP changes

### Option 2: SSH Jump Host / Bastion

Use one node as a bastion for all SSH traffic:

```bash
# Add to ~/.ssh/config
Host file-cheap-*
  ProxyJump root@<master-public-ip>
```

**Pros**: Single entry point, audit logging possible
**Cons**: Requires bastion to be always running

### Option 3: WireGuard VPN

Self-hosted VPN similar to Tailscale:

```bash
# Install on your machine and servers
apt install wireguard

# Generate keys and configure peers
wg genkey | tee privatekey | wg pubkey > publickey
```

**Pros**: Fast, modern VPN, full control
**Cons**: Manual key management, more setup

### Option 4: Cloudflare Tunnel (Zero Trust)

Use Cloudflare's zero-trust network for SSH access:

```bash
# Install cloudflared on servers
cloudflared tunnel create file-cheap
```

**Pros**: No exposed ports, browser-based SSH option
**Cons**: Depends on Cloudflare, may add latency

## Troubleshooting

### Cannot connect to cluster

```bash
task infra:kubeconfig  # Re-fetch kubeconfig
task doctor            # Run diagnostics
```

### Pods not starting

```bash
task wtf               # Show events and errors
kubectl describe pod <pod-name> -n file-processor --kubeconfig=$HOME/.kube/file-cheap
```

### SSL certificate issues

```bash
kubectl get certificate -n file-processor --kubeconfig=$HOME/.kube/file-cheap
kubectl describe certificate file-cheap-tls -n file-processor --kubeconfig=$HOME/.kube/file-cheap
```

### Database issues

```bash
task prod:psql         # Connect to database
task backup:list       # Check for backups to restore
```
