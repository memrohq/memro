# Deploying memro.co to AWS

This guide shows you how to deploy memro.co on AWS using ECS Fargate, RDS PostgreSQL, and Application Load Balancer.

## Architecture

```
┌─────────────────────────────────────────────────┐
│              Application Load Balancer          │
│                  (HTTPS/HTTP)                   │
└────────┬────────────────────────────────────────┘
         │
         │
┌────────▼────────────────────────────────────────┐
│           ECS Fargate Cluster                   │
│                                                 │
│  ┌──────────────┐      ┌──────────────┐        │
│  │   Backend    │      │   Qdrant     │        │
│  │  (Container) │      │ (Container)  │        │
│  └──────────────┘      └──────────────┘        │
└────────┬────────────────────────────────────────┘
         │
         │ PostgreSQL Connection
         │
┌────────▼────────────────────────────────────────┐
│           RDS PostgreSQL                        │
│        (Multi-AZ for HA)                        │
└─────────────────────────────────────────────────┘
```

## Prerequisites

- AWS CLI configured
- Docker installed locally
- AWS account with appropriate permissions

## Step 1: Create RDS PostgreSQL Database

```bash
# Create database
aws rds create-db-instance \
  --db-instance-identifier memro-db \
  --db-instance-class db.t3.micro \
  --engine postgres \
  --engine-version 15.4 \
  --master-username memro \
  --master-user-password YOUR_SECURE_PASSWORD \
  --allocated-storage 20 \
  --vpc-security-group-ids sg-XXXXXXXX \
  --db-subnet-group-name default \
  --publicly-accessible false

# Get endpoint
aws rds describe-db-instances \
  --db-instance-identifier memro-db \
  --query 'DBInstances[0].Endpoint.Address'
```

## Step 2: Create ECR Repositories

```bash
# Create repository for backend
aws ecr create-repository --repository-name memro/backend

# Get login command
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin \
  ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com
```

## Step 3: Build and Push Docker Images

```bash
# Build backend image
cd backend
docker build -t memro/backend .

# Tag and push
docker tag memro/backend:latest \
  ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com/memro/backend:latest

docker push ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com/memro/backend:latest
```

## Step 4: Create ECS Cluster

```bash
# Create cluster
aws ecs create-cluster --cluster-name memro-cluster

# Create task execution role (if not exists)
aws iam create-role \
  --role-name ecsTaskExecutionRole \
  --assume-role-policy-document file://task-execution-role.json

aws iam attach-role-policy \
  --role-name ecsTaskExecutionRole \
  --policy-arn arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy
```

## Step 5: Create Task Definition

Create `task-definition.json`:

```json
{
  "family": "memro",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "512",
  "memory": "1024",
  "executionRoleArn": "arn:aws:iam::ACCOUNT_ID:role/ecsTaskExecutionRole",
  "containerDefinitions": [
    {
      "name": "backend",
      "image": "ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com/memro/backend:latest",
      "portMappings": [
        {
          "containerPort": 8081,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {
          "name": "DATABASE_URL",
          "value": "postgresql://memro:PASSWORD@RDS_ENDPOINT:5432/memro"
        },
        {
          "name": "QDRANT_URL",
          "value": "http://localhost:6333"
        },
        {
          "name": "RUST_LOG",
          "value": "info"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/memro",
          "awslogs-region": "us-east-1",
          "awslogs-stream-prefix": "backend"
        }
      }
    },
    {
      "name": "qdrant",
      "image": "qdrant/qdrant:latest",
      "portMappings": [
        {
          "containerPort": 6333,
          "protocol": "tcp"
        }
      ]
    }
  ]
}
```

Register task definition:
```bash
aws ecs register-task-definition --cli-input-json file://task-definition.json
```

## Step 6: Create Application Load Balancer

```bash
# Create ALB
aws elbv2 create-load-balancer \
  --name memro-alb \
  --subnets subnet-XXXXXXXX subnet-YYYYYYYY \
  --security-groups sg-XXXXXXXX

# Create target group
aws elbv2 create-target-group \
  --name memro-tg \
  --protocol HTTP \
  --port 8081 \
  --vpc-id vpc-XXXXXXXX \
  --target-type ip \
  --health-check-path /health

# Create listener
aws elbv2 create-listener \
  --load-balancer-arn ALB_ARN \
  --protocol HTTP \
  --port 80 \
  --default-actions Type=forward,TargetGroupArn=TARGET_GROUP_ARN
```

