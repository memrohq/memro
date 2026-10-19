package api

import (
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/memrohq/memro/server/internal/auth"
	"github.com/memrohq/memro/server/internal/domain"
)

const (
	otpTTL         = 15 * time.Minute
	sessionTTL     = 7 * 24 * time.Hour
	maxOTPAttempts = 5
	// With 5 attempts per code this allows 25 guesses an hour per email.
	otpCodesPerHour = 5
)

type authMessage struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// POST /auth/login — email a one-time code.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := mail.ParseAddress(email); err != nil || strings.ContainsAny(email, " <>") {
		httpError(w, http.StatusBadRequest, "A valid email is required")
		return
	}
	devLog := !s.email.Configured() && s.cfg.DevLogOTP
	if !s.email.Configured() && !devLog {
		httpError(w, http.StatusServiceUnavailable, "Email login is not configured on this server")
		return
	}
	code, err := auth.NewOTP()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to generate code")
		return
	}
	if !s.otpLimiter.Allow(email) {
		httpError(w, http.StatusTooManyRequests, "Too many sign-in codes requested. Try again later.")
		return
	}
	ctx, tenant := r.Context(), domain.DefaultTenantID
	if err := s.store.EnsureUser(ctx, email, tenant); err != nil {
		slog.Error("ensure user", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if err := s.store.SetUserOTP(ctx, email, tenant, auth.HashOTP(email, code), time.Now().Add(otpTTL)); err != nil {
		slog.Error("set otp", "err", err)
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if devLog {
		slog.Warn("DEV_LOG_OTP: sign-in code (email is not configured)", "email", email, "code", code)
		writeJSON(w, http.StatusOK, authMessage{true, "OTP sent to email"})
		return
	}
	if err := s.email.SendOTP(email, code); err != nil {
		slog.Error("send otp email", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to send login email. Please check server logs.")
		return
	}
	writeJSON(w, http.StatusOK, authMessage{true, "OTP sent to email"})
}

// POST /auth/verify — exchange a code for a session, creating the user's
// agent identity on first login.
func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	email, code := strings.ToLower(strings.TrimSpace(req.Email)), strings.TrimSpace(req.Code)
	ctx := r.Context()
	user, err := s.store.GetUserByEmail(ctx, email, domain.DefaultTenantID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	// Same response for unknown users and wrong codes, to avoid revealing
	// which emails have accounts.
	if user == nil || user.OTPHash == nil || user.OTPExpiresAt == nil {
		httpError(w, http.StatusUnauthorized, "Invalid OTP")
		return
	}
	if time.Now().After(*user.OTPExpiresAt) {
		httpError(w, http.StatusUnauthorized, "OTP expired")
		return
	}
	// The attempt is counted before the code is compared, atomically, so
	// parallel guesses share the same small budget.
	allowed, err := s.store.ReserveOTPAttempt(ctx, user.ID, maxOTPAttempts)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !allowed {
		httpError(w, http.StatusTooManyRequests, "Too many attempts. Request a new code.")
		return
	}
	hash := auth.HashOTP(email, code)
	if !auth.EqualHash(*user.OTPHash, hash) {
		httpError(w, http.StatusUnauthorized, "Invalid OTP")
		return
	}
	if ok, err := s.store.ConsumeUserOTP(ctx, user.ID, hash); err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	} else if !ok {
		httpError(w, http.StatusUnauthorized, "Invalid OTP")
		return
	}

	token, hash, err := auth.NewSessionToken()
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	if err := s.store.CreateSession(ctx, user.ID, user.TenantID, hash, time.Now().Add(sessionTTL)); err != nil {
		slog.Error("create session", "err", err)
		httpError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	resp := map[string]any{"token": token, "user_id": user.ID.String(), "agent_id": nil}
	existing, err := s.store.GetIdentityByUser(ctx, user.ID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing != nil {
		resp["agent_id"] = existing.ID
	} else {
		// Dashboard accounts act through their session and API keys, so the
		// agent's private key is not needed by anyone: it is discarded rather
		// than handed to the browser. SDK users who want ed25519 signing create
		// their own identity locally (POST /identity with a public key).
		pub, _, err := auth.GenerateKeyPair()
		if err != nil {
			httpError(w, http.StatusInternalServerError, "Failed to create agent identity")
			return
		}
		if err := s.store.CreateIdentity(ctx, pub, &user.ID, user.TenantID); err != nil {
			slog.Error("create identity", "err", err)
			httpError(w, http.StatusInternalServerError, "Failed to create agent identity")
			return
		}
		resp["agent_id"] = pub
	}
	writeJSON(w, http.StatusOK, resp)
}

// DELETE /auth/logout
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		_ = s.store.DeleteSessionByTokenHash(r.Context(), auth.HashToken(strings.TrimPrefix(h, "Bearer ")))
	}
	writeJSON(w, http.StatusOK, authMessage{true, "Logged out successfully"})
}
