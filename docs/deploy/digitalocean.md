# Deploying memro.co to DigitalOcean

This guide shows you how to deploy memro.co on a DigitalOcean Droplet with Managed PostgreSQL.

## Prerequisites

- DigitalOcean account
- `doctl` CLI installed
- Domain name (optional, for HTTPS)

## Architecture

```
┌─────────────────────────────────────────┐
│         DigitalOcean Droplet            │
│  (Docker + Docker Compose)              │
│                                         │
│  ┌──────────┐  ┌──────────┐            │
│  │ Backend  │  │  Qdrant  │            │
│  │  (Rust)  │  │ (Vector) │            │
│  └──────────┘  └──────────┘            │
└────────┬────────────────────────────────┘
         │
         │ PostgreSQL Connection
         │
┌────────▼────────────────────────────────┐
│   Managed PostgreSQL Database           │
│   (DigitalOcean Managed DB)             │
└─────────────────────────────────────────┘
```

## Step 1: Create Managed PostgreSQL Database

```bash
# Create database cluster
doctl databases create memro-db \
  --engine pg \
  --region nyc3 \
  --size db-s-1vcpu-1gb

# Get connection string
doctl databases connection memro-db
```

Save the connection string - you'll need it for `.env`

## Step 2: Create Droplet

```bash
# Create Ubuntu droplet
doctl compute droplet create memro-prod \
  --region nyc3 \
  --size s-2vcpu-4gb \
  --image ubuntu-22-04-x64 \
  --ssh-keys YOUR_SSH_KEY_ID

# Get droplet IP
doctl compute droplet list
```

## Step 3: SSH into Droplet and Install Docker

```bash
ssh root@YOUR_DROPLET_IP

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Install Docker Compose
apt-get update
apt-get install docker-compose-plugin -y

# Verify installation
docker --version
docker compose version
```

## Step 4: Deploy memro.co

```bash
# Clone repository
git clone https://github.com/memro-co/memro.git
cd memro

# Create .env file
cat > .env << EOF
DATABASE_URL=postgresql://USER:PASS@HOST:PORT/memro
QDRANT_URL=http://qdrant:6333
PORT=8081
RUST_LOG=info
EOF

# Start services
docker compose up -d

# Verify it's running
curl http://localhost:8081/health
```

## Step 5: Configure Firewall

```bash
# Allow HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 8081/tcp

# Enable firewall
ufw enable
```

## Step 6: Setup Nginx Reverse Proxy (Optional)

```bash
# Install Nginx
apt-get install nginx -y

# Create config
cat > /etc/nginx/sites-available/memro << EOF
server {
    listen 80;
    server_name memro.yourdomain.com;

    location / {
        proxy_pass http://localhost:8081;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }
}
EOF

# Enable site
ln -s /etc/nginx/sites-available/memro /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx
```

## Step 7: Setup SSL with Let's Encrypt (Optional)

```bash
# Install certbot
apt-get install certbot python3-certbot-nginx -y

# Get certificate
certbot --nginx -d memro.yourdomain.com

# Auto-renewal is configured automatically
```

## Step 8: Verify Deployment

```bash
# Test from outside
curl https://memro.yourdomain.com/health

# Check logs
docker compose logs -f backend
```

## Monitoring

```bash
# View resource usage
docker stats

# View logs
docker compose logs -f

# Restart services
docker compose restart
```

## Backup Strategy

### Database Backups
DigitalOcean Managed PostgreSQL includes automatic daily backups.

### Manual Backup
```bash
# Backup database
pg_dump $DATABASE_URL > backup_$(date +%Y%m%d).sql

# Upload to Spaces (DigitalOcean S3)
s3cmd put backup_*.sql s3://your-bucket/backups/
```

## Scaling

### Vertical Scaling (Bigger Droplet)
```bash
# Resize droplet
doctl compute droplet-action resize DROPLET_ID --size s-4vcpu-8gb
```

### Horizontal Scaling (Load Balancer)
```bash
# Create load balancer
doctl compute load-balancer create \
  --name memro-lb \
  --region nyc3 \
  --forwarding-rules entry_protocol:http,entry_port:80,target_protocol:http,target_port:8081
```

## Cost Estimate

- **Droplet** (2 vCPU, 4GB RAM): $24/month
- **Managed PostgreSQL** (1 vCPU, 1GB RAM): $15/month
- **Total**: ~$39/month

For production, recommend:
- **Droplet**: $48/month (4 vCPU, 8GB RAM)
- **Database**: $30/month (2 vCPU, 2GB RAM)
- **Total**: ~$78/month

## Troubleshooting

### Backend won't start
```bash
# Check logs
docker compose logs backend

# Verify database connection
docker compose exec backend env | grep DATABASE_URL
```

### Out of memory
```bash
# Check memory usage
free -h

# Resize droplet or add swap
fallocate -l 2G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
```

## Support

- GitHub Issues: https://github.com/memro-co/memro/issues
- Discord: https://discord.gg/memro
