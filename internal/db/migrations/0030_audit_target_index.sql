-- Object history (Jamf-style History button) reads the audit log by target.
CREATE INDEX IF NOT EXISTS audit_log_target_idx ON audit_log (target_id, at DESC);
