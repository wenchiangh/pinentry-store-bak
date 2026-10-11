# pinentry-store

A self-contained Assuan pinentry. Another program asks for a passphrase. This program looks it up in the operating-system credential store and either returns it or asks the user to use, update, or save it.

One small native binary per operating system. The `gpg-agent` setting is a single `pinentry-program` line. macOS comes first. Windows and Linux are included when the same design can be built there.

## Product

The program is the connector to the operating-system credential store. It does not add Touch ID, Windows Hello, fingerprint checks, a master password, sync, or encryption of its own. An operating-system prompt, such as unlocking the keychain, stays separate from this program's dialog.

It does not implement `gpg-agent`'s `OPTION allow-external-password-cache` or the `PASSWORD_FROM_CACHE` status. The agent cache TTL, which can skip pinentry entirely, is outside this program.

Saved entries can be listed, deleted, or set to prompt again by the same binary. `GETINFO flavor` reports `store`.

## Protocol

The credential key is the whole opaque `SETKEYINFO` string and nothing else. `gpg-agent` sends these forms:

- `n/` plus a keygrip for the normal cache. The keygrip comes from the public key, so it stays stable when the passphrase changes and differs per subkey.
- `s/` plus a keygrip for the SSH cache. The same grip stays separate from `n/`.
- `u/` plus a client cache id. The id is opaque and may change. The protocol has no more stable field.

Do not match on description, email, short key id, owner, or tty, and do not add a separate type column. A missing `SETKEYINFO`, or `SETKEYINFO --clear`, means no credential read or write. Card PINs (`CACHE_MODE_PIN`), nonces, and ignore mode arrive as `--clear` and are not stored.

`SETDESC`, the prompt, the title, and the error are display text. `CONFIRM` and `MESSAGE` do not create entries. They use the same text interface with no password field. `CLEARPASSPHRASE` deletes the credential and then removes the saved entry for that key. If the credential cannot be deleted, the saved entry stays. TOTP and other one-time codes have no protocol type and get no special case.

`SETREPEAT` blocks auto-confirm. It is a new passphrase for this `SETKEYINFO`, so the form has two fields and Cancel. Save and don't-ask-next-time appear only when this ask has a stable `SETKEYINFO`. On a form that asks for a new passphrase, don't-ask-next-time is shown but cannot be changed until save is checked. While it cannot be changed, the existing flag stays as it is. It does not offer Use even when a secret is already stored. When `SETERROR` is also set, `SETREPEAT` still decides the form, and `SETERROR` is the error text. If the two entries differ, stay on that form, show `SETREPEATERROR`, and do not return a passphrase or change the store. Cancel from the form returns nothing.

When the entries match and the user accepts, apply the first matching outcome:

- `SETKEYINFO` missing or `--clear`: do not read or write the store. Return the typed passphrase.
- The user saves and the store accepts it: the new secret replaces the stored one. The saved entry is recorded, and its don't-ask mark matches the checkbox. Return the typed passphrase.
- The user does not save: leave the stored secret and the saved entry as they are. Return the typed passphrase.
- The user saves and the store rejects it: show that it was not saved and wait for acknowledgement. Leave the stored secret in place. A checked don't-ask box does not turn the mark on. If that box is cleared, clear the mark and keep the saved entry. Return the typed passphrase.

Cancel from the form before these outcomes leaves the store unchanged.

Skip the prompt only when a stored secret exists, that key is marked no-prompt, and the ask has neither `SETREPEAT` nor `SETERROR`. `SETERROR` without `SETREPEAT` forces a prompt with one password field and Cancel. Save and don't-ask-next-time appear only when this ask has a stable `SETKEYINFO`. That prompt does not offer Use and does not return the stored secret. The stored secret stays until a replacement is saved successfully. After that save, the saved entry is recorded and its don't-ask mark matches the checkbox. Leaving the box checked, without a successful save, does not turn the mark on and does not add an entry. A mark that was already set stays set when the box is left checked, so a later ask without `SETERROR` can return that same secret with no prompt. Clearing the checkbox clears the mark and leaves an existing saved entry in place. It does not create an entry that was not already there.

