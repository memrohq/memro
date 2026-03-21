-- Add asset_url to memories table
ALTER TABLE memories ADD COLUMN IF NOT EXISTS asset_url TEXT;
