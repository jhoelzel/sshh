# Error Handling

`shh-h` uses one error taxonomy across use cases, adapters, the Wails bridge,
and the React client. Human-readable messages remain useful, while stable codes
let the UI distinguish validation, stale state, conflicts, unavailable services,
and unexpected failures without parsing message text.

## Codes

| Code | Meaning | Retryable by default |
| --- | --- | --- |
| `invalid_argument` | User input or a command parameter is invalid. | No |
| `not_found` | The requested saved object or live resource no longer exists. | No |
| `conflict` | Current state or an external edit prevents the operation. | No |
| `stale` | A frontend lease, session generation, or resource owner is outdated. | Yes |
| `authentication_required` | Explicit credentials or a key passphrase are required. | No |
| `permission_denied` | The operating system or remote endpoint denied access. | No |
| `resource_exhausted` | A bounded runtime resource has reached its admission limit. | No |
| `unavailable` | A required service, transport, or platform feature is unavailable. | Yes |
| `canceled` | The operation was deliberately canceled. | No |
| `deadline_exceeded` | A bounded operation timed out. | Yes |
| `internal` | No more specific safe classification is available. | No |

## Go Contract

`internal/apperror` owns the codes, typed error implementation, standard-error
classification, and frontend descriptor. Typed errors retain their original
cause for `errors.Is`, `errors.As`, tests, and local diagnostics. The descriptor
contains only the stable code, a safe user-facing message, an optional operation
name, and retry guidance; wrapped causes are never serialized to the frontend.

Use the most specific code at the layer that understands the failure:

- Domain validation becomes `invalid_argument` at its use-case boundary.
- Missing saved or live objects become `not_found`.
- External edits, duplicate names, and invalid state transitions become
  `conflict`.
- Lease ownership and generation mismatches become `stale`.
- Admission limits become `resource_exhausted`; the message names the bounded
  resource and the action required to release capacity.
- Platform adapters preserve standard cancellation, deadline, not-found, and
  permission errors so the common classifier can recognize them.
- Unexpected implementation or persistence failures remain `internal` unless a
  higher layer can safely give them a more precise meaning.

## Wails Contract

Wails' application-wide `ErrorFormatter` serializes every rejected Go call as a
JSON string with this shape:

```json
{"code":"stale","message":"Frontend lease is missing or stale.","retryable":true}
```

The JSON is intentionally returned as a string. Wails v2 constructs a JavaScript
`Error` from the formatter result, so returning an object would otherwise reduce
the rejection to `[object Object]`.

`frontend/src/lib/bridge/client.ts` wraps every backend promise and converts the
envelope into `BackendError`. Existing UI code can continue reading
`Error.message`; workflows that need recovery behavior can also inspect `code`,
`operation`, and `retryable`. Malformed or legacy rejections are normalized to an
`internal` `BackendError` instead of leaking an untyped value through the app.

## Lifecycle Errors

Expected state changes are represented by the normal session, transfer, and
tunnel state models, not by thrown errors. Errors are reserved for rejected
commands or failed operations. A visible failed state may still carry a bounded
human-readable reason, while the command that initiated it uses this taxonomy.

## Terminal Stream Errors

Terminal output is untrusted byte data rather than a command rejection. The
frontend controller validates event identity, ordering, offsets, size, base64
framing, and decoded byte count before passing a chunk to xterm. Invalid frames
produce a bounded visible application error and do not throw from the Wails
event callback or advance the expected sequence, so a corrected frame can be
retried safely.

If xterm itself rejects a chunk, the controller reports a bounded parser error
and halts that output stream. It does not acknowledge a later sequence past the
rejected bytes. Error messages contain no terminal contents.
