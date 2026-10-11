-- When each installed-profile row was reported. The agent and MDM (ProfileList) report the
-- same profiles separately; only the most recent report reflects the Mac.
ALTER TABLE computer_profiles ADD COLUMN reported_at TIMESTAMPTZ NOT NULL DEFAULT now();
