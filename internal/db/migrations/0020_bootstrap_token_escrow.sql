-- SecurityInfo's BootstrapTokenAllowedForAuthentication was mistakenly stored as the escrow flag,
-- so a Mac that answered "disallowed" (normal without Automated Device Enrollment) showed as not
-- escrowed even after it sent SetBootstrapToken. Rebuild the flag from the token NanoMDM stores.
UPDATE computers c
SET bootstrap_token_escrowed = (d.bootstrap_token_b64 IS NOT NULL)
FROM devices d
WHERE d.id = c.udid;

