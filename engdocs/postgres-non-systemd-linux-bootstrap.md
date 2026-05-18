---
title: "Local PostgreSQL bootstrap for bd PG-backed scopes (non-systemd Linux: OpenRC, runit, s6)"
description: "One-time setup of a private PostgreSQL instance on 127.0.0.1:5433 for local development of bd PG-backed scopes on Linux systems using OpenRC, runit, or s6 instead of systemd."
---

This document describes how to set up a private PostgreSQL instance on a Linux
developer machine that uses **OpenRC**, **runit**, or **s6** as the init system,
for use as the storage backend of one or more PG-backed bd scopes.

The postgres data directory (`~/.local/share/beads/postgres/data`), port (5433),
and credentials file (`~/.config/beads/credentials`) are identical to the systemd-user
runbook at `engdocs/postgres-local-bootstrap.md`. If you are running systemd, use
that doc instead; this doc is exclusively for non-systemd Linux.

This bootstrap is **opt-in** and **manual**. `gc init` and `gc destroy` do not
invoke any of the steps below.

`gc doctor --explain-postgres-non-systemd-linux-bootstrap` prints the same sequence
as a copy-pastable shell script (see §10).

## 1. Audience and prerequisites

**Audience.** Developers running bd-backed scopes on:

- **Gentoo Linux** with OpenRC (default init system on Gentoo).
- **Alpine Linux** with OpenRC (default init system on Alpine).
- **Void Linux** with runit (default init system on Void).
- **Alpine Linux** with s6-overlay (common in container environments; less common on
  bare developer machines).

If you are running systemd (Fedora, Debian, Ubuntu, Arch, etc.), use
`engdocs/postgres-local-bootstrap.md` instead.

**Outcome.** A private PostgreSQL ≥ 14 server, owned by your user account, listening
on `127.0.0.1:5433` only. The service management mechanism differs by init system (§6).

**What you must have on PATH.**

- `pg_ctl`, `postgres`, `psql` from PostgreSQL ≥ 14.
- `openssl` for password generation.
- `ss` (from `iproute2`) for port detection. On Alpine with BusyBox `netstat`, use
  `netstat -tln` in place of `ss -tln`.
- Init-system tools: `rc-service`, `rc-update` (OpenRC); `sv`, `ln` (runit);
  `s6-svc` (s6). See §6 for init-system-specific prerequisites.

**Platform.** Linux with OpenRC, runit, or s6. Not systemd. `/run/systemd/system`
MUST NOT exist on a target machine for this runbook to apply.

**Cross-version compatibility.** Same as the systemd-user runbook: PostgreSQL 14, 15,
16. PostgreSQL ≥ 14 defaults to `scram-sha-256` for TCP auth.

**What this doc does NOT modify.**

- System-wide PostgreSQL data directories.
- System package manager state (beyond confirming packages are installed).
- Your shell rc files (except the OPTIONAL export in §8).

## 2. Detect existing state

Common to all init systems. Detection strategy differs for port (uses `ss` from
iproute2; BusyBox alternative noted).

```bash
set -euo pipefail

# 2a. Refuse if our target data dir already exists.
if [ -e "$HOME/.local/share/beads/postgres/data" ]; then
    echo "FATAL: $HOME/.local/share/beads/postgres/data already exists." >&2
    echo "       remove it (rm -rf) before re-bootstrapping. this destroys any data already there." >&2
    exit 1
fi

# 2b. Refuse if our target credentials file already exists.
if [ -e "$HOME/.config/beads/credentials" ]; then
    echo "FATAL: $HOME/.config/beads/credentials already exists." >&2
    echo "       remove it (and any associated server) before re-bootstrapping." >&2
    exit 1
fi

# 2c. Refuse if port 5433 is in use.
if ss -tln 2>/dev/null | grep -q ':5433\b'; then
    echo "FATAL: port 5433 is already in use." >&2
    echo "       another process is bound to 127.0.0.1:5433. stop it before re-running." >&2
    exit 1
fi

# 2d. WARN if port 5432 is in use.
if ss -tln 2>/dev/null | grep -q ':5432\b'; then
    echo "WARNING: a PostgreSQL instance appears active on port 5432."
    echo "         this bootstrap installs a SEPARATE private instance on port 5433."
    echo "         the two will coexist; you will have two running PostgreSQL servers."
    echo "         press Enter to continue, or Ctrl-C to abort."
    read -r _
fi

# 2e. Confirm pg_ctl is on PATH and version >= 14.
if ! command -v pg_ctl >/dev/null 2>&1; then
    echo "FATAL: pg_ctl not found on PATH." >&2
    echo "       install PostgreSQL >= 14 from your distribution's package manager." >&2
    exit 1
fi
PG_VERSION_MAJOR=$(pg_ctl --version | awk '{print $NF}' | cut -d. -f1)
if [ "${PG_VERSION_MAJOR:-0}" -lt 14 ]; then
    echo "FATAL: pg_ctl version ${PG_VERSION_MAJOR} < 14." >&2
    echo "       this bootstrap requires PostgreSQL >= 14 (scram-sha-256 defaults)." >&2
    exit 1
fi
```

