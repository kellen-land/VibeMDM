-- Mobile device commands: Lost Mode location and Activation Lock bypass codes.

-- Last location reported by DeviceLocation (only available in MDM Lost Mode).
ALTER TABLE mobile_devices
    ADD COLUMN last_latitude      DOUBLE PRECISION,
    ADD COLUMN last_longitude     DOUBLE PRECISION,
    ADD COLUMN last_location_accuracy DOUBLE PRECISION,
    ADD COLUMN last_location_at   TIMESTAMPTZ;

-- Activation Lock bypass codes from supervised devices (ActivationLockBypassCode), sealed.
CREATE TABLE mobile_device_activation_lock (
    mobile_device_id UUID PRIMARY KEY REFERENCES mobile_devices (id) ON DELETE CASCADE,
    bypass_code_encrypted TEXT NOT NULL,
    collected_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
