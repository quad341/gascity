---
title: "Local PostgreSQL bootstrap for bd PG-backed scopes (macOS launchd)"
description: "One-time setup of a private launchd-managed PostgreSQL instance on 127.0.0.1:5433 for local development of bd PG-backed scopes on macOS."
---

This document describes how to set up a private PostgreSQL instance on a
macOS developer machine for use as the storage backend of one or more PG-backed
bd scopes. The result is a PostgreSQL server running under your user account via
a launchd LaunchAgent, listening on `127.0.0.1:5433` only, starting automatically
at each login, and reachable via the slice-2 INI credentials file at
`~/.config/beads/credentials`.

This bootstrap is **opt-in** and **manual**. `gc init` and `gc destroy` do not
invoke any of the steps below and do not require this server to exist.
PG-backed scopes that target a different (cloud, container, externally-managed)
PostgreSQL endpoint do not need this doc.

This doc is the macOS launchd analogue of `engdocs/postgres-local-bootstrap.md`
(Linux systemd-user). The postgres data directory, port, and credentials file are
identical; only the service-management layer differs.

The numbered sections below are the copy-pastable bootstrap sequence. `gc doctor`
only verifies the completed setup; it does not print or run this bootstrap.

## 1. Audience and prerequisites

**Audience.** Developers running bd-backed scopes on a macOS development machine,
where PostgreSQL is the chosen storage backend for one or more scopes (per
`MetadataState.Backend`). Developers using the Dolt backend do not need this doc;
developers targeting a remote/cloud PostgreSQL do not need this doc.

**Outcome.** A private PostgreSQL ≥ 14 server, owned by your user account, listening
on `127.0.0.1:5433` only, started by launchd at each login via a LaunchAgent plist
at `~/Library/LaunchAgents/com.beads.postgres.plist`. A slice-2-compatible credentials
file at `~/.config/beads/credentials` containing the generated role password.

**What you must have on PATH.**

- `pg_ctl`, `postgres`, `psql` from PostgreSQL ≥ 14 (Homebrew: `brew install postgresql@16`).
- `openssl` for password generation (ships with macOS; verify with `openssl version`).
- `launchctl` (ships with macOS).
- No `sudo` is required.

**Platform.** This doc targets macOS 12 (Monterey) or later. Homebrew is the only
supported package manager for PostgreSQL on macOS. A system PostgreSQL (via Xcode
Command Line Tools or similar) does not ship `pg_ctl`; do not attempt this runbook
with a non-Homebrew PostgreSQL.

**Homebrew path.** If PostgreSQL was installed with `brew install postgresql@16`,
the binaries are under `$(brew --prefix postgresql@16)/bin/`. Add that directory
to your `PATH` before running this doc:

```bash
export PATH="$(brew --prefix postgresql@16)/bin:$PATH"
```

Future shell sessions pick it up if you add the export to `~/.zshrc` (or the profile
for your shell). Verify with `pg_ctl --version`.

**Boot survival semantics (important difference from Linux).** LaunchAgents start at
*login*, not at boot. If no user is logged in to this Mac, the postgres server does
not run. For a developer workstation where the user remains logged in, this is
acceptable. If you need postgres to run while the machine is unattended (CI slave,
server repurposing), a LaunchDaemon (requiring root) or a container runtime is the
right tool; see `engdocs/postgres-container-bootstrap.md` for the container alternative.

**What this doc does NOT modify.**

- Any system-installed PostgreSQL instance or data directory.
- Your shell rc files (`~/.zshrc`, `~/.bash_profile`, etc.) — except the OPTIONAL
  export you add in §8.
- `gc` configuration or any city/rig directory.

## 2. Detect existing state

Before doing anything destructive, refuse to proceed when prior state would be
clobbered. The doc names every artefact it creates so re-runs are explicit about
which artefact blocked progress.

