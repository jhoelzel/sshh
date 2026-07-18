# Windows Runtime

## Support Status

shh-h uses the Windows Pseudoconsole API, commonly called ConPTY, for local
terminal sessions. ConPTY requires Windows 10 version 1809 or newer or Windows
Server 2019 or newer. The Wails desktop UI separately requires a supported
system WebView2 runtime.

The ConPTY transport is implemented and exercised on the native `windows-2025`
GitHub Actions runner. Full WebView2 interaction validation for focus, keyboard
traversal, AltGr, IME composition, and clipboard behavior remains an open
release gate. Passing the transport suite does not claim those UI behaviors.

## Shell Selection

An explicit local-profile executable is resolved exactly and produces an error
when it is unavailable. With an empty executable, the adapter tries:

1. PowerShell Core (`pwsh.exe`).
2. Windows PowerShell from `SystemRoot`, then `powershell.exe` on `PATH`.
3. `ComSpec`, then `cmd.exe`.
4. WSL (`wsl.exe`).

Arguments and the working directory pass directly from the profile. An empty
working directory uses the current user's home directory. Profile environment
overrides merge case-insensitively over the inherited Windows environment, and
the resulting double-null-terminated Unicode block is passed to `CreateProcessW`.
The runtime owns `TERM=xterm-256color`, `COLORTERM=truecolor`, and the session ID.

## Lifecycle

The adapter creates separate synchronous input and output channels as required
by ConPTY. It creates the shell suspended, attaches the HPCON process attribute,
assigns the process to a private job configured with
`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, and resumes only after that assignment.
Descendants therefore join the same cleanup boundary before the root can spawn
them.

Ctrl+C and the graceful hangup stage send the control byte through ConPTY.
Closing a transport cancels both host pipe ends, including a synchronous write
that is already blocked. The session manager then escalates to terminating the
complete job when a shell does not exit. `Wait` reads the root exit code,
terminates any remaining descendants, and closes the pseudoconsole on a separate
goroutine while the output reader drains final frames. A bounded fallback closes
undrained output so older Windows releases cannot block teardown indefinitely;
every process, job, and pipe handle is released exactly once.

## Native Verification

Run the targeted suite from a native Windows checkout:

```powershell
go test -count=1 -timeout=2m ./internal/adapter/localpty
```

The suite launches its own test executable through the real ConPTY API and
verifies initial dimensions, environment delivery, live resize, output while
closing, teardown without an output reader, nonzero exit status, and descendant
process cleanup. Cross-compilation and browser-only tests are useful additional
checks but do not replace this native run.

## References

- [Creating a Pseudoconsole session](https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session)
- [ClosePseudoConsole](https://learn.microsoft.com/en-us/windows/console/closepseudoconsole)
- [CreateProcessW](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw)
