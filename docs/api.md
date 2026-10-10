# REST API

Base path `/api/v1`. Authenticate with a session cookie (console) or an API token created under
**Settings → API Tokens**:

```bash
curl -H "Authorization: Bearer vmdm_…" https://mdm.example.com/api/v1/computers?q=macbook
```

Lists return `{"items": [...], "total": n}` and accept `limit`/`offset`. Errors return `{"error": "..."}`.

| Resource | Endpoints |
| --- | --- |
| Computers | `GET /computers`, `GET/PATCH/DELETE /computers/{id}`, `GET /computers/{id}/{commands,policy-logs,tasks,inventory}`, `POST /computers/{id}/commands` (MDM), `POST /computers/{id}/tasks` (agent), `POST /computers/{id}/update-inventory`, `POST /computers/{id}/push` |
| Groups | `GET/POST /groups`, `GET/PUT/DELETE /groups/{id}`, `GET/PUT /groups/{id}/members`, `GET /groups/criteria-fields`, `POST /groups/preview` |
| Scripts | `GET/POST /scripts`, `GET/PUT/DELETE /scripts/{id}`, `POST /scripts/{id}/run` |
| Policies | `GET/POST /policies`, `GET/PUT/DELETE /policies/{id}`, `GET /policies/{id}/logs`, `POST /policies/{id}/{flush,run}`, `GET /policy-logs` |
| Profiles | `GET/POST /profiles`, `GET/PUT/DELETE /profiles/{id}`, `GET /profiles/{id}/deployments`, `POST /profiles/{id}/{deploy,remove}` |
| Packages | `GET /packages`, `POST /packages` (multipart `file`), `GET/PUT/DELETE /packages/{id}`, `GET /packages/{id}/download` |
| Extension attributes | `GET/POST /extension-attributes`, `GET/PUT/DELETE /extension-attributes/{id}` |
| Activity | `GET /commands`, `GET/DELETE /commands/{uuid}`, `GET /tasks`, `POST /tasks/{id}/cancel` |
| Enrollment | `GET /enrollment/status`, `GET /enrollment/profile`, `GET/POST /enrollment/invitations`, `PATCH/DELETE /enrollment/invitations/{id}` |
| Settings | `GET/PUT /settings`, `GET/PUT /settings/apns`, `POST /settings/apns/{request,decrypt}`, `PUT /settings/apns/certificate`, `GET /settings/scep`, `POST /settings/scep/regenerate` |
| Remote shell | `GET /computers/{id}/shell` (WebSocket, admin, gated by `remote_shell_enabled`), `GET /shell-sessions`, `GET /shell-sessions/{id}` (transcript) |
| Admin | `GET/POST /users`, `PUT/DELETE /users/{id}`, `POST /users/{id}/password`, `GET/POST/DELETE /api-tokens`, `GET /audit` |

Command types for `POST /computers/{id}/commands`: `DeviceInformation`, `SecurityInfo`, `ProfileList`,
`InstalledApplicationList`, `CertificateList`, `DeviceLock` (`pin`, `message`), `EraseDevice` (`pin`),
`RestartDevice`, `ShutDownDevice`, `EnableRemoteDesktop`, `DisableRemoteDesktop`, `InstallProfile` (`profile_id`),
`RemoveProfile` (`profile_id` or `identifier`), `RemoveMDMProfile`, `InstallEnterpriseApplication` (`manifest_url`),
`AvailableOSUpdates`, `ScheduleOSUpdate`, `SetRecoveryLock`, `DeviceConfigured`, `Raw` (`plist`).

Task types for `POST /computers/{id}/tasks`: `run_script` (`script_id`, `parameters[8]`), `run_command` (`command`),
`run_policy` (`policy_id`), `inventory`, `custom_trigger` (`trigger`), `notify`, `restart`, `shutdown`, `logout_user`.