Note: §2 does not include a service-file check (unlike the systemd-user runbook's
unit-file check in §2a of `postgres-local-bootstrap.md`). Service file detection
varies by init system; each §6 subsection includes its own idempotency check.

## 3. Initialise the data directory

Identical to the systemd-user runbook.

```bash
mkdir -p "$HOME/.local/share/beads"
chmod 700 "$HOME/.local/share/beads"

pg_ctl initdb \
    -D "$HOME/.local/share/beads/postgres/data" \
    -o "--auth-local=peer --auth-host=scram-sha-256 --encoding=UTF8 --locale=C"

chmod 700 "$HOME/.local/share/beads/postgres/data"
```

## 4. Configure the listener

Identical to the systemd-user runbook.

```bash
cat >> "$HOME/.local/share/beads/postgres/data/postgresql.conf" <<'EOF'

# beads-postgres bootstrap overrides (engdocs/postgres-non-systemd-linux-bootstrap.md):
listen_addresses = '127.0.0.1'
port = 5433
unix_socket_directories = '/tmp'
log_destination = 'stderr'
logging_collector = off
EOF
```

## 5. Set the role password

Identical to the systemd-user runbook.

```bash
PG_PASSWORD="$(openssl rand -base64 24)"
echo "Generated PG password (copy this somewhere safe in case this shell dies):"
echo "    $PG_PASSWORD"

pg_ctl -D "$HOME/.local/share/beads/postgres/data" start

psql -h /tmp -p 5433 -U "$USER" -d postgres \
    -c "ALTER ROLE \"$USER\" WITH PASSWORD '$PG_PASSWORD';"

pg_ctl -D "$HOME/.local/share/beads/postgres/data" stop
```

## 6. Install the service unit

**Proceed to the subsection for your init system.**

### 6.1 OpenRC (Gentoo, Alpine Linux)

OpenRC does not have a per-user supervision model in mainstream distro packages.
The service runs as your user via `start-stop-daemon --user`.
**Root access is required** for this subsection (to install into `/etc/init.d/`).

```bash
PG_BIN="$(command -v postgres)"
if [ -z "${PG_BIN}" ]; then
    echo "FATAL: 'postgres' binary not found on PATH." >&2
    echo "       install PostgreSQL >= 14 from your distribution's package manager." >&2
    exit 1
fi

# Idempotency: refuse if the init script already exists.
if [ -f /etc/init.d/beads-postgres ]; then
    echo "FATAL: /etc/init.d/beads-postgres already exists." >&2
    echo "       remove it before re-bootstrapping." >&2
    exit 1
fi

sudo tee /etc/init.d/beads-postgres > /dev/null <<EOF
#!/sbin/openrc-run

name="beads-postgres"
description="beads-postgres (private PostgreSQL instance for bd PG-backed scopes)"
pidfile="/run/beads-postgres.pid"
command="${PG_BIN}"
command_args="-D ${HOME}/.local/share/beads/postgres/data"
command_user="${USER}"
start_stop_daemon_args="--background --make-pidfile"

depend() {
    need net
    after bootmisc
}
EOF

sudo chmod 755 /etc/init.d/beads-postgres
sudo rc-update add beads-postgres default
sudo rc-service beads-postgres start
```

Verify: `rc-service beads-postgres status` should show `started`.

To read logs: OpenRC routes stderr to the system logger. Check `dmesg` or the init
system's log file (e.g., `/var/log/messages` on Gentoo, `/var/log/openrc` on Alpine).

### 6.2 runit (Void Linux)

