# Deploying memro.co to Google Cloud Platform

This guide shows you how to deploy memro.co on GCP using Cloud Run, Cloud SQL PostgreSQL, and Cloud Load Balancing.

## Architecture

```
┌─────────────────────────────────────────────────┐
│         Cloud Load Balancer (HTTPS)             │
└────────┬────────────────────────────────────────┘
         │
         │
┌────────▼────────────────────────────────────────┐
│              Cloud Run Services                 │
│                                                 │
│  ┌──────────────┐      ┌──────────────┐        │
│  │   Backend    │      │   Qdrant     │        │
│  │  (Container) │      │ (Container)  │        │
│  └──────────────┘      └──────────────┘        │
└────────┬────────────────────────────────────────┘
         │
         │ Private IP Connection
         │
┌────────▼────────────────────────────────────────┐
│         Cloud SQL PostgreSQL                    │
│          (High Availability)                    │
└─────────────────────────────────────────────────┘
```

## Prerequisites

- GCP account with billing enabled
- `gcloud` CLI installed and configured
- Docker installed locally

## Step 1: Set Up Project

```bash
# Set project ID
export PROJECT_ID=memro-prod
export REGION=us-central1

# Create project (if needed)
gcloud projects create $PROJECT_ID

# Set active project
gcloud config set project $PROJECT_ID

# Enable required APIs
gcloud services enable \
  run.googleapis.com \
  sql-component.googleapis.com \
  sqladmin.googleapis.com \
  containerregistry.googleapis.com \
  cloudresourcemanager.googleapis.com
```

## Step 2: Create Cloud SQL PostgreSQL Instance

```bash
# Create instance
gcloud sql instances create memro-db \
  --database-version=POSTGRES_15 \
  --tier=db-f1-micro \
  --region=$REGION \
  --root-password=YOUR_SECURE_PASSWORD

# Create database
gcloud sql databases create memro \
  --instance=memro-db

# Create user
gcloud sql users create memro \
  --instance=memro-db \
  --password=YOUR_SECURE_PASSWORD

# Get connection name
gcloud sql instances describe memro-db \
  --format='value(connectionName)'
```

## Step 3: Build and Push Docker Images

```bash
# Configure Docker for GCR
gcloud auth configure-docker

# Build backend
cd backend
docker build -t gcr.io/$PROJECT_ID/memro-backend .

# Push to Container Registry
docker push gcr.io/$PROJECT_ID/memro-backend

# Build Qdrant (or use official image)
docker pull qdrant/qdrant:latest
docker tag qdrant/qdrant:latest gcr.io/$PROJECT_ID/qdrant
docker push gcr.io/$PROJECT_ID/qdrant
```

## Step 4: Deploy Backend to Cloud Run

```bash
# Get Cloud SQL connection name
export SQL_CONNECTION=$(gcloud sql instances describe memro-db \
  --format='value(connectionName)')

# Deploy backend
gcloud run deploy memro-backend \
  --image gcr.io/$PROJECT_ID/memro-backend \
  --platform managed \
  --region $REGION \
  --allow-unauthenticated \
  --set-env-vars="DATABASE_URL=postgresql://memro:PASSWORD@/memro?host=/cloudsql/$SQL_CONNECTION" \
  --set-env-vars="QDRANT_URL=http://qdrant:6333" \
  --set-env-vars="RUST_LOG=info" \
  --add-cloudsql-instances=$SQL_CONNECTION \
  --memory=1Gi \
  --cpu=1 \
  --min-instances=1 \
  --max-instances=10

# Get service URL
gcloud run services describe memro-backend \
  --region $REGION \
  --format='value(status.url)'
```

## Step 5: Deploy Qdrant to Cloud Run

```bash
# Deploy Qdrant
gcloud run deploy qdrant \
  --image gcr.io/$PROJECT_ID/qdrant \
  --platform managed \
  --region $REGION \
  --allow-unauthenticated \
  --memory=2Gi \
  --cpu=1 \
  --min-instances=1

# Get Qdrant URL
export QDRANT_URL=$(gcloud run services describe qdrant \
  --region $REGION \
  --format='value(status.url)')

# Update backend with Qdrant URL
gcloud run services update memro-backend \
  --region $REGION \
  --set-env-vars="QDRANT_URL=$QDRANT_URL"
```

## Step 6: Configure Custom Domain (Optional)

```bash
# Map custom domain
gcloud run domain-mappings create \
  --service memro-backend \
  --domain memro.yourdomain.com \
  --region $REGION

# Follow instructions to update DNS records
```

## Step 7: Set Up Cloud Load Balancer (For HTTPS)

```bash
# Create serverless NEG for Cloud Run
gcloud compute network-endpoint-groups create memro-neg \
  --region=$REGION \
  --network-endpoint-type=serverless \
  --cloud-run-service=memro-backend

# Create backend service
gcloud compute backend-services create memro-backend-service \
  --global

# Add NEG to backend service
gcloud compute backend-services add-backend memro-backend-service \
  --global \
  --network-endpoint-group=memro-neg \
  --network-endpoint-group-region=$REGION

# Create URL map
gcloud compute url-maps create memro-lb \
  --default-service memro-backend-service

# Create SSL certificate
gcloud compute ssl-certificates create memro-cert \
  --domains=memro.yourdomain.com

# Create HTTPS proxy
gcloud compute target-https-proxies create memro-https-proxy \
  --url-map=memro-lb \
  --ssl-certificates=memro-cert

# Create forwarding rule
gcloud compute forwarding-rules create memro-https-rule \
  --global \
  --target-https-proxy=memro-https-proxy \
  --ports=443
```

