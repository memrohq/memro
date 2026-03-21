use axum::{
    body::{Body, to_bytes},
    extract::{State, Request},
    http::StatusCode,
    middleware::Next,
    response::Response,
};
use std::sync::Arc;
use chrono::Utc;
use crate::adapters::api::AppState;
use crate::domain::AuthSession;

#[derive(Clone)]
pub struct AuthenticatedAgent {
    pub id: String,
    pub user_id: Option<uuid::Uuid>,
    pub tenant_id: uuid::Uuid,
}

pub async fn auth_middleware(
    State(state): State<Arc<AppState>>,
    req: Request,
    next: Next,
) -> Result<Response, (StatusCode, String)> {
    let (parts, body) = req.into_parts();

    // Check for Human Dashboard Session Token
    if let Some(auth_header) = parts.headers.get("authorization") {
        if let Ok(auth_str) = auth_header.to_str() {
            if auth_str.starts_with("Bearer ") {
                let token = &auth_str[7..];

                // Verify Session
                let session: AuthSession = state.identity_store.get_session_by_token(token).await
                    .map_err(|_| (StatusCode::INTERNAL_SERVER_ERROR, "Database error".to_string()))?
                    .ok_or((StatusCode::UNAUTHORIZED, "Invalid session token".to_string()))?;

                if Utc::now() > session.expires_at {
                    return Err((StatusCode::UNAUTHORIZED, "Session expired".to_string()));
                }

                // Get associated Agent Identity
                let identity = state.identity_store.get_agent_identity_by_user_id(&session.user_id).await
                    .map_err(|_| (StatusCode::INTERNAL_SERVER_ERROR, "Database error".to_string()))?
                    .ok_or((StatusCode::UNAUTHORIZED, "No agent associated with this user".to_string()))?;

                let authenticated_agent = AuthenticatedAgent {
                    id: identity.id,
                    user_id: identity.user_id,
                    tenant_id: identity.tenant_id,
                };

                let mut req = Request::from_parts(parts, body);
                req.extensions_mut().insert(authenticated_agent);

                return Ok(next.run(req).await);
            }
        }
    }

    // Fallback to Machine Auth (Ed25519)
    let agent_id = parts.headers
        .get("x-agent-id")
        .and_then(|v| v.to_str().ok())
        .ok_or((StatusCode::UNAUTHORIZED, "Missing X-Agent-Id or Authorization header".to_string()))?;

    let signature = parts.headers
        .get("x-signature")
        .and_then(|v| v.to_str().ok())
        .ok_or((StatusCode::UNAUTHORIZED, "Missing X-Signature header".to_string()))?;

    let timestamp_str = parts.headers
        .get("x-timestamp")
        .and_then(|v| v.to_str().ok())
        .ok_or((StatusCode::UNAUTHORIZED, "Missing X-Timestamp header".to_string()))?;

    let timestamp: i64 = timestamp_str
        .parse()
        .map_err(|_| (StatusCode::UNAUTHORIZED, "Invalid X-Timestamp".to_string()))?;

    let now = Utc::now().timestamp();
    if (now - timestamp).abs() > 300 {
        return Err((StatusCode::UNAUTHORIZED, "Request timestamp expired".to_string()));
    }

    // Verify identity exists in the database
    let identity = state.identity_store.get_identity(agent_id).await
        .map_err(|_| (StatusCode::INTERNAL_SERVER_ERROR, "Database error".to_string()))?
        .ok_or((StatusCode::UNAUTHORIZED, "Agent identity not registered".to_string()))?;

    // Read the body to verify the signature. 10MB limit.
    let bytes = to_bytes(body, 10_000_000).await
        .map_err(|_| (StatusCode::BAD_REQUEST, "Failed to read body".to_string()))?;

    // Robust Payload: TIMESTAMP + BODY
    let mut payload = timestamp_str.as_bytes().to_vec();
    payload.extend_from_slice(&bytes);

    if !state.crypto_service.verify_signature(agent_id, &payload, signature) {
        return Err((StatusCode::UNAUTHORIZED, "Invalid signature".to_string()));
    }

    // Wrap the validated agent into an Extension for downstream handlers
    let authenticated_agent = AuthenticatedAgent {
        id: identity.id,
        user_id: identity.user_id,
        tenant_id: identity.tenant_id,
    };

    // Reconstruct the request with the body
    let mut req = Request::from_parts(parts, Body::from(bytes));
    req.extensions_mut().insert(authenticated_agent);

    Ok(next.run(req).await)
}

pub async fn rate_limit_middleware(
    State(state): State<Arc<AppState>>,
    req: Request,
    next: Next,
) -> Result<Response, (StatusCode, String)> {
    let agent = req.extensions().get::<AuthenticatedAgent>()
        .ok_or((StatusCode::UNAUTHORIZED, "Missing authenticated agent".to_string()))?;
    
    let path = req.uri().path();
    let key = format!("{}:{}", agent.id, path);

    // Dynamic limits based on path
    let _limit = if path.contains("/mee/reason") {
        20 // 20 per minute
    } else if path.contains("/mee/evolve") {
        5  // 5 per minute
    } else if path.contains("/mee/resolve") {
        10 // 10 per minute
    } else {
        100 // Default high limit for other endpoints
    };

    // Note: The existing state.rate_limiter uses a fixed window from AppState init.
    // For the preview, we'll use a simplified check or create specialized limiters.
    if !state.rate_limiter.check_and_increment(&key) {
        return Err((StatusCode::TOO_MANY_REQUESTS, "Rate limit exceeded for this endpoint".to_string()));
    }

    Ok(next.run(req).await)
}
