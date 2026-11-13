# Security policy

## Reporting a vulnerability

Please report security problems privately through GitHub:
**Security → Report a vulnerability** on this repository. Do not open a
public issue.

Include the version (shown under **More** in the menu), your operating
system, and steps to reproduce. You should get a reply within two weeks.

## Scope and design notes

Unison Cue runs `unison` on your behalf with your user rights. It does
not open network ports, send data anywhere, or update itself.

- **Passwords and passphrases** typed into the ssh dialog go straight from
  the dialog to ssh through a pipe. Unison Cue does not store or log them.
- **Settings, state and logs** are stored with owner-only permissions
  (macOS), because logs list the names of synced files.
- **External programs** (osascript, PowerShell, Notepad, cmd) are started
  by absolute path, and text is passed to them in environment variables
  rather than spliced into scripts.
- Anyone who can edit your settings file can change which program runs as
  `unison`. This is the same trust level as your shell profile.
