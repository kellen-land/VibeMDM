# 3. APNs and certificates

MDM rests on public-key infrastructure. Four certificates matter in VibeMDM:

| Certificate | Issued by | Used for | Lifetime | Where in VibeMDM |
| --- | --- | --- | --- | --- |
| **APNs MDM push certificate** | Apple (Apple Push Certificates Portal) | Authenticating the server to APNs; its topic identifies your MDM | 1 year | Settings → Push certificate |
| **SCEP CA** | VibeMDM itself (generated on first boot) | Signing device identity certificates | 10 years | Settings → SCEP CA |
| **Device identity certificate** | The SCEP CA | Authenticating each device on `/mdm` (`Mdm-Signature`) | 3 years | Issued at enrollment |
| **TLS server certificate** | A public CA (e.g. Let's Encrypt) | HTTPS for devices, browsers and agents | 90 days (Let's Encrypt) | Your reverse proxy (e.g. Nginx Proxy Manager) |

Two optional ones:

| Certificate | Purpose | Where |
| --- | --- | --- |
| **Profile-signing identity** | Signs the enrollment profile so macOS shows it as verified | Settings → Profile signing (`signing_cert_pem`, `signing_key_pem`) |
| **FileVault escrow certificate** | Macs encrypt their FileVault recovery key to it | Generated on first boot ([Chapter 11](11-filevault.md)) |

## 3.1 The APNs push certificate

### Why you need one

A Mac only contacts its MDM server when something tells it to. That something is an APNs push sent to the
device's push token with your certificate's **topic** (`com.apple.mgmt.External.<UUID>`). Without a push
certificate, VibeMDM cannot generate an enrollment profile, because the profile must contain the topic.

Apple issues MDM push certificates only through a **vendor-signed CSR**:

1. The customer (you) creates a private key and a certificate signing request (CSR).
2. An MDM vendor with Apple's "MDM CSR" signing permission signs the CSR and wraps it in a plist.
3. The customer uploads the plist to the **Apple Push Certificates Portal** (`identity.apple.com/pushcert`)
   with an Apple Account. Apple returns the push certificate.
4. The customer pairs Apple's certificate with the private key from step 1.

