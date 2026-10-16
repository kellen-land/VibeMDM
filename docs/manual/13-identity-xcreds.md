# 13. Identity and XCreds

**Identity → XCreds** integrates [XCreds](https://github.com/twocanoes/xcreds) by Twocanoes (BSD-3-Clause). XCreds
replaces the macOS login window with your identity provider's sign-in page, using OpenID Connect (OIDC). It
creates the local account on first sign-in and keeps the local password in sync with the cloud password. VibeMDM:

1. installs XCreds with the Installomator `xcreds` label, as a Mac apps catalog entry that keeps it updated; and
2. configures it with a managed-preferences profile (`com.vibemdm.xcreds`, domain `com.twocanoes.xcreds`) built from
   the bundled `builtin:com.twocanoes.xcreds` schema.

The app and the settings profile always share the same scope.

## 13.1 Before you start

| Need | Notes |
| --- | --- |
| An OIDC app registration at your identity provider | You need its **client ID**, the **discovery URL**, the **redirect URI**, and for Google a **client secret** |
| An XCreds licence from Twocanoes | The bundled schema says XCreds runs as a **14-day trial** without a licence. Check Twocanoes' current licensing terms; they are set by Twocanoes, not VibeMDM |
| Test Macs you can erase | A broken login-window configuration can lock users out |
| A local admin account on each test Mac | Your way back in if cloud login fails |

## 13.2 Configure XCreds

1. Open **Identity → XCreds** and choose your provider: **Microsoft Entra ID**, **Google Workspace**, **Okta** or
   **Other OpenID Connect**.
2. Enter the tenant (Entra) or Okta domain. The console fills in the discovery URL:

   | Provider | Discovery URL |
   | --- | --- |
   | Entra ID | `https://login.microsoftonline.com/<tenant-id-or-domain>/v2.0/.well-known/openid-configuration` |
   | Google | `https://accounts.google.com/.well-known/openid-configuration` |
   | Okta | `https://<your-okta-domain>/.well-known/openid-configuration` |
   | Other | Your provider's `/.well-known/openid-configuration` URL (must be `https://`) |

3. Enter the **client ID** and, for Google, the **client secret**.
4. Check the **redirect URI** (default `xcreds://auth/`). It must match the app registration exactly.
5. Optionally upload the **licence file**. It is stored base64-encoded and emitted as plist `<data>` in the profile
   (`LicenseFile`, `format: data`). After saving, the API reports `has_license: true` and never echoes the licence
   back. Saving with an empty licence field keeps the stored licence.
6. Open **Advanced** for login-window behaviour, claim mapping, admin groups and password-sync options (§13.4).
7. Set the scope (start with a pilot group), leave **Install XCreds with Installomator** ticked unless you deploy the
   app another way, turn on **Enable XCreds** and save.

API:

```json
PUT /api/v1/identity/xcreds
{
  "enabled": true,
  "install_app": true,
  "scope": { "group_ids": ["<xcreds-pilot>"] },
  "values": {
    "discoveryURL": "https://login.microsoftonline.com/contoso.onmicrosoft.com/v2.0/.well-known/openid-configuration",
    "clientID": "00000000-0000-0000-0000-000000000000",
    "redirectURI": "xcreds://auth/",
    "scopes": "profile openid offline_access",
    "shouldShowCloudLoginByDefault": true,
    "shouldDetectNetworkToDetermineLoginWindow": true,
    "shouldShowMacLoginButton": true,
    "aliasName": "upn",
    "CreateAdminIfGroupMember": ["Mac Admins"]
  }
}
```

When `enabled` is true, `discoveryURL` and `clientID` are required. `GET /api/v1/identity/xcreds` returns the
current values (licence blanked), scope, profile ID and the XCreds app's install status.

What happens on save:

- The profile `com.vibemdm.xcreds` is created or updated and synced to the scope (or removed everywhere when
  disabled). As with every profile, deploy again after group-membership changes
  ([§6.2](06-scoping.md#62-where-scope-is-used-and-when-it-is-evaluated)).
- The Mac App "XCreds" (label `xcreds`, category Identity, `keep_updated`) is created or updated with the same scope,
  enabled only if XCreds is enabled **and** `install_app` is true.
- Agents in scope are asked to check in, so the install starts within seconds.

## 13.3 Provider notes

### Microsoft Entra ID

- Register an application in Entra ID; add a public-client/native redirect URI that matches `redirectURI`.
- Grant the OIDC scopes you list (`openid`, `profile`, `offline_access`; add `email` if you map it).
- Optional **ROPG** keys (`shouldUseROPGForLoginWindowLogin`, `shouldUseROPGForMenuLogin`,
  `shouldUseROPGForPasswordChangeChecking`, `resource`, e.g. `https://graph.microsoft.com`) verify passwords with the
  OAuth resource-owner password grant. Microsoft discourages ROPG and it does not work for accounts that need MFA
  interactively; test it carefully.
- `aliasName: upn` lets users sign in later with their e-mail-style UPN.

### Google Workspace

- Create an OAuth client and copy the client ID **and client secret** (Google requires the secret). The secret is
  stored in the profile in clear text, which any local admin can read.
- Turn on `shouldSetGoogleAccessTypeToOffline` so XCreds gets a refresh token and can keep checking the password.
- `shouldSetGoogleHDParam` restricts sign-in to your hosted domain.

### Okta

- Create an OIDC native app in Okta with a redirect URI matching `redirectURI`.
- The discovery URL above uses the org authorization server. If you use a custom authorization server, use its
  discovery URL instead (check your Okta configuration).

## 13.4 Settings reference (bundled schema)

| Group | Keys |
| --- | --- |
| Provider | `discoveryURL`, `clientID`, `clientSecret`, `redirectURI`, `scopes` |
| Login window | `shouldShowCloudLoginByDefault`, `shouldPreferLocalLoginInsteadOfCloudLogin`, `shouldDetectNetworkToDetermineLoginWindow`, `shouldShowMacLoginButton`, `shouldShowLocalOnlyCheckbox`, `cloudLoginText`, `loginWindowBackgroundImageURL`, `loginWindowLogoPath`, `shouldShowConfigureWifiButton`, `shouldShowShutdownButton`, `shouldShowRestartButton`, `shouldShowSystemInfoButton`, `shouldSwitchToLoginWindowWhenLocked` |
| Claim mapping | `map_username`, `map_fullname`, `map_firstname`, `map_lastname`, `map_uid`, `aliasName` |
| Access control | `allowedUsersArray`, `allowUsersClaim`, `allowLoginIfMemberOfGroup` |
| Account creation | `CreateAdminUser`, `CreateAdminIfGroupMember`, `skipUserSetupBuddy` |
| Password sync | `PasswordOverwriteSilent`, `KeychainReset`, `verifyPassword`, `refreshRateHours`, `refreshRateMinutes`, `shouldShowRefreshBanner`, `passwordChangeURL` |
| ROPG | `shouldUseROPGForLoginWindowLogin`, `shouldUseROPGForMenuLogin`, `shouldUseROPGForPasswordChangeChecking`, `resource` |
| Google | `shouldSetGoogleAccessTypeToOffline`, `shouldSetGoogleHDParam` |
| Menu bar app | `shouldShowSignInMenuItem` |
| Secrets | `settingsOverrideScriptPath` (a root-owned script that supplies secrets such as local admin credentials, keeping them out of profiles) |
| Licence | `LicenseFile` (`format: data`) |

The schema was written from XCreds' public documentation. XCreds evolves quickly; if you need a key the schema
doesn't define, the API rejects it, so check the XCreds admin guide and consider a custom schema
([Chapter 8](08-configuration-profiles.md#83-application--custom-settings-json-schemas)).

## 13.5 Testing advice

1. **Use a pilot Mac you can erase**, with a separate local admin account that you know works.
2. Start with `shouldShowMacLoginButton: true` and `shouldDetectNetworkToDetermineLoginWindow: true`, so you can
   always reach the standard login window and offline logins still work.
3. Scope XCreds to the pilot group, confirm the app installed (Mac apps → XCreds logs) and the profile is
   `installed`, then log out.
4. Sign in with a cloud account. Check that the local account was created with the expected short name and that it
   has a **Secure Token** if FileVault is on (`sysadminctl -secureTokenStatus <user>`; VibeMDM inventory shows it per
   local account). Accounts created at the login window may need the bootstrap token to receive a Secure Token.
5. Change the cloud password and confirm the local password and login keychain follow.
6. Test FileVault unlock after a restart, and offline login.
7. Only then widen the scope.

To turn XCreds off, disable it: the profile is removed from every Mac and the app is no longer installed. Removing
the app itself from Macs (and restoring the stock login window) is up to you; check XCreds' uninstall instructions.

## 13.6 Study notes: Platform SSO, XCreds and the Kerberos SSO extension

> **Study note (comparison):**
>
> | | **Platform SSO** (Apple) | **XCreds** (third party) | **Kerberos SSO extension** (Apple) |
> | --- | --- | --- | --- |
> | What it is | An Apple framework (macOS 13+, much expanded in 14) in which an IdP's SSO extension integrates with the macOS login process | A replacement login window (authorization plugin + app) that signs in with OIDC | Built-in SSO extension (`com.apple.AppSSOKerberos.KerberosExtension`) for on-premises Active Directory |
> | Delivered by | The `com.apple.extensiblesso` payload with Platform SSO settings, plus the IdP's app (for example Microsoft Company Portal, Okta Verify) | An app (Installomator here) plus a managed-preferences profile | The `com.apple.extensiblesso` payload, type Credential, with realm and domains |
> | Identity source | A cloud IdP that supports Apple's Platform SSO protocol | Any OIDC provider | Active Directory (Kerberos) |
> | Authentication methods | Password (synced to the IdP), **Secure Enclave key** (phishing-resistant, passwordless), smart card | Cloud password at the login window, kept in sync with the local password | AD password; gets Kerberos tickets for SSO to intranet services |
> | Local account | Existing local account is registered with the IdP; newer releases add creating accounts at the login window and registration during setup | Created at first cloud login | Existing local or mobile account; no AD binding needed |
> | MDM requirements | MDM-delivered payload; some features need ADE or newer macOS versions | Any MDM (or none) | MDM-delivered payload |
> | Network | Needs the IdP | Needs the IdP (with offline local fallback) | Needs line of sight to domain controllers for ticket and password operations |
>
> Exam points: Platform SSO is Apple's strategic direction for cloud identity on the Mac and needs IdP support;
> the Kerberos SSO extension replaced the need to **bind** Macs to Active Directory; extensible SSO
> (`com.apple.extensiblesso`) is the payload family for both *redirect* (web SSO) and *credential* (Kerberos)
> extensions. Which Platform SSO features need which macOS version changes with each release, so check the *Apple
> Platform Deployment* guide.

Next: [Chapter 14 — White-glove onboarding](14-white-glove-onboarding.md).
