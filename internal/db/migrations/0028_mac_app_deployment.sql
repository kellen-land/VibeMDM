-- How a Mac app is deployed: installed automatically on Macs in scope, or offered in Self Service only
-- (installed when someone clicks Install; "keep up to date" then updates it where it is installed).
ALTER TABLE mac_apps ADD COLUMN deployment TEXT NOT NULL DEFAULT 'automatic' CHECK (deployment IN ('automatic', 'self_service'));