```bash
set -euo pipefail

# 2a. Refuse if our target plist already exists.
if [ -f "$HOME/Library/LaunchAgents/com.beads.postgres.plist" ]; then
    echo "FATAL: $HOME/Library/LaunchAgents/com.beads.postgres.plist already exists." >&2
    echo "       remove it (and re-run §11 uninstallation if needed) before re-bootstrapping." >&2
    exit 1
fi

# 2b. Refuse if our target data dir already exists.
if [ -e "$HOME/.local/share/beads/postgres/data" ]; then
    echo "FATAL: $HOME/.local/share/beads/postgres/data already exists." >&2
    echo "       remove it (rm -rf) before re-bootstrapping. this destroys any data already there." >&2
    exit 1
fi

# 2c. Refuse if our target credentials file already exists.
if [ -e "$HOME/.config/beads/credentials" ]; then
    echo "FATAL: $HOME/.config/beads/credentials already exists." >&2
    echo "       remove it (and any associated server) before re-bootstrapping." >&2
    exit 1
fi

# 2d. Refuse if port 5433 is in use by anything.
if lsof -i TCP:5433 -sTCP:LISTEN -nP 2>/dev/null | grep -q .; then
    echo "FATAL: port 5433 is already in use." >&2
    echo "       another process is bound to 127.0.0.1:5433. stop it before re-running." >&2
    exit 1
fi

# 2e. WARN (do not refuse) if a PostgreSQL appears active on :5432.
if lsof -i TCP:5432 -sTCP:LISTEN -nP 2>/dev/null | grep -q .; then
    echo "WARNING: a PostgreSQL instance appears active on port 5432."
    echo "         this bootstrap installs a SEPARATE private instance on port 5433."
    echo "         the two will coexist; you will have two running PostgreSQL servers."
    echo "         press Enter to continue, or Ctrl-C to abort."
    read -r _
fi

# 2f. Confirm pg_ctl is on PATH and version >= 14.
if ! command -v pg_ctl >/dev/null 2>&1; then
    echo "FATAL: pg_ctl not found on PATH." >&2
    echo "       install PostgreSQL >= 14 via Homebrew: brew install postgresql@16" >&2
    exit 1
fi
PG_VERSION_MAJOR=$(pg_ctl --version | awk '{print $NF}' | cut -d. -f1)
if [ "${PG_VERSION_MAJOR:-0}" -lt 14 ]; then
    echo "FATAL: pg_ctl version ${PG_VERSION_MAJOR} < 14." >&2
    echo "       this bootstrap requires PostgreSQL >= 14 (scram-sha-256 defaults)." >&2
    exit 1
fi
```

Note: macOS does not ship `ss` from `iproute2`; this doc uses `lsof` for port detection
instead. All other detection logic is identical to the Linux runbook.

## 3. Initialise the data directory

Create the data directory and run `initdb`. Identical to the Linux runbook.

```bash
mkdir -p "$HOME/.local/share/beads"
chmod 700 "$HOME/.local/share/beads"

pg_ctl initdb \
    -D "$HOME/.local/share/beads/postgres/data" \
    -o "--auth-local=peer --auth-host=scram-sha-256 --encoding=UTF8 --locale=C"

chmod 700 "$HOME/.local/share/beads/postgres/data"
```

## 4. Configure the listener

Identical to the Linux runbook: append to `postgresql.conf`, leave `pg_hba.conf` as-is.

```bash
cat >> "$HOME/.local/share/beads/postgres/data/postgresql.conf" <<'EOF'

# beads-postgres bootstrap overrides (engdocs/postgres-macos-launchd-bootstrap.md):
listen_addresses = '127.0.0.1'
port = 5433
unix_socket_directories = '/tmp'
log_destination = 'stderr'
logging_collector = off
EOF
```

`log_destination = 'stderr'` routes PostgreSQL logs to the system log via launchd.
Read them with `log show --predicate 'process == "postgres"' --last 1h` or via
Console.app. For a live tail: `log stream --predicate 'process == "postgres"'`.

## 5. Set the role password

Identical to the Linux runbook. Peer auth on the Unix socket lets us set the role
password before TCP connections are possible.

```bash
PG_PASSWORD="$(openssl rand -base64 24)"
echo "Generated PG password (copy this somewhere safe in case this shell dies):"
echo "    $PG_PASSWORD"

pg_ctl -D "$HOME/.local/share/beads/postgres/data" start

psql -h /tmp -p 5433 -U "$USER" -d postgres \
    -c "ALTER ROLE \"$USER\" WITH PASSWORD '$PG_PASSWORD';"

pg_ctl -D "$HOME/.local/share/beads/postgres/data" stop
```

## 6. Install the launchd LaunchAgent

Resolve the absolute path to the `postgres` binary, then write a launchd LaunchAgent
plist. The binary path is resolved at install time (launchd does not expand `$PATH`
inside a plist). The plist uses XML format; all paths are absolute.

```bash
PG_BIN="$(command -v postgres)"
if [ -z "${PG_BIN}" ]; then
    echo "FATAL: 'postgres' binary not found on PATH." >&2
    echo "       install PostgreSQL >= 14 via Homebrew: brew install postgresql@16" >&2
    exit 1
fi

mkdir -p "$HOME/Library/LaunchAgents"
cat > "$HOME/Library/LaunchAgents/com.beads.postgres.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.beads.postgres</string>
    <key>ProgramArguments</key>
    <array>
        <string>${PG_BIN}</string>
        <string>-D</string>
        <string>${HOME}/.local/share/beads/postgres/data</string>
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/tmp/beads-postgres.log</string>
    <key>WorkingDirectory</key>
    <string>${HOME}</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>BEADS_CREDENTIALS_FILE</key>
        <string>${HOME}/.config/beads/credentials</string>
    </dict>
</dict>
</plist>
EOF

chmod 644 "$HOME/Library/LaunchAgents/com.beads.postgres.plist"
launchctl bootstrap gui/"$(id -u)" "$HOME/Library/LaunchAgents/com.beads.postgres.plist"
```

`KeepAlive=true` is the launchd equivalent of systemd's `Restart=on-failure`. launchd
restarts the process if it exits for any reason.

`RunAtLoad=true` starts the service immediately when the plist is loaded, and at each
subsequent login.

