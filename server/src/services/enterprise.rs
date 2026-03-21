/// Enterprise features: SLA enforcement, usage tracking, and billing integration
use anyhow::Result;
use std::sync::Arc;
use uuid::Uuid;
use chrono::{Utc, Duration};
use crate::ports::TenantStore;

#[derive(Debug, Clone)]
pub struct TenantSLA {
    pub tenant_id: Uuid,
    pub tier: TenantTier,
    pub requests_per_minute: i64,
    pub requests_per_hour: i64,
    pub requests_per_day: i64,
    pub storage_gb: i64,
    pub max_agents: i64,
    pub created_at: chrono::DateTime<chrono::Utc>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TenantTier {
    Free,
    Professional,
    Enterprise,
}

impl TenantTier {
    pub fn requests_per_minute(&self) -> i64 {
        match self {
            TenantTier::Free => 60,
            TenantTier::Professional => 1000,
            TenantTier::Enterprise => 10000,
        }
    }

    pub fn requests_per_hour(&self) -> i64 {
        match self {
            TenantTier::Free => 3600,
            TenantTier::Professional => 100000,
            TenantTier::Enterprise => 1000000,
        }
    }

    pub fn requests_per_day(&self) -> i64 {
        match self {
            TenantTier::Free => 86400,
            TenantTier::Professional => 1000000,
            TenantTier::Enterprise => 10000000,
        }
    }

    pub fn storage_gb(&self) -> i64 {
        match self {
            TenantTier::Free => 1,
            TenantTier::Professional => 100,
            TenantTier::Enterprise => 10000,
        }
    }

    pub fn max_agents(&self) -> i64 {
        match self {
            TenantTier::Free => 1,
            TenantTier::Professional => 100,
            TenantTier::Enterprise => 100000,
        }
    }
}

pub struct EnterpriseManager {
    tenant_store: Arc<dyn TenantStore>,
}

impl EnterpriseManager {
    pub fn new(tenant_store: Arc<dyn TenantStore>) -> Self {
        Self { tenant_store }
    }

    /// Check if tenant is within SLA limits
    pub async fn check_sla_limit(
        &self,
        tenant_id: &Uuid,
        tier: TenantTier,
    ) -> Result<bool> {
        // Get usage in last minute
        let now = Utc::now();
        let one_minute_ago = now - Duration::minutes(1);

        let minute_usage = self.tenant_store
            .get_tenant_usage(tenant_id, "api_calls", one_minute_ago)
            .await
            .unwrap_or(0);

        if minute_usage >= tier.requests_per_minute() {
            return Ok(false);
        }

        Ok(true)
    }

    /// Record usage event for billing and enforcement
    pub async fn record_usage(
        &self,
        tenant_id: &Uuid,
        metric: &str,
        value: i64,
    ) -> Result<()> {
        self.tenant_store.record_tenant_usage(tenant_id, metric, value).await
    }

    /// Get tenant's current usage statistics
    pub async fn get_usage_stats(
        &self,
        tenant_id: &Uuid,
        tier: TenantTier,
    ) -> Result<UsageStats> {
        let now = Utc::now();
        let one_minute_ago = now - Duration::minutes(1);
        let one_hour_ago = now - Duration::hours(1);
        let one_day_ago = now - Duration::days(1);

        let minute_usage = self.tenant_store
            .get_tenant_usage(tenant_id, "api_calls", one_minute_ago)
            .await
            .unwrap_or(0);
        let hour_usage = self.tenant_store
            .get_tenant_usage(tenant_id, "api_calls", one_hour_ago)
            .await
            .unwrap_or(0);
        let day_usage = self.tenant_store
            .get_tenant_usage(tenant_id, "api_calls", one_day_ago)
            .await
            .unwrap_or(0);

        Ok(UsageStats {
            minute_usage,
            hour_usage,
            day_usage,
            minute_limit: tier.requests_per_minute(),
            hour_limit: tier.requests_per_hour(),
            day_limit: tier.requests_per_day(),
            minute_remaining: (tier.requests_per_minute() - minute_usage).max(0),
            hour_remaining: (tier.requests_per_hour() - hour_usage).max(0),
            day_remaining: (tier.requests_per_day() - day_usage).max(0),
        })
    }
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct UsageStats {
    pub minute_usage: i64,
    pub hour_usage: i64,
    pub day_usage: i64,
    pub minute_limit: i64,
    pub hour_limit: i64,
    pub day_limit: i64,
    pub minute_remaining: i64,
    pub hour_remaining: i64,
    pub day_remaining: i64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tier_limits() {
        assert_eq!(TenantTier::Free.requests_per_minute(), 60);
        assert_eq!(TenantTier::Professional.requests_per_minute(), 1000);
        assert_eq!(TenantTier::Enterprise.requests_per_minute(), 10000);

        assert_eq!(TenantTier::Free.storage_gb(), 1);
        assert_eq!(TenantTier::Professional.storage_gb(), 100);
        assert_eq!(TenantTier::Enterprise.storage_gb(), 10000);
    }
}