Void Linux supports per-user runit services via `~/.local/sv/` and `~/.local/service/`.
The user's `runsvdir` must be running; see the Void Linux handbook for details on
enabling per-user services (`sv` user supervision).

```bash
PG_BIN="$(command -v postgres)"
if [ -z "${PG_BIN}" ]; then
    echo "FATAL: 'postgres' binary not found on PATH." >&2
    echo "       install PostgreSQL >= 14 from your distribution's package manager." >&2
    exit 1
fi

# Idempotency: refuse if the service directory already exists.
if [ -e "$HOME/.local/sv/beads-postgres" ]; then
    echo "FATAL: $HOME/.local/sv/beads-postgres already exists." >&2
    echo "       remove it before re-bootstrapping." >&2
    exit 1
fi

mkdir -p "$HOME/.local/sv/beads-postgres"
cat > "$HOME/.local/sv/beads-postgres/run" <<EOF
#!/bin/sh
exec 2>&1
exec "${PG_BIN}" -D "${HOME}/.local/share/beads/postgres/data"
EOF
chmod 755 "$HOME/.local/sv/beads-postgres/run"

# Create a log directory and logging script (optional but recommended).
mkdir -p "$HOME/.local/sv/beads-postgres/log"
cat > "$HOME/.local/sv/beads-postgres/log/run" <<'LOGEOF'
#!/bin/sh
exec svlogd -tt ./main
LOGEOF
chmod 755 "$HOME/.local/sv/beads-postgres/log/run"
mkdir -p "$HOME/.local/sv/beads-postgres/log/main"

# Enable by symlinking into the service directory.
mkdir -p "$HOME/.local/service"
ln -s "$HOME/.local/sv/beads-postgres" "$HOME/.local/service/beads-postgres"
```

After the symlink, if `runsvdir ~/.local/service` is running, the service starts
within 5 seconds. Verify with `sv status ~/.local/sv/beads-postgres`.

**Starting `runsvdir` at login (Void Linux).** If not already configured, add to
`~/.xinitrc` (X11) or your display manager session:

```bash
runsvdir "$HOME/.local/service" &
```

Or use the system-level `usersvd` package (if available for your Void Linux installation)
to automatically start per-user runit trees at login.

**Log access.** With the log script above, logs accumulate in
`~/.local/sv/beads-postgres/log/main/current`. Read with `tail -f` or `svlogd`.

### 6.3 s6 (Alpine with s6-overlay)

s6 is most commonly encountered on Alpine Linux in container environments (via
`s6-overlay`) rather than on bare developer machines. These instructions target
the s6-overlay 3.x per-user service convention.

This subsection covers the common case of s6 running alongside an existing s6
supervision tree. If you are setting up s6 from scratch, consult the s6-overlay
documentation for your distro before proceeding.

```bash
PG_BIN="$(command -v postgres)"
if [ -z "${PG_BIN}" ]; then
    echo "FATAL: 'postgres' binary not found on PATH." >&2
    echo "       install PostgreSQL >= 14 from your distribution's package manager." >&2
    exit 1
fi

# s6 user service directory. Adjust S6_USER_SERVICES_DIR if your s6-overlay
# uses a different path (e.g., /run/user/${UID}/service).
S6_USER_SERVICES_DIR="${HOME}/.s6/service"

# Idempotency: refuse if the service directory already exists.
if [ -e "${S6_USER_SERVICES_DIR}/beads-postgres" ]; then
    echo "FATAL: ${S6_USER_SERVICES_DIR}/beads-postgres already exists." >&2
    echo "       remove it before re-bootstrapping." >&2
    exit 1
fi

mkdir -p "${S6_USER_SERVICES_DIR}/beads-postgres"
cat > "${S6_USER_SERVICES_DIR}/beads-postgres/run" <<EOF
#!/bin/execlineb -P
fdmove -c 2 1
${PG_BIN} -D ${HOME}/.local/share/beads/postgres/data
EOF
chmod 755 "${S6_USER_SERVICES_DIR}/beads-postgres/run"

# Notify s6 supervisor of the new service.
if command -v s6-svscanctl >/dev/null 2>&1; then
    s6-svscanctl -a "${S6_USER_SERVICES_DIR}" 2>/dev/null || true
fi
```

Verify: `s6-svstat ${S6_USER_SERVICES_DIR}/beads-postgres` should show `up`.

