use axum::{
    extract::State,
    http::StatusCode,
    Json,
};
use rand::Rng;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use uuid::Uuid;
use chrono::{Utc, Duration};
use crate::domain::{User, AuthSession, AgentIdentity};
use super::AppState;
use ed25519_dalek::SigningKey;
use rand::rngs::OsRng;

#[derive(Deserialize)]
pub struct LoginRequest {
    pub email: String,
}

#[derive(Serialize)]
pub struct LoginResponse {
    pub success: bool,
    pub message: String,
}

pub async fn login_handler(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<LoginRequest>,
) -> Result<Json<LoginResponse>, (StatusCode, String)> {
    let email = payload.email.trim().to_lowercase();
    if email.is_empty() {
        return Err((StatusCode::BAD_REQUEST, "Email is required".to_string()));
    }

    // Generate 6-digit OTP
    let otp: String = (0..6)
        .map(|_| rand::thread_rng().gen_range(0..10).to_string())
        .collect();

    let expires_at = Utc::now() + Duration::minutes(15);

    // For now, determining tenant from domain or header is stubbed.
    // Use default tenant ID.
    let tenant_id = uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000000").unwrap();

    // Ensure user exists
    let user = state.identity_store.get_user_by_email(&email, &tenant_id).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
        
    if user.is_none() {
        let new_user = User {
            id: Uuid::new_v4(),
            email: email.clone(),
            tenant_id,
            otp_code: None,
            otp_expires_at: None,
            created_at: Utc::now(),
        };
        state.identity_store.create_user(&new_user, &tenant_id).await
            .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
    }

    state.identity_store.set_user_otp(&email, &tenant_id, &otp, expires_at).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    // Send real email using the configured SMTP service
    state.email_service.send_otp(&email, &otp).await
        .map_err(|e| {
            tracing::error!("Failed to send OTP email: {}", e);
            (StatusCode::INTERNAL_SERVER_ERROR, "Failed to send login email. Please check server logs.".to_string())
        })?;

    tracing::info!("Real OTP email sent to {}", email);

    Ok(Json(LoginResponse {
        success: true,
        message: "OTP sent to email".to_string(),
    }))
}

#[derive(Deserialize)]
pub struct VerifyRequest {
    pub email: String,
    pub code: String,
}

#[derive(Serialize)]
pub struct VerifyResponse {
    pub token: String,
    pub user_id: String,
    pub agent_id: Option<String>,
    pub private_key: Option<String>, // Only returned once on creation
}

pub async fn verify_handler(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<VerifyRequest>,
) -> Result<Json<VerifyResponse>, (StatusCode, String)> {
    let email = payload.email.trim().to_lowercase();
    let code = payload.code.trim();

    // For now, determining tenant from domain or header is stubbed.
    let tenant_id = uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000000").unwrap();

    let user = state.identity_store.get_user_by_email(&email, &tenant_id).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?
        .ok_or((StatusCode::BAD_REQUEST, "User not found".to_string()))?;

    if user.otp_code.is_none() || user.otp_code.as_deref() != Some(code) {
        return Err((StatusCode::UNAUTHORIZED, "Invalid OTP".to_string()));
    }

    if let Some(expires_at) = user.otp_expires_at {
        if Utc::now() > expires_at {
            return Err((StatusCode::UNAUTHORIZED, "OTP expired".to_string()));
        }
    } else {
        return Err((StatusCode::UNAUTHORIZED, "No OTP found".to_string()));
    }

    // OTP is valid.
    let _ = state.identity_store.clear_user_otp(&email, &tenant_id).await;

    // Generate session token
    let session_id = Uuid::new_v4();
    let token = Uuid::new_v4().to_string(); // Simple bearer token for dashboard interactions
    let session = AuthSession {
        id: session_id,
        user_id: user.id,
        tenant_id: user.tenant_id,
        token_hash: token.clone(), 
        expires_at: Utc::now() + Duration::days(7),
        created_at: Utc::now(),
    };

    state.identity_store.create_session(&session, &tenant_id).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let existing_agent: Option<AgentIdentity> = state.identity_store.get_agent_identity_by_user_id(&user.id).await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    // Check if the user possesses an agent identity.
    let (resp_agent_id, resp_private_key) = if let Some(agent) = existing_agent {
        (Some(agent.id), None)
    } else {
        // Generate new agent identity
        let mut csprng = OsRng;
        let signing_key = SigningKey::generate(&mut csprng);
        let pub_hex = hex::encode(signing_key.verifying_key().as_bytes());
        let priv_hex = hex::encode(signing_key.to_bytes());

        let new_agent = AgentIdentity {
            id: pub_hex.clone(),
            user_id: Some(user.id),
            tenant_id: tenant_id.clone(),
            created_at: Utc::now(),
        };

        state.identity_store.store_identity(&new_agent).await
            .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

        (Some(pub_hex), Some(priv_hex))
    };

    Ok(Json(VerifyResponse {
        token,
        user_id: user.id.to_string(),
        agent_id: resp_agent_id,
        private_key: resp_private_key,
    }))
}

pub async fn logout_handler(
    State(state): State<Arc<AppState>>,
    req: axum::extract::Request,
) -> Result<Json<LoginResponse>, (StatusCode, String)> {
    let auth_header = req.headers().get("authorization")
        .and_then(|h| h.to_str().ok())
        .filter(|h| h.starts_with("Bearer "))
        .map(|h| &h[7..]);

    if let Some(token) = auth_header {
        let _ = state.identity_store.delete_session_by_token(token).await;
    }

    Ok(Json(LoginResponse {
        success: true,
        message: "Logged out successfully".to_string(),
    }))
}