`StandardErrorPath=/tmp/beads-postgres.log` captures PostgreSQL's stderr. Read with
`tail -f /tmp/beads-postgres.log`. This file grows unbounded; add a logrotate or periodic
cleanup if needed. An alternative is `sudo log show --predicate 'process == "postgres"'`
(macOS Unified Log) which does not require a log file.

`EnvironmentVariables` injects `BEADS_CREDENTIALS_FILE` for services launched by launchd
(not for your shell — see §8 for shell setup).

`launchctl bootstrap gui/$(id -u) ...` is the macOS 10.15+ replacement for the older
`launchctl load`. The `gui/UID` domain is the per-user launchd domain. The plist is
read from disk once at this step; future logins reload it automatically from
`~/Library/LaunchAgents/`.

Verify the service started: `launchctl print gui/$(id -u)/com.beads.postgres` should
show `state = running`.

## 7. Populate the credentials file

Identical to the Linux runbook.

```bash
mkdir -p "$HOME/.config/beads"
cat > "$HOME/.config/beads/credentials" <<EOF
[127.0.0.1:5433]
password=$PG_PASSWORD
EOF
chmod 600 "$HOME/.config/beads/credentials"

unset PG_PASSWORD
```

## 8. Configure the shell environment

Add `BEADS_CREDENTIALS_FILE` to your shell startup file so interactive shells and
tools you run from the terminal pick it up. The LaunchAgent plist (§6) already
injects this variable for services launched by launchd — this step is for your
interactive shell sessions.

```bash
# Add to ~/.zshrc (zsh, the macOS default since Catalina):
echo 'export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"' >> "$HOME/.zshrc"

# If you use bash, add to ~/.bash_profile instead:
# echo 'export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"' >> "$HOME/.bash_profile"
```

The current shell does not see this until you open a new shell or run
`source ~/.zshrc`. Export it manually for the rest of this session:

```bash
export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"
```

## 9. Boot survival and login-persistence notes

LaunchAgents start at login, not at boot. This is sufficient for developer workstations
where the user account is always logged in.

**Gaps to be aware of:**

- If you log out and no other user is logged in, postgres stops.
- If Fast User Switching leaves your session in the background, launchd may or may not
  keep user agents running depending on the macOS version and configuration.
- On a shared or multi-user Mac, each user installs their own `com.beads.postgres.plist`
  — they use different ports or ensure only one user runs this bootstrap.

**No sudo required** for a LaunchAgent. This is intentional: the LaunchAgent runs postgres
under your user account, with your uid/gid, with no elevated privileges.

To check service status at any time:

```bash
launchctl print gui/$(id -u)/com.beads.postgres
```

Expected output includes `state = running`.

## 10. Verify

Run `gc doctor` in the city or rig that targets the PG-backed scope. Expected output:

```
✓ postgres-server: reachable at 127.0.0.1:5433
✓ postgres-auth:   credentials resolved from ~/.config/beads/credentials
```

If `postgres-server` reports a `✗` error:

| Symptom                                              | Likely cause                                                    | Fix                                                           |
|------------------------------------------------------|-----------------------------------------------------------------|---------------------------------------------------------------|
| `server not reachable at 127.0.0.1:5433`            | launchd agent failed to start or postgres crashed              | `cat /tmp/beads-postgres.log` for errors                      |
| `local PG not installed yet — see engdocs/postgres-macos-launchd-bootstrap.md for one-time setup` | this doc was not run | run this doc |
| `metadata missing postgres host/port; cannot probe` | scope metadata does not have `PostgresHost`/`PostgresPort`     | edit scope metadata or re-run `gc init` for the scope         |
| `auth failed`                                       | postgres-auth cannot read the credentials file                  | confirm `chmod 600 ~/.config/beads/credentials`               |

If the checks still fail after rerunning the relevant section above, use the
launchd logs and the credentials file permissions as the source of truth; there
is no separate `gc doctor` bootstrap-printing mode.

## 11. Uninstallation

Reverse the bootstrap in inverse order.

```bash
# 11a. Stop and unload the LaunchAgent.
launchctl bootout gui/"$(id -u)" "$HOME/Library/LaunchAgents/com.beads.postgres.plist" \
    || true  # tolerate already-unloaded

# 11b. Remove the plist file.
rm -f "$HOME/Library/LaunchAgents/com.beads.postgres.plist"

# 11c. Remove the data directory (DESTRUCTIVE — back up first if needed).
rm -rf "$HOME/.local/share/beads/postgres"

# 11d. Remove the credentials file.
rm -f "$HOME/.config/beads/credentials"

# 11e. Remove the log file.
rm -f /tmp/beads-postgres.log

# 11f. Remove BEADS_CREDENTIALS_FILE from ~/.zshrc (or ~/.bash_profile) if you added it in §8.
echo "remember to remove 'export BEADS_CREDENTIALS_FILE=...' from your shell rc if present."
```

After uninstallation, `gc doctor` against any remaining PG-backed scope will report
`✗ postgres-server: server not reachable at 127.0.0.1:5433` with the FixHint pointing
back at this document.
