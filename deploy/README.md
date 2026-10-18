# memro Infrastructure

Infrastructure as Code (IaC) and deployment configurations for the memro ecosystem.

## Overview

This repository contains:
- **Docker**: Production-ready Dockerfiles
- **Kubernetes**: K8s manifests for cloud deployment
- **Terraform**: Infrastructure provisioning for AWS/GCP/Azure
- **Monitoring**: Observability and monitoring configs
- **CI/CD**: GitHub Actions workflows

## Quick Start

### Docker Compose (Development)

```bash
# Use the root docker-compose.yml
cd /Users/freya/Documents/work/hackit/memrohq
docker-compose up -d
```

### Kubernetes (Production)

```bash
# Apply Kubernetes manifests
kubectl apply -f kubernetes/namespace.yaml
kubectl apply -f kubernetes/postgres.yaml
kubectl apply -f kubernetes/backend.yaml
kubectl apply -f kubernetes/ingress.yaml
```

### Terraform (Cloud Provisioning)

```bash
cd terraform/aws  # or gcp, azure
terraform init
terraform plan
terraform apply
```

## Directory Structure

```
memro-infra/
├── docker/              # Production Dockerfiles
│   ├── backend/
│   ├── frontend-developer/
│   └── landing/
├── kubernetes/          # K8s manifests
│   ├── namespace.yaml
│   ├── postgres.yaml
│   ├── backend.yaml
│   ├── frontend.yaml
│   └── ingress.yaml
├── terraform/           # Infrastructure as Code
│   ├── aws/
│   ├── gcp/
│   └── azure/
├── monitoring/          # Observability configs
│   ├── prometheus/
│   ├── grafana/
│   └── loki/
├── ci-cd/              # CI/CD pipelines
│   └── github-actions/
└── README.md
```

## Deployment Options

### Option 1: Docker Compose (Simplest)

**Best for**: Development, small deployments, testing

```bash
docker-compose up -d
```

**Pros**: Easy to set up, minimal configuration
**Cons**: Not suitable for production scale

---

### Option 2: Kubernetes (Recommended for Production)

**Best for**: Production, scalability, high availability

```bash
kubectl apply -k kubernetes/
```

**Pros**: Auto-scaling, self-healing, industry standard
**Cons**: More complex setup

**Requirements**:
- Kubernetes cluster (GKE, EKS, AKS, or self-hosted)
- kubectl configured
- Ingress controller (nginx, traefik)

---

### Option 3: Managed Cloud (Coming Soon)

**Best for**: No infrastructure management

Visit https://memro.co/pricing for managed hosting options.

---

## Configuration

### Environment Variables

Create a `.env` file or use Kubernetes secrets:

```bash
# Required
OPENAI_API_KEY=sk-...
DATABASE_URL=postgres://user:pass@host:5432/memro  # Postgres with the pgvector extension

# Optional
LOG_LEVEL=info
PORT=8081
```

### Secrets Management

#### Kubernetes Secrets

```bash
kubectl create secret generic memro-secrets \
  --from-literal=openai-api-key=$OPENAI_API_KEY \
  --from-literal=database-url=$DATABASE_URL
```

#### AWS Secrets Manager (Terraform)

```hcl
resource "aws_secretsmanager_secret" "memro_secrets" {
  name = "memro/production"
}
```

---

## Monitoring & Observability

### Prometheus + Grafana

```bash
# Deploy monitoring stack
kubectl apply -f monitoring/prometheus/
kubectl apply -f monitoring/grafana/

# Access Grafana
kubectl port-forward svc/grafana 3000:3000
# Open http://localhost:3000
```

### Metrics Exposed

- **Backend API**: `/metrics` (Prometheus format)
- **PostgreSQL**: via postgres_exporter

### Dashboards

Pre-built Grafana dashboards in `monitoring/grafana/dashboards/`:
- `memro-overview.json` - System overview
- `memro-api.json` - API performance
- `memro-database.json` - Database metrics

---

## Scaling

### Horizontal Scaling (Kubernetes)

```yaml
# backend deployment
spec:
  replicas: 3  # Scale to 3 instances
  
# Or use HPA (Horizontal Pod Autoscaler)
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: memro-backend-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: memro-backend
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Database Scaling

- **Vertical**: Increase PostgreSQL resources
- **Horizontal**: Use read replicas (pg_pool, patroni)
- **Managed**: Use RDS, Cloud SQL, or Azure Database

### Vector Search Scaling

Embeddings are stored in Postgres with pgvector, so they scale (and are
backed up) with the database. Add an HNSW index once single agents hold
more than ~100k memories.

---

## Backup & Recovery

### Database Backups

```bash
# Automated backups (cron job)
kubectl apply -f kubernetes/backup-cronjob.yaml

# Manual backup
kubectl exec -it postgres-0 -- pg_dump memro > backup.sql

# Restore
kubectl exec -i postgres-0 -- psql memro < backup.sql
```

Embeddings live in the same database, so `pg_dump` backs them up too.

---

## Security

### TLS/HTTPS

```yaml
# Ingress with TLS
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: memro-ingress
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - api.memro.co
    secretName: memro-tls
```

### Network Policies

```bash
kubectl apply -f kubernetes/network-policies/
```

### Pod Security

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
```

---

## CI/CD

### GitHub Actions

Workflows in `.github/workflows/`:
- `test.yml` - Run tests on PR
- `build.yml` - Build Docker images
- `deploy-staging.yml` - Deploy to staging
- `deploy-production.yml` - Deploy to production

### Example Workflow

```yaml
name: Deploy to Production
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Deploy to K8s
        run: |
          kubectl apply -k kubernetes/
```

---

## Cost Optimization

### Development

- Use Docker Compose locally
- Single-node Kubernetes (minikube, k3s)

### Production (Small)

- **Compute**: 2-4 vCPUs, 8-16 GB RAM
- **Database**: Managed PostgreSQL with pgvector (small instance)
- **Estimated**: $50-150/month

### Production (Medium)

- **Compute**: 8-16 vCPUs, 32-64 GB RAM
- **Database**: Managed PostgreSQL with pgvector and read replicas
- **Estimated**: $300-800/month

---

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -n memro
kubectl describe pod memro-backend-xxx
kubectl logs memro-backend-xxx
```

### Database Connection Issues

```bash
# Test connection
kubectl exec -it memro-backend-xxx -- psql $DATABASE_URL

# Check secrets
kubectl get secret memro-secrets -o yaml
```

### Performance Issues

```bash
# Check resource usage
kubectl top pods -n memro

# Check metrics
curl http://backend:8080/metrics
```

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for infrastructure contribution guidelines.

---

## License

MIT License - see LICENSE file

---

## Links

- **Main Repository**: https://github.com/memro-co/memro-core
- **Documentation**: https://docs.memro.co/infrastructure
- **Issues**: https://github.com/memro-co/memro-infra/issues