## Step 8: Configure Auto Scaling

```bash
# Update Cloud Run service with scaling config
gcloud run services update memro-backend \
  --region $REGION \
  --min-instances=2 \
  --max-instances=20 \
  --concurrency=80 \
  --cpu-throttling \
  --cpu-boost
```

## Step 9: Set Up Monitoring

```bash
# Create log-based metric
gcloud logging metrics create memro_errors \
  --description="Count of error logs" \
  --log-filter='resource.type="cloud_run_revision"
    resource.labels.service_name="memro-backend"
    severity>=ERROR'

# Create alerting policy
gcloud alpha monitoring policies create \
  --notification-channels=CHANNEL_ID \
  --display-name="memro High Error Rate" \
  --condition-display-name="Error rate > 10/min" \
  --condition-threshold-value=10 \
  --condition-threshold-duration=60s
```

## Step 10: Verify Deployment

```bash
# Get service URL
export SERVICE_URL=$(gcloud run services describe memro-backend \
  --region $REGION \
  --format='value(status.url)')

# Test health endpoint
curl $SERVICE_URL/health

# Test identity creation
curl -X POST $SERVICE_URL/identity
```

## Cost Estimate

### Development
- **Cloud Run**: ~$10/month (minimal traffic)
- **Cloud SQL db-f1-micro**: ~$10/month
- **Container Registry**: ~$1/month
- **Total**: ~$21/month

### Production
- **Cloud Run**: ~$50/month (moderate traffic, 2-10 instances)
- **Cloud SQL db-n1-standard-1**: ~$50/month
- **Load Balancer**: ~$20/month
- **Container Registry**: ~$5/month
- **Total**: ~$125/month

## Monitoring

```bash
# View logs
gcloud run services logs read memro-backend \
  --region $REGION \
  --limit=50

# View metrics
gcloud monitoring time-series list \
  --filter='resource.type="cloud_run_revision"' \
  --format=json

# Check service status
gcloud run services describe memro-backend \
  --region $REGION
```

## Backup Strategy

```bash
# Enable automated backups (enabled by default)
gcloud sql instances patch memro-db \
  --backup-start-time=03:00

# Create on-demand backup
gcloud sql backups create \
  --instance=memro-db \
  --description="Manual backup $(date +%Y%m%d)"

# List backups
gcloud sql backups list --instance=memro-db
```

## Scaling

### Vertical Scaling (Bigger Instances)
```bash
# Upgrade Cloud SQL
gcloud sql instances patch memro-db \
  --tier=db-n1-standard-2

# Upgrade Cloud Run
gcloud run services update memro-backend \
  --region $REGION \
  --memory=2Gi \
  --cpu=2
```

### Horizontal Scaling (Auto)
Cloud Run automatically scales based on traffic. Configure limits:

```bash
gcloud run services update memro-backend \
  --region $REGION \
  --min-instances=5 \
  --max-instances=50
```

## CI/CD with Cloud Build

Create `cloudbuild.yaml`:

```yaml
steps:
  # Build backend
  - name: 'gcr.io/cloud-builders/docker'
    args: ['build', '-t', 'gcr.io/$PROJECT_ID/memro-backend', './backend']
  
  # Push to Container Registry
  - name: 'gcr.io/cloud-builders/docker'
    args: ['push', 'gcr.io/$PROJECT_ID/memro-backend']
  
  # Deploy to Cloud Run
  - name: 'gcr.io/google.com/cloudsdktool/cloud-sdk'
    entrypoint: gcloud
    args:
      - 'run'
      - 'deploy'
      - 'memro-backend'
      - '--image'
      - 'gcr.io/$PROJECT_ID/memro-backend'
      - '--region'
      - 'us-central1'
      - '--platform'
      - 'managed'

images:
  - 'gcr.io/$PROJECT_ID/memro-backend'
```

Trigger build:
```bash
gcloud builds submit --config cloudbuild.yaml
```

## Troubleshooting

### Service won't start
```bash
# Check logs
gcloud run services logs read memro-backend \
  --region $REGION \
  --limit=100

# Check service details
gcloud run services describe memro-backend \
  --region $REGION
```

### Database connection issues
```bash
# Test Cloud SQL connection
gcloud sql connect memro-db --user=memro

# Verify Cloud SQL proxy
cloud_sql_proxy -instances=$SQL_CONNECTION=tcp:5432
```

### High latency
```bash
# Check Cloud Run metrics
gcloud monitoring time-series list \
  --filter='metric.type="run.googleapis.com/request_latencies"'

# Increase resources
gcloud run services update memro-backend \
  --region $REGION \
  --memory=2Gi \
  --cpu=2
```

## Support

- GitHub Issues: https://github.com/memro-co/memro/issues
- Discord: https://discord.gg/memro
