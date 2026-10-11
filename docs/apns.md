# Apple Push Notification certificate (APNs)

Apple devices only connect to an MDM server when they receive a push notification. Every MDM server
needs an **MDM push certificate** issued by Apple. Apple only issues them to registered MDM vendors, so
self-hosters use a vendor that signs requests on their behalf. VibeMDM supports two paths.

## Path A — mdmcert.download (recommended for self-hosters)

[mdmcert.download](https://mdmcert.download) is a free community service run by the MicroMDM maintainers.
VibeMDM automates the exchange from **Settings → Enrollment → Push certificate**:

1. **Generate request** – enter the e-mail address registered at mdmcert.download and tick *Submit*.
   VibeMDM creates a private key and CSR and sends it to mdmcert.download.
2. You receive an e-mail with an encrypted `.p7` file. Open it in a text editor, paste the contents into
   **Decrypt signed request**, and download the resulting `.plist`.
3. Sign in to [identity.apple.com/pushcert](https://identity.apple.com/pushcert/) with any Apple Account,
   choose *Create a Certificate*, upload the `.plist`, and download the resulting `MDM_ Certificate.pem`.
4. Back in VibeMDM, **Upload Apple's certificate**. VibeMDM pairs it with the private key from step 1.

The topic (`com.apple.mgmt.External.…`) is now shown; enrollment profiles embed it automatically.

## Path B — existing certificate and key

If you already have a push certificate (for example exported from another MDM), paste the PEM certificate
and the **unencrypted** PEM private key into *Upload existing certificate + key*.

## Renewal

Push certificates expire after one year. Renew through the same Apple Account **before** expiry (identity.apple.com → *Renew*),
keeping the same topic, and upload the new certificate. If the topic changes, every Mac must re-enroll.