**Note on s6 service paths.** The s6 supervision tree root (`S6_USER_SERVICES_DIR`)
varies by distro and configuration. Common paths:
- `~/.s6/service` (user-level, if configured with s6-overlay 3.x user bundles)
- `/run/user/${UID}/service` (runtime directory approach)

If neither applies to your setup, consult your distro's s6-overlay documentation.
This runbook uses `~/.s6/service` as the default; override `S6_USER_SERVICES_DIR`
before running.

## 7. Populate the credentials file

Identical to the systemd-user runbook.

```bash
mkdir -p "$HOME/.config/beads"
cat > "$HOME/.config/beads/credentials" <<EOF
[127.0.0.1:5433]
password=$PG_PASSWORD
EOF
chmod 600 "$HOME/.config/beads/credentials"

unset PG_PASSWORD
```

## 8. Configure the user-services environment

No `environment.d` equivalent on non-systemd Linux. Add to your shell rc instead.

```bash
# For bash (common on Gentoo, Alpine, Void):
echo 'export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"' >> "$HOME/.bashrc"

# For zsh:
# echo 'export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"' >> "$HOME/.zshrc"
```

Export manually for the current session:

```bash
export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"
```

## 9. Boot survival notes

**OpenRC.** `rc-update add beads-postgres default` (§6.1) schedules the service
to start in the `default` runlevel, which runs at boot. Boot survival is automatic
after §6.1 completes. `sudo rc-service beads-postgres start` starts it for the
current session.

**runit (Void Linux).** Boot survival depends on `runsvdir` starting at boot. On
Void Linux, per-user `runsvdir` is typically configured via `usersvd` or a login
hook. If the service is not running after reboot, check that `runsvdir ~/.local/service`
runs at login. No additional step is needed beyond §6.2 if your Void Linux setup
already runs per-user runit services.

**s6.** Boot survival depends on your s6-overlay scan directory being supervised at
boot. If your s6 supervision tree starts at boot (common in s6-overlay setups), the
service starts automatically when the scan directory is rescanned. Verify with your
distro's s6-overlay boot documentation.

## 10. Verify

Run `gc doctor` in the city or rig targeting the PG-backed scope:

```
✓ postgres-server: reachable at 127.0.0.1:5433
✓ postgres-auth:   credentials resolved from ~/.config/beads/credentials
```

If `postgres-server` reports a `✗` error:

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| `server not reachable at 127.0.0.1:5433` | Service failed to start | Check init-system logs for your distro (see §6 per-init notes) |
| `local PG not installed yet — see engdocs/postgres-non-systemd-linux-bootstrap.md for one-time setup` | This doc was not run | Run this doc |
| `auth failed` | postgres-auth cannot read the credentials file | Confirm `chmod 600 ~/.config/beads/credentials` |

`gc doctor --explain-postgres-non-systemd-linux-bootstrap` reprints this document
as a copy-pastable shell script.

## 11. Uninstallation

**OpenRC:**

```bash
sudo rc-service beads-postgres stop || true
sudo rc-update del beads-postgres default || true
sudo rm -f /etc/init.d/beads-postgres
rm -rf "$HOME/.local/share/beads/postgres"
rm -f "$HOME/.config/beads/credentials"
echo "remove 'export BEADS_CREDENTIALS_FILE=...' from your shell rc if present."
```

**runit (Void Linux):**

```bash
sv stop "$HOME/.local/service/beads-postgres" || true
rm -f "$HOME/.local/service/beads-postgres"   # remove symlink
rm -rf "$HOME/.local/sv/beads-postgres"
rm -rf "$HOME/.local/share/beads/postgres"
rm -f "$HOME/.config/beads/credentials"
echo "remove 'export BEADS_CREDENTIALS_FILE=...' from your shell rc if present."
```

**s6:**

```bash
S6_USER_SERVICES_DIR="${HOME}/.s6/service"
s6-svc -d "${S6_USER_SERVICES_DIR}/beads-postgres" 2>/dev/null || true
rm -rf "${S6_USER_SERVICES_DIR}/beads-postgres"
rm -rf "$HOME/.local/share/beads/postgres"
rm -f "$HOME/.config/beads/credentials"
echo "remove 'export BEADS_CREDENTIALS_FILE=...' from your shell rc if present."
```
