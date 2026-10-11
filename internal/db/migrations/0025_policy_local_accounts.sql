-- Local Accounts policy payload (Jamf Pro: Create Account, Reset Password, Delete Account,
-- Disable Account for FileVault). Passwords are sealed inside each entry ("password_sealed").
ALTER TABLE policies ADD COLUMN accounts JSONB NOT NULL DEFAULT '[]'::jsonb;