The caller's title, description, prompt, repeat label, and error stay as sent. Above the fields, a passphrase page states what this operation does: enter a passphrase, use or replace a saved passphrase, enter it twice, or enter it again. A confirmation or a message shows the caller's description and the buttons, and does not restate them. On a prompt for an existing secret, without `SETREPEAT` and without `SETERROR`, the buttons are OK and Cancel. An empty field uses the saved passphrase. A typed field uses the typed passphrase. There is no checkbox for that choice. Save cannot be changed while the field is empty. While it cannot be changed, it stays off. Don't-ask-next-time can be changed without checking save when the field is empty. While the field has text and save is unchecked, a mark that is off cannot be turned on, and an existing mark can still be cleared. Accepting the stored passphrase may set the don't-ask mark without writing the secret again, and records the saved entry if it is missing. Clearing that checkbox clears the mark and keeps the entry. When the typed passphrase is the one returned, a successful save replaces the stored secret, records the saved entry, and sets the don't-ask mark from its checkbox. Without a successful save, the stored secret stays, a checked don't-ask box does not turn the mark on, and a cleared box clears the mark without creating an entry. With no stored secret, the entry is written only after the new secret is saved successfully. When the mark is already set, its checkbox starts checked. A failed save on a prompt other than `SETREPEAT` does not turn the mark on and does not remove the stored secret. If the box is cleared, that failure still clears an existing mark. Show that it was not saved and wait for acknowledgement, then return the typed passphrase and close the window. Space toggles a checkbox. Enter confirms the focused button, or confirms the page when focus is on a field or checkbox.

If a required store change does not happen, show that failure, wait for acknowledgement, and return the typed passphrase anyway. Do not leave the entry form open for another attempt. A store change already made is not undone. A later ask follows the no-prompt rule on whatever secret and flag remain.

The prompt request carries the description, whether a secret exists, whether `SETREPEAT` is set, and whether `SETERROR` is set. It does not carry the secret. Use keeps the passphrase in the Assuan parent. Cancel of an operating-system prompt falls back to manual entry. A store miss, a missing credential service, and a user refusal are the same result: prompt, and do not persist unless the user saves and the store accepts it. Deleting an entry makes the next request miss. An ask with no `SETKEYINFO`, or with `SETKEYINFO --clear`, reads and writes neither the credential store nor the saved-entry catalog.

Speak the Assuan greeting and percent-escaping. Implement `GETPIN`, `CONFIRM`, `MESSAGE`, `BYE`, and `RESET`. Answer `GETINFO flavor`, `version`, `ttyinfo`, and `pid`. `flavor` is `store`. `pid` lets the client cancel the open window. Unknown `OPTION` values, including `no-grab`, `allow-external-password-cache`, and `putenv`, return `OK` and are ignored. Ignoring `OPTION putenv` does not discard variables already in this process. Honor `SETTIMEOUT`. Client cancel, or a closed Assuan pipe, closes the window and returns cancel.

## Storage

The store interface is `get`, `put`, and `delete`, keyed by the full `SETKEYINFO`. Protection at rest belongs to the store.

- macOS uses the login keychain through Security. The first access can show the keychain access prompt. After that, reads are silent. Do not use a Data Protection keychain item: an unsigned command-line program lacks the entitlement and fails with `errSecMissingEntitlement` (-34018). Do not mark the item synchronizable.
- Windows uses Credential Manager through `CredWrite` and `CredRead`, type generic. The secret is limited to 2560 bytes. A longer passphrase is not saved. `CredRead` returns the plaintext for the logged-in user and does not ask for Windows Hello.
- Linux uses Secret Service when a daemon is present. Do not use the session-only kernel keyring. Without a daemon, do not persist. The collection is usually unlocked once per graphical login. After that, reads are silent.

`store=auto` is the default. Use the operating-system store, and if it is missing do not write the passphrase the user just typed. Do not add an encrypted-file layer or a master password. `store=file` is an explicit opt-in plaintext store at `$XDG_DATA_HOME/pinentry-store/secrets`, or `~/.local/share/pinentry-store/secrets`, mode `0600`. It is separate from the saved-entry catalog.

The saved-entry catalog is `$XDG_STATE_HOME/pinentry-store/entries.json`, or `~/.local/state/pinentry-store/entries.json`. It is a JSON array in insertion order. Each object has `id`, the full `SETKEYINFO`, and `no_prompt`, whether the next ask is silent. The file holds no secrets and no descriptions. The directory is mode `0700` and the file is mode `0600`. The same paths are used on macOS. A missing file is an empty catalog. A line means this program saved a secret for that id. `no_prompt` is the don't-ask mark. The line's presence is not that mark. A read error leaves the file unchanged and is treated as no entry, so the ask prompts.

Credential items use the service name `pinentry-store` and the full `SETKEYINFO` as the account. An over-long id is not saved, and the typed passphrase is still returned for that request.

`list` reads only this file. It does not read the credential store. On the next `GETPIN` for an id, a stored secret with no line gains a line that still prompts, and a line is removed only when the secret is missing. A read that fails for another reason leaves the line in place and still prompts, without returning a secret. A failure while writing that repair is ignored and does not block the prompt.

Configuration is `$XDG_CONFIG_HOME/pinentry-store/config`, or `~/.config/pinentry-store/config`. It holds `ui=auto|gui|tui` and `store=auto|file`.

