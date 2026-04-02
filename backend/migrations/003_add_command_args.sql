-- Migration: 003_add_command_args
-- Description: Add command_args column to commands table for proper argument tracking
-- Date: 2026-04-02

-- Add command_args column (stores JSON array of arguments)
ALTER TABLE commands 
ADD COLUMN IF NOT EXISTS command_args JSONB DEFAULT '[]'::jsonb;

-- Add command_timeout column for better timeout tracking
ALTER TABLE commands 
ADD COLUMN IF NOT EXISTS command_timeout INTEGER DEFAULT 300;

-- Update existing command_payload to ensure proper JSON format
-- This ensures backward compatibility with the application code
UPDATE commands 
SET command_args = '[]'::jsonb 
WHERE command_args IS NULL;

-- Add index for faster queries on command arguments
CREATE INDEX IF NOT EXISTS idx_commands_args ON commands USING GIN (command_args);

-- Add index for timeout queries
CREATE INDEX IF NOT EXISTS idx_commands_timeout ON commands(command_timeout);

-- Add constraint to ensure command_timeout is positive
ALTER TABLE commands 
ADD CONSTRAINT IF NOT EXISTS positive_timeout 
CHECK (command_timeout > 0 AND command_timeout <= 3600);

-- Comments for documentation
COMMENT ON COLUMN commands.command_args IS 'JSON array of command arguments';
COMMENT ON COLUMN commands.command_timeout IS 'Command timeout in seconds (max 3600)';
