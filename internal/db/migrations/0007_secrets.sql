-- Allow APNs private keys to be stored encrypted at rest (enc:v1:...) in
-- NanoMDM's push_certs table. VibeMDM decrypts them in its push-cert store.
ALTER TABLE push_certs DROP CONSTRAINT IF EXISTS push_certs_key_pem_check;
ALTER TABLE push_certs ADD CONSTRAINT push_certs_key_pem_check
    CHECK (SUBSTRING(key_pem FROM 1 FOR 5) = '-----' OR SUBSTRING(key_pem FROM 1 FOR 7) = 'enc:v1:');