## Step 7: Create ECS Service

```bash
aws ecs create-service \
  --cluster memro-cluster \
  --service-name memro-service \
  --task-definition memro \
  --desired-count 2 \
  --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[subnet-XXXXXXXX,subnet-YYYYYYYY],securityGroups=[sg-XXXXXXXX],assignPublicIp=ENABLED}" \
  --load-balancers targetGroupArn=TARGET_GROUP_ARN,containerName=backend,containerPort=8081
```

## Step 8: Configure Auto Scaling

```bash
# Register scalable target
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --resource-id service/memro-cluster/memro-service \
  --scalable-dimension ecs:service:DesiredCount \
  --min-capacity 2 \
  --max-capacity 10

# Create scaling policy
aws application-autoscaling put-scaling-policy \
  --service-namespace ecs \
  --resource-id service/memro-cluster/memro-service \
  --scalable-dimension ecs:service:DesiredCount \
  --policy-name cpu-scaling \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration file://scaling-policy.json
```

## Step 9: Setup CloudWatch Monitoring

```bash
# Create log group
aws logs create-log-group --log-group-name /ecs/memro

# Create CloudWatch dashboard
aws cloudwatch put-dashboard \
  --dashboard-name memro \
  --dashboard-body file://dashboard.json
```

## Step 10: Configure SSL/TLS

```bash
# Request certificate in ACM
aws acm request-certificate \
  --domain-name memro.yourdomain.com \
  --validation-method DNS

# Add HTTPS listener to ALB
aws elbv2 create-listener \
  --load-balancer-arn ALB_ARN \
  --protocol HTTPS \
  --port 443 \
  --certificates CertificateArn=CERT_ARN \
  --default-actions Type=forward,TargetGroupArn=TARGET_GROUP_ARN
```

## Cost Estimate

### Development
- **ECS Fargate**: ~$30/month (2 tasks, 0.5 vCPU, 1GB RAM)
- **RDS db.t3.micro**: ~$15/month
- **ALB**: ~$20/month
- **Total**: ~$65/month

### Production
- **ECS Fargate**: ~$150/month (4 tasks, 1 vCPU, 2GB RAM)
- **RDS db.t3.small (Multi-AZ)**: ~$60/month
- **ALB**: ~$20/month
- **CloudWatch**: ~$10/month
- **Total**: ~$240/month

## Monitoring

```bash
# View logs
aws logs tail /ecs/memro --follow

# Check service status
aws ecs describe-services \
  --cluster memro-cluster \
  --services memro-service

# View metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/ECS \
  --metric-name CPUUtilization \
  --dimensions Name=ServiceName,Value=memro-service \
  --start-time 2024-01-01T00:00:00Z \
  --end-time 2024-01-02T00:00:00Z \
  --period 3600 \
  --statistics Average
```

## Backup Strategy

```bash
# Enable automated RDS backups (already enabled by default)
aws rds modify-db-instance \
  --db-instance-identifier memro-db \
  --backup-retention-period 7

# Create manual snapshot
aws rds create-db-snapshot \
  --db-instance-identifier memro-db \
  --db-snapshot-identifier memro-snapshot-$(date +%Y%m%d)
```

## Troubleshooting

### Tasks failing to start
```bash
# Check task logs
aws ecs describe-tasks \
  --cluster memro-cluster \
  --tasks TASK_ARN

# View stopped tasks
aws ecs list-tasks \
  --cluster memro-cluster \
  --desired-status STOPPED
```

### Database connection issues
```bash
# Verify security group rules
aws ec2 describe-security-groups --group-ids sg-XXXXXXXX

# Test connection from ECS task
aws ecs execute-command \
  --cluster memro-cluster \
  --task TASK_ARN \
  --container backend \
  --interactive \
  --command "/bin/sh"
```

## Support

- GitHub Issues: https://github.com/memro-co/memro/issues
- Discord: https://discord.gg/memro
