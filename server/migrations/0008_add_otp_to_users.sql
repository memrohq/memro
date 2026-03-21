-- Add OTP fields to the users table for passwordless login
ALTER TABLE users ADD COLUMN IF NOT EXISTS otp_code VARCHAR;
ALTER TABLE users ADD COLUMN IF NOT EXISTS otp_expires_at TIMESTAMPTZ;
