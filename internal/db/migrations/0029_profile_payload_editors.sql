-- Computer Configuration Profiles built with the payload editors (like mobile profiles):
-- [{type, uuid, values}]; NULL for an uploaded .mobileconfig or a Custom Settings profile.
ALTER TABLE config_profiles ADD COLUMN payloads JSONB;
