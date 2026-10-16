# MDM command inspector

The inspector follows every MDM command from the moment it is queued until the device answers, and explains each
step in plain language. It doubles as a study aid for how the Apple MDM protocol actually moves.

## The life of a command

| Step | Event | What happened |
| --- | --- | --- |
| 1 | `queued` | The server stored the command in the device's queue (NanoMDM `enrollment_queue`). |
| 2 | `push_sent` / `push_failed` | The server sent an APNs push containing only the device's *PushMagic*: "come check in". APNs never carries the command itself. |
| 3 | `delivered` | The device connected to `/mdm` (reporting `Idle` or a previous result) and the server answered with the next queued command plist. |
| 4 | `acknowledged` / `error` / `not_now` | The device reported the result in its next request. `NotNow` means "busy or locked, try again later" and the command stays queued. |
| — | `cancelled` | An administrator deactivated the command before it was delivered. |

A device keeps connecting until its queue is empty: each response carries the next command, and an empty response
ends the session. That is why several `delivered`/`acknowledged` pairs often appear within a second.

## Using it

- **Command detail** (`GET /api/v1/commands/{uuid}/inspect`): a one-line summary of what the command asks for, a
  summary of the result, the full timeline, and the request/response plists.
- **Per-device timeline** (`GET /api/v1/computers/{id}/timeline`): every command event for a Mac, newest first.
- **Search** (`GET /api/v1/commands/search?q=…`): searches command and result plists.

## Redaction

Sensitive values are redacted by default everywhere: lock and erase PINs, passwords, profile payloads, recovery-key
CMS blobs, bootstrap and unlock tokens, PushMagic, SCEP challenges and similar keys. Placeholders note the original
size, for example `[redacted 6 bytes]`.

Once every device a command was queued for has answered, the stored payload and the devices' responses are
rewritten with secrets redacted (profiles, passwords, tokens), so a database copy does not hold them. Admins can request raw payloads with `?reveal=1`; for delivered commands this returns the redacted copy. Every reveal is written to the audit log. Viewers always receive
redacted payloads. Search matches against the redacted text, so it cannot be used to confirm a secret value.

Timeline events are kept for 90 days.