With no subcommand the binary speaks Assuan. `pinentry-store list` prints each saved entry as the id, a tab, and `prompt` or `no-prompt`. `pinentry-store list no-prompt` prints only the entries whose mark is set, in the same columns. `pinentry-store delete <SETKEYINFO>` deletes the credential and then removes the line. A missing credential is success. If the credential cannot be deleted, the line stays and the command exits 1. If the line cannot be removed after the credential is deleted, the command exits 1. `pinentry-store prompt <SETKEYINFO>` clears the don't-ask mark and leaves the secret and the line. A missing line is success and is not created.

A process in the same logged-in session can read an unlocked store. That limit belongs to the operating system.

## Interface

One text renderer draws every prompt. It writes to a terminal supplied by the host, because standard input and output are the Assuan pipes. It uses the alternate screen and does not echo the passphrase. Do not use a text-interface framework. The passphrase is never placed on a command line.

A caller terminal is a concrete `OPTION ttyname`, such as `/dev/pts/N`, that opens as a live terminal. `/dev/tty` does not count, because this process has no controlling terminal. On Windows, `GPG_TTY` is often unset. An already attached console is the caller terminal, and that check happens before `AllocConsole`. `isatty` on the Assuan pipes is never the test. On Linux, this call is in a graphical session when any of these is non-empty: the `--display` argument, `DISPLAY` in this process, `WAYLAND_DISPLAY` in this process, or `OPTION display`. Current `gpg-agent` does not send `OPTION display`. Unless `keep-display` is set, it passes the client session's `DISPLAY` as `--display` and in this process. When the client session has `WAYLAND_DISPLAY`, `gpg-agent` puts that value in this process at start. On macOS a graphical session is an Aqua session. On Windows it is an interactive desktop.

`auto` classifies this call. It does not open a window merely because a graphical host exists.

- When a caller terminal exists, draw there and do not open a second window.
- Open the system console only when no caller terminal exists and this call is in an interactive graphical session.
- If neither is available, fail the request.

`PINENTRY_USER_DATA` containing `USE_CURSES=1` applies to this call, including when the config says `ui=gui`. That value, and `ui=tui`, use the caller terminal only and do not open a window. If no caller terminal exists, the request fails. `ui=gui` uses the system console whenever this call is in an interactive graphical session, including when a caller terminal also exists. A remote or background session never opens a window, even when `ui=gui`: macOS or Linux with `SSH_CONNECTION`, Windows off an interactive desktop, macOS outside an Aqua session, or Linux when this call is not in a graphical session. Those sessions use the caller terminal when it exists, and otherwise fail.

The console host only supplies a terminal. A later native dialog can replace that host behind the same prompt interface. The request stays description, whether a secret exists, whether `SETREPEAT` is set, and whether `SETERROR` is set. It does not carry the secret. The result stays use, update, or new input, plus the save and no-prompt flags, or cancel. The Assuan parent applies the Protocol store rules to that result. A returned checkbox does not keep a secret or flag that those rules remove.

- Windows allocates a console only when the process has none, then opens `CONIN$` and `CONOUT$` in-process. Leave the Go standard handles on the Assuan pipes. Do not `freopen` them. The new console may stay in the background.
- macOS opens Terminal.app through Launch Services (`open`), not AppleScript, so it does not ask for Automation permission. The child is the same binary and returns the result on a socket in a directory of mode `0700`. The window may remain after exit. Clear the screen and do not echo the passphrase.
- Linux tries `xdg-terminal-exec`, then `x-terminal-emulator`, GNOME Console, GNOME Terminal, Konsole, xfce4-terminal, kitty, alacritty, foot, and xterm. The parent waits on the result socket because some emulators exit before the window closes. Start an emulator only when this call is in a graphical session. If no emulator exists, the request fails. Do not link GTK or Qt.

Without `SETREPEAT` and without `SETERROR`, an existing secret offers a password field, save, don't-ask-next-time, OK, and Cancel. An empty field uses the saved passphrase. A typed field uses the typed passphrase. A missing secret offers one password field, save, don't-ask-next-time, and Cancel. `SETERROR` without `SETREPEAT` offers one password field, save, don't-ask-next-time, and Cancel, and does not return the stored secret. `SETREPEAT` offers two password fields, save, don't-ask-next-time, and Cancel. Leaving save unchecked leaves a stored passphrase in place.

## Build

Go, with `CGO_ENABLED=0`, one binary per operating system. The startup code selects the store and the prompt host. There is no plugin loader. The Assuan flow is tested with an in-memory store and a scripted prompt.

Passphrases live in garbage-collected memory and cannot be wiped reliably. The binary is expected to be a few to about ten megabytes.

The current window is the system console. A later native dialog may call system frameworks, including AppKit through purego, behind the same prompt interface. Do not take a C or C++ UI toolkit, a bundled desktop stack, or a large UI framework for this one form.
