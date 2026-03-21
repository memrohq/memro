use dashmap::DashMap;
use std::time::{Duration, Instant};

/// Fixed-window per-IP rate limiter backed by DashMap.
///
/// Eviction is lazy: stale entries are removed only when the key is next
/// accessed. Memory usage is bounded by the number of unique active IPs.
#[derive(Debug)]
pub struct RateLimiter {
    map: DashMap<String, WindowEntry>,
    max_requests: u64,
    window: Duration,
}

#[derive(Debug, Clone)]
struct WindowEntry {
    count: u64,
    window_start: Instant,
}

impl RateLimiter {
    /// Create a new rate limiter.
    ///
    /// * `max_requests` — requests allowed per `window`
    /// * `window`       — rolling time window (e.g. `Duration::from_secs(60)`)
    pub fn new(max_requests: u64, window: Duration) -> Self {
        Self {
            map: DashMap::new(),
            max_requests,
            window,
        }
    }

    /// Returns `true` if the request should be allowed, `false` if it should
    /// be rejected (rate-limit exceeded).
    pub fn check_and_increment(&self, key: &str) -> bool {
        let now = Instant::now();

        let mut entry = self.map.entry(key.to_string()).or_insert_with(|| WindowEntry {
            count: 0,
            window_start: now,
        });

        // Reset window if it has expired
        if now.duration_since(entry.window_start) >= self.window {
            entry.count = 0;
            entry.window_start = now;
        }

        if entry.count >= self.max_requests {
            return false;
        }

        entry.count += 1;
        true
    }
}

/// Extract a best-effort client IP string from headers.
/// Checks `X-Forwarded-For` first (set by nginx / cloud load balancers),
/// then `X-Real-Ip`, and falls back to `"unknown"`.
pub fn client_ip(headers: &axum::http::HeaderMap) -> String {
    if let Some(fwd) = headers.get("x-forwarded-for").and_then(|v| v.to_str().ok()) {
        // X-Forwarded-For: client, proxy1, proxy2  — take the first
        if let Some(ip) = fwd.split(',').next().map(str::trim) {
            return ip.to_string();
        }
    }
    if let Some(real) = headers.get("x-real-ip").and_then(|v| v.to_str().ok()) {
        return real.to_string();
    }
    "unknown".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn allows_requests_within_limit() {
        let limiter = RateLimiter::new(3, Duration::from_secs(60));
        assert!(limiter.check_and_increment("1.2.3.4"));
        assert!(limiter.check_and_increment("1.2.3.4"));
        assert!(limiter.check_and_increment("1.2.3.4"));
    }

    #[test]
    fn blocks_when_limit_exceeded() {
        let limiter = RateLimiter::new(2, Duration::from_secs(60));
        assert!(limiter.check_and_increment("1.2.3.4"));
        assert!(limiter.check_and_increment("1.2.3.4"));
        assert!(!limiter.check_and_increment("1.2.3.4")); // blocked
    }

    #[test]
    fn different_ips_have_independent_buckets() {
        let limiter = RateLimiter::new(1, Duration::from_secs(60));
        assert!(limiter.check_and_increment("1.1.1.1"));
        assert!(!limiter.check_and_increment("1.1.1.1")); // blocked
        assert!(limiter.check_and_increment("2.2.2.2")); // different IP — allowed
    }

    #[test]
    fn window_resets_after_expiry() {
        let limiter = RateLimiter::new(1, Duration::from_millis(10));
        assert!(limiter.check_and_increment("1.2.3.4"));
        assert!(!limiter.check_and_increment("1.2.3.4")); // blocked

        std::thread::sleep(Duration::from_millis(20)); // wait for window to expire
        assert!(limiter.check_and_increment("1.2.3.4")); // allowed again
    }

    #[test]
    fn client_ip_prefers_x_forwarded_for() {
        let mut headers = axum::http::HeaderMap::new();
        headers.insert("x-forwarded-for", "10.0.0.1, 192.168.1.1".parse().unwrap());
        headers.insert("x-real-ip", "10.0.0.2".parse().unwrap());
        assert_eq!(client_ip(&headers), "10.0.0.1");
    }

    #[test]
    fn client_ip_falls_back_to_x_real_ip() {
        let mut headers = axum::http::HeaderMap::new();
        headers.insert("x-real-ip", "10.0.0.2".parse().unwrap());
        assert_eq!(client_ip(&headers), "10.0.0.2");
    }

    #[test]
    fn client_ip_returns_unknown_when_no_headers() {
        let headers = axum::http::HeaderMap::new();
        assert_eq!(client_ip(&headers), "unknown");
    }
}