Self-hosters use [mdmcert.download](https://mdmcert.download), a free community service run by the MicroMDM
maintainers, as the signing vendor. VibeMDM automates its side of the exchange.

### Path A: mdmcert.download (recommended)

1. Register your e-mail address at mdmcert.download (one-time).
2. In the console, open **Settings → Push certificate** and choose **Generate request**. Enter that e-mail address and tick
   **Submit**. VibeMDM creates a private key and CSR, stores the key (sealed if the encryption key is set) and
   posts the request to mdmcert.download.
   API: `POST /api/v1/settings/apns/request` `{"email": "you@example.com", "submit": true}`.
3. mdmcert.download e-mails you an encrypted `.p7` file. Open it in a text editor, paste its contents into
   **Decrypt signed request** and download the resulting `.plist`. The file is encrypted to your request's key,
   so only your server can decrypt it.
   API: `POST /api/v1/settings/apns/decrypt` `{"encrypted": "<contents of the .p7>"}`.
4. Sign in to the Apple Push Certificates Portal with an Apple Account. Choose **Create a Certificate**, upload the
   `.plist` and download `MDM_ Certificate.pem` (the name varies).
5. Back in VibeMDM, choose **Upload Apple's certificate**. VibeMDM pairs it with the pending private key, checks
   that they match and stores both.
   API: `PUT /api/v1/settings/apns/certificate` `{"cert_pem": "-----BEGIN CERTIFICATE-----…"}`.
6. **Settings → Push certificate** now shows the topic, subject and expiry (`GET /api/v1/settings/apns`). New enrollment
   profiles embed the topic automatically.

> Use a **shared, documented Apple Account** that your organization controls (for example `mdm-push@example.com`),
> not a personal one. Renewal must be done with the same account.

### Path B: existing certificate and key

If you already have a push certificate and its private key (for example from another self-hosted MDM), paste the
PEM certificate and the **unencrypted** PEM private key under **Upload existing certificate + key**.
API: `PUT /api/v1/settings/apns` `{"cert_pem": "…", "key_pem": "…"}`. The server rejects a key that does not
match the certificate.

Moving a certificate from another MDM keeps the topic, but the devices still trust that other server's
SCEP CA and URL, so they must re-enroll in VibeMDM anyway.

### Renewal

| Rule | Detail |
| --- | --- |
| Lifetime | One year from issue |
| Renew **before** expiry | After expiry, no device can be woken until a renewed certificate with the same topic is installed |
| Same Apple Account | Sign in to the portal with the account that created it and choose **Renew** next to the certificate. Renew keeps the topic |
| Never "Create" a new one to renew | A new certificate has a new topic. Every device would have to re-enroll |
| VibeMDM side | Run Path A again (Generate request → Decrypt → upload the renewed certificate). The topic must match |

The Health check **APNs push certificate** warns 30 days before expiry and turns critical at 7 days or after
expiry ([Chapter 15](15-health.md)).

> **Study note:** the push certificate is the single most important object in an Apple MDM deployment. Exam
> questions often test three facts: it expires yearly; it must be renewed (not re-created) with the **same Apple
> Account** to keep the topic; and if the topic changes, devices stop responding until they re-enroll. The
> push itself contains only `{"mdm": "<PushMagic>"}`, never a command or any data.

### How pushes work in VibeMDM

- `POST /api/v1/computers/{id}/push` sends a blank push (the console's **Send push** button).
- Every command queued through the commander (`Enqueue`) is followed by a push to that device. The result,
  `push_sent` with the APNs ID or `push_failed` with the error, is recorded in the command inspector
  ([Chapter 16](16-mdm-commands-and-inspector.md)).
- The push service uses four workers and HTTP/2 to Apple. `VIBEMDM_APNS_URL` can point it at a test server.
- The Health check **APNs push failures** opens when, in the last hour, at least 3 pushes and at least 50 % of
  pushes failed. It turns critical if none succeeded.

## 3.2 The SCEP CA and device identities

### What VibeMDM's CA does

On first boot VibeMDM creates an RSA certificate authority valid for **10 years**. It is stored in the `scep_ca`
table with its private key sealed at rest. The CA is created only when none exists; if a stored CA cannot be read
(for example because the encryption key is wrong), the server refuses to start rather than replace it.

The `/scep` endpoint implements SCEP (`GetCACaps`, `GetCACert`, `PKIOperation`). When a Mac installs the
enrollment profile:

1. Its `com.apple.security.scep` payload tells it to generate a **2048-bit RSA key on the device** (`Keysize` 2048,
   `Key Usage` 5 = digital signature + key encipherment), with subject `O=<org>, CN=<org> Device Identity`.
2. It sends a PKCS#10 CSR containing the **challenge password**, wrapped in a SCEP PKCS#7 message, to
   `/scep?operation=PKIOperation`.
3. VibeMDM checks the challenge, signs a client-auth certificate valid for **3 years**, records it in `scep_certs`
   and returns it.
4. The `com.apple.mdm` payload references this identity (`IdentityCertificateUUID`). The device uses it to sign
   every MDM request.

**Settings → SCEP CA** (`GET /api/v1/settings/scep`) shows the CA subject, validity, the number of identities
issued and still active, and the CA certificate PEM. Admins also see the static challenge.

### SCEP challenges

| Challenge | Validity | Single use? | Where it comes from |
| --- | --- | --- | --- |
| Minted for a user profile download (`/enroll/profile`) | 2 hours | yes | Embedded in that profile |
| Minted for the agent (`--mdm`, `/agent/v1/enrollment-profile`) | 1 hour | yes | Embedded in that profile |
| Minted for an admin download (`GET /api/v1/enrollment/profile`) | 7 days | yes | Embedded in that profile |
| Minted for Apple Configurator (`/enroll/configurator/{token}`) | 24 hours | **no** (one profile prepares a batch) | Embedded in that profile |
| **Static** challenge | until regenerated | no | Settings → SCEP CA (admin only). For scripted enrollments and the simulator |

Every downloaded enrollment profile therefore carries its own fresh challenge. A leaked profile file is useless
after its challenge is used or expires. Expired challenges are pruned hourly.

> **Study note:** SCEP lets a device obtain a certificate **without the private key ever leaving the device**. The
> shared secret is the challenge password. In production, Apple and most vendors recommend dynamic (one-time)
> challenges, which VibeMDM implements. On iOS and macOS, the ACME payload (`com.apple.security.acme`) is a
> newer alternative that supports hardware-bound keys and attestation. VibeMDM uses SCEP.

### Regenerating the CA

`POST /api/v1/settings/scep/regenerate` with `{"confirm": "REGENERATE"}` (admin) replaces the CA **and**
rotates the static challenge. Every existing device identity stops verifying, so **every Mac must re-enroll**.
Use it only after a CA key compromise.

### Expiry

| Item | Health check | What to do |
| --- | --- | --- |
| SCEP CA (10 years) | Warns 90 days before expiry, critical at 14 days | Plan a re-enrollment campaign; the CA cannot be renewed in place |
| Device identity (3 years) | None | VibeMDM verifies identities against the CA, including the validity period. It has no automatic identity renewal, so a Mac enrolled for more than 3 years will fail authentication and must re-enroll. Track enrollment dates with a smart group on `mdm_enrolled_at` |

> **Uncertain area:** Apple's MDM protocol lets a server renew a device identity by sending the device a new
> profile with a fresh SCEP payload, but the exact requirements (matching identifiers, macOS versions) are not
> covered here and VibeMDM does not implement it. Check Apple's Device Management documentation before relying
> on it.

## 3.3 Profile signing

macOS shows a downloaded profile as **Verified** if it is signed by an identity that chains to a trusted root,
and as **Unsigned** otherwise. An unsigned enrollment profile works, but users are more likely to trust a
verified one.

To sign the enrollment profile:

1. Get a code- or S/MIME-capable certificate and its private key. Any identity that chains to a root macOS trusts
   gives the "Verified" label. A certificate from your own CA shows as verified only on Macs that trust that CA.
2. In **Settings → Profile signing**, paste the certificate PEM (the leaf first, then any intermediates) and the private
   key PEM.
   API: `PUT /api/v1/settings` `{"signing_cert_pem": "…", "signing_key_pem": "…"}`. To remove: `{"clear_signing": true}`.
3. Download a new enrollment profile; it is now a PKCS#7 SignedData file.

Only the enrollment profile is signed. Profiles delivered later with `InstallProfile` travel over the
authenticated MDM channel and do not need a signature.

The Health check **Profile signing certificate** warns when the stored certificate cannot be parsed or expires
within 30 days.

## 3.4 The TLS certificate

Apple devices check the server's TLS certificate the same way Safari does. The certificate lives in your reverse proxy
([§2.2.1](02-installation.md#221-nginx-proxy-manager)); a Let's Encrypt certificate works, as long as it is renewed
before it expires. If you use your own CA for TLS, the CA must be
trusted on the device **before** enrollment. With Apple Configurator you can add an anchor certificate during
**Prepare**; on a Mac, the user would have to install and trust the CA manually first, which is why a public
certificate is strongly recommended.

> **Study note (certificates and trust):**
> - A **certificate** binds a public key to a subject and is signed by an issuer. Trust comes from a chain that ends
>   at a **root** in the device's trust store. On macOS the System Roots keychain holds Apple's trusted roots.
> - Installing a root certificate through a configuration profile (`com.apple.security.root`) adds it to the trust
>   store. On iOS and iPadOS, a manually installed root must also be enabled for full trust in
>   *Settings → General → About → Certificate Trust Settings*. Roots delivered by MDM are trusted automatically.
> - **Identity** = certificate + private key (`com.apple.security.pkcs12` for an imported identity, SCEP or ACME
>   for an on-device one).
> - On macOS, inspect certificates in **Keychain Access**, or with `security find-certificate` and
>   `security find-identity`. The MDM identity is in the System keychain.
> - Know the payload types: `com.apple.security.root` (CA certificate), `com.apple.security.pkcs1` (DER/PEM
>   certificate), `com.apple.security.pkcs12` (identity), `com.apple.security.scep`, `com.apple.security.acme`.

Next: [Chapter 4 — Enrollment](04-enrollment.md).
