# pinentry-store

pinentry-store is a pinentry program. When GnuPG or another Assuan client asks for a passphrase, this program looks that passphrase up in the operating-system credential store. It returns the saved passphrase, or it asks you to type one and optionally save it.

The binary speaks the pinentry Assuan protocol on standard input and standard output. `GETINFO flavor` reports `store`. The current version is 0.1.0.

macOS is the first target. The same program also builds for Linux and Windows. It is a single Go binary, built with `CGO_ENABLED=0`. It does not link GTK, Qt, or cgo.

## What it stores

The lookup key is the full `SETKEYINFO` string from the client, and nothing else. A description, an email address, a key id, an owner, or a terminal name is display text for that prompt. It is not used to find a saved passphrase.

A client that omits `SETKEYINFO`, or sends `SETKEYINFO --clear`, gets a normal prompt. Nothing is read or written. Card PINs arrive as `--clear` and are not stored.

The operating system holds the secret:

| System | Store |
| --- | --- |
| macOS | Login keychain, generic password. Service `pinentry-store`, account the full `SETKEYINFO`. The item is not a Data Protection item and is not synchronizable. |
| Windows | Credential Manager, generic credential. The secret is limited to 2560 bytes. |
| Linux | Secret Service, when a daemon is running. With no daemon, the passphrase is not saved. |

This program does not add Touch ID, Windows Hello, a fingerprint check, a master password, sync, or encryption of its own. An operating-system unlock prompt stays separate from this program's dialog.

A second file records which ids have a saved secret, and whether the next ask should skip the prompt. That file contains no secrets. See [Saved entries](#saved-entries).

## Install

Go 1.27.2 or later. Cgo stays off.

```sh
CGO_ENABLED=0 go build -o pinentry-store .
install -m 0755 pinentry-store ~/.local/bin/pinentry-store
```

Point `gpg-agent` at that binary. In `gpg-agent.conf`:

```
pinentry-program /absolute/path/to/pinentry-store
```

Reload the agent (`gpgconf --kill gpg-agent`) so it picks up the new program.

A local build reports version 0.1.0. Pushing a tag such as `v0.1.0` runs the release workflow, which sets that version from the tag and publishes six archives: Linux, macOS, and Windows, each for `amd64` and `arm64`. Unix archives are `.tar.gz`. Windows archives are `.zip`. The names look like `pinentry-store_0.1.0_darwin_arm64.tar.gz`. Checksums are in `checksums.txt`.

mise installs a release with its GitHub backend:

```toml
[tools]
"github:OWNER/pinentry-store" = "0.1.0"
```

Replace `OWNER` with the GitHub account that owns the repository.

`PINENTRY_USER_DATA` containing `USE_CURSES=1` forces the caller's terminal for that call, including when the config says `ui=gui`.

## Commands

With no subcommand the program speaks Assuan and stays running until the client says `BYE`.

```
pinentry-store [options] | list [no-prompt] | delete SETKEYINFO | prompt SETKEYINFO
```

| Command | Effect |
| --- | --- |
| `pinentry-store list` | Print each saved id, a tab, and `prompt` or `no-prompt`. |
| `pinentry-store list no-prompt` | Same columns, only ids marked don't-ask. |
| `pinentry-store delete SETKEYINFO` | Delete the credential, then remove the saved-entry line. A missing credential is success. If the credential cannot be deleted, the line stays and the command exits 1. |
| `pinentry-store prompt SETKEYINFO` | Clear the don't-ask mark. The secret and the line stay. A missing line is success and is not created. |

`list` reads only the saved-entry file. It does not scan the operating-system store. An id appears after the next ask that finds its secret.

These options are accepted, matching pinentry: `--display` / `-D`, `--ttyname` / `-T`, `--ttytype` / `-N`, `--timeout` / `-o`, plus the usual display and locale options. Unknown long options are warned about and ignored. A missing option value is an error.

## Configuration

The config file is `$XDG_CONFIG_HOME/pinentry-store/config`, or `~/.config/pinentry-store/config`. A missing file uses the defaults. Blank lines and lines starting with `#` are ignored.

```
ui=auto
store=auto
```

`ui` is `auto`, `tui`, or `gui`.

- `auto` draws on the caller's terminal when that terminal is real. Otherwise, in an interactive graphical session, it opens a system console. A remote or background session never opens a window.
- `tui` uses only the caller's terminal. `/dev/tty` does not count. The Assuan pipes are not treated as a terminal.
- `gui` uses the system console whenever this call is in an interactive graphical session, including when a caller terminal also exists.

On macOS the system console is Terminal.app. On Linux the program tries, in order, `xdg-terminal-exec`, `x-terminal-emulator`, GNOME Console, GNOME Terminal, Konsole, xfce4-terminal, kitty, alacritty, foot, and xterm. On Windows it uses the attached console, or allocates one when the process has none.

`store` is `auto` or `file`.

- `auto` uses the operating-system store. If that store is missing, the passphrase you just typed is not written anywhere else.
- `file` is an explicit plaintext store at `$XDG_DATA_HOME/pinentry-store/secrets`, or `~/.local/share/pinentry-store/secrets`, mode `0600`. It is off unless you set it.

The same XDG paths are used on macOS. `~/Library` is not used.

## Saved entries

`$XDG_STATE_HOME/pinentry-store/entries.json`, or `~/.local/state/pinentry-store/entries.json`, is a JSON array. The directory is mode `0700` and the file is mode `0600`. Each object is:

```json
{
  "id": "n/0123456789ABCDEF",
  "no_prompt": false
}
```

`id` is the full `SETKEYINFO`. `no_prompt` is the don't-ask mark. A line means this program has a saved secret for that id. The line's presence is not itself the don't-ask mark. The file is not a secret store.

On the next `GETPIN` for an id, a stored secret with no line gains a line that still prompts. A line whose secret is missing is removed. Any other read error leaves the line in place, and the prompt is shown.

## The prompt

The caller's title, description, prompt, repeat label, and error are shown as sent. This program adds a short explanation of the current operation.

- No saved secret: one passphrase field. With a stable `SETKEYINFO`, Save and Don't ask are shown. Don't ask cannot be changed until Save is checked.
- A saved secret, and this ask is not `SETREPEAT` or `SETERROR`: leave the field empty to use the saved passphrase, or type a passphrase to use what you type. Save stays off and cannot be checked while the field is empty. Don't ask can be changed while the field is empty. While the field has text and Save is unchecked, an existing don't-ask mark can be cleared, and a mark that is off cannot be turned on.
- `SETREPEAT`: enter the passphrase twice. The two entries must match. Leaving Save unchecked keeps the saved passphrase.
- `SETERROR` without `SETREPEAT`: enter the passphrase again. The saved passphrase is not returned for this request.
- Confirm and message dialogs show the caller's text and the buttons. A one-button dialog continues on Enter, Esc, or Ctrl-C.

Space toggles a checkbox. Enter activates the focused button, or confirms the page when focus is on a field or checkbox. Esc and Ctrl-C cancel, except on a one-button dialog and on a failure notice, where they continue.

The prompt is skipped only when a secret exists, don't-ask is set, and the ask has neither `SETREPEAT` nor `SETERROR`.

If a save fails, the dialog says so, the stored secret stays, and the typed passphrase is still returned.

## Development

```sh
CGO_ENABLED=0 go test ./...
```

Tests use an in-memory store and a scripted prompt. They do not call the login keychain or Credential Manager.

The design note is [docs/20261011-pinentry-os-credential-store.md](docs/20261011-pinentry-os-credential-store.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
