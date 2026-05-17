---
title: "Local PostgreSQL bootstrap for bd PG-backed scopes"
description: "One-time setup of a private systemd-user PostgreSQL instance on 127.0.0.1:5433 for local development of bd PG-backed scopes."
---

This document describes how to set up a private PostgreSQL
instance on a Linux developer machine for use as the storage
backend of one or more PG-backed bd scopes. The result is a
PostgreSQL server running under your user account via
`systemd --user`, listening on `127.0.0.1:5433` only, surviving
reboots via `loginctl enable-linger`, and reachable via the
slice-2 INI credentials file at `~/.config/beads/credentials`.

This bootstrap is **opt-in** and **manual**. `gc init` and
`gc destroy` do not invoke any of the steps below, do not write
any of the files, and do not require this server to exist.
PG-backed scopes that target a different (cloud, container,
externally-managed) PostgreSQL endpoint do not need this doc.

The long-term home for this sequence is upstream `bd`, as
`bd init --backend=postgres --bootstrap-local`. Until that lands,
the steps below stand in. An operator (or an agent) following
this document verbatim succeeds. The doc is idempotent: a second
run on the same machine refuses to proceed when prior artefacts
exist, with explicit error messages identifying which artefact
blocked re-bootstrap.

`gc doctor --explain-postgres-bootstrap` prints the same
sequence as a single copy-pastable shell script (see §10).

## 1. Audience and prerequisites

**Audience.** Developers running bd-backed scopes on a Linux
development machine, where PostgreSQL is the chosen storage
backend for one or more scopes (per `MetadataState.Backend`).
Developers using the Dolt backend do not need this doc;
developers targeting a remote/cloud PostgreSQL do not need this
doc.

**Outcome.** A private PostgreSQL ≥ 14 server, owned by your
user account, listening on `127.0.0.1:5433` only, started and
restarted by `systemd --user` with `loginctl enable-linger`
configured so the server survives logout and reboot. A
slice-2-compatible credentials file at
`~/.config/beads/credentials` containing the generated role
password. An `environment.d` snippet so user services see
`BEADS_CREDENTIALS_FILE`.

**What you must have on PATH.**

- `pg_ctl`, `postgres`, `psql` from PostgreSQL ≥ 14.
- `systemctl`, `loginctl` from systemd.
- `ss` (from `iproute2`) for port detection.
- `openssl` for password generation.
- `sudo` (for the linger step only).

**Platform.** This doc targets Linux with systemd-user available
(`/run/systemd/system` exists). macOS (launchd) and BSD (rc.d /
runit / OpenRC) equivalents are out of scope; follow-on engdocs
will cover them.

**Cross-version compatibility.** Tested on PostgreSQL 14, 15, 16.
The doc does not claim PostgreSQL 13 or earlier compatibility:
those versions ship with `md5` defaults in `pg_hba.conf` whereas
PostgreSQL ≥ 14 ships with `scram-sha-256`, and the credentials
flow assumes scram. If you must target an older PostgreSQL,
adjust §4's `pg_hba.conf` notes accordingly and use a server-side
password hashing scheme compatible with your version.

**What this doc does NOT modify.**

- System-wide PostgreSQL service units (under
  `/etc/systemd/system/`).
- System-installed PostgreSQL data directories (under
  `/var/lib/postgresql/`).
- Your shell rc files (`~/.bashrc`, `~/.zshrc`, etc.).
- `gc` configuration or any city/rig directory.

If you have a system-installed PostgreSQL on the default port
`:5432`, this bootstrap is safe to run alongside it; the private
instance uses port `:5433`.

## 2. Detect existing state

Before doing anything destructive, refuse to proceed when prior
state would be clobbered. The doc names every artefact it
creates so re-runs are explicit about which artefact blocked
progress.

```bash
set -euo pipefail

# 2a. Refuse if our target unit file already exists. The
# operator must remove it explicitly to re-bootstrap.
if [ -f "$HOME/.config/systemd/user/beads-postgres.service" ]; then
    echo "FATAL: $HOME/.config/systemd/user/beads-postgres.service already exists." >&2
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
if ss -tln | grep -q ':5433\b'; then
    echo "FATAL: port 5433 is already in use." >&2
    echo "       another process is bound to 127.0.0.1:5433. stop it before re-running." >&2
    exit 1
fi

# 2e. WARN (do not refuse) if a system PostgreSQL appears active on :5432.
if ss -tln | grep -q ':5432\b'; then
    echo "WARNING: a PostgreSQL instance appears active on port 5432."
    echo "         this bootstrap installs a SEPARATE private instance on port 5433."
    echo "         the two will coexist; you will have two running PostgreSQL servers."
    echo "         press Enter to continue, or Ctrl-C to abort."
    read -r _
fi

# 2f. Confirm pg_ctl is on PATH and version >= 14.
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

## 3. Initialize the data directory

Create the data directory and run `initdb`. The data directory
is owned by your user with mode `700`; PostgreSQL refuses to
start otherwise. The `pg_ctl initdb` form is the modern wrapper;
`initdb` directly works equivalently.

```bash
mkdir -p "$HOME/.local/share/beads"
chmod 700 "$HOME/.local/share/beads"

pg_ctl initdb \
    -D "$HOME/.local/share/beads/postgres/data" \
    -o "--auth-local=peer --auth-host=scram-sha-256 --encoding=UTF8 --locale=C"

# initdb sets mode 700 on the data dir already; reassert
# defensively.
chmod 700 "$HOME/.local/share/beads/postgres/data"
```

The `--auth-local=peer` setting is what allows the next step
(setting the role password) to connect via Unix socket without a
password. The `--auth-host=scram-sha-256` setting is the default
on PostgreSQL ≥ 14 but pinned explicitly so the doc is robust
against initdb default changes. `--encoding=UTF8 --locale=C` is
the operator-friendliness pin; the encoding handles any byte
sequence and the C locale avoids LC_COLLATE surprises in tests.

## 4. Configure the listener

`initdb` writes a `postgresql.conf` with sensible defaults
(shared_buffers, work_mem, etc.). We keep those defaults and
only override what we must: listen address, port, and Unix
socket directory. Append rather than overwrite so the defaults
survive.

```bash
cat >> "$HOME/.local/share/beads/postgres/data/postgresql.conf" <<'EOF'

# beads-postgres bootstrap overrides (engdocs/postgres-local-bootstrap.md):
listen_addresses = '127.0.0.1'
port = 5433
unix_socket_directories = '/tmp'
log_destination = 'stderr'
logging_collector = off
EOF
```

`pg_hba.conf` is left as-is. PostgreSQL ≥ 14's `initdb` defaults
to `local all all peer` for Unix sockets and
`host all all 127.0.0.1/32 scram-sha-256` for TCP — exactly what
we want. Do not append to `pg_hba.conf`; redundant lines hide
authentication intent.

`log_destination = 'stderr'` and `logging_collector = off` send
PostgreSQL's logs to systemd-journal via the unit's stderr; you
read them with `journalctl --user -u beads-postgres`. We
deliberately do NOT use `log_destination = 'syslog'` (architect's
draft suggestion); journal is the systemd-native pathway and
needs no `syslog_ident` configuration.

## 5. Set the role password

Generate a password and set it on the per-user PostgreSQL role.
The role was created by `initdb` (PostgreSQL creates a superuser
role matching the OS user during initdb). We connect via Unix
socket on `/tmp` so peer auth lets us in without a password we
do not yet have.

```bash
PG_PASSWORD="$(openssl rand -base64 24)"
echo "Generated PG password (copy this somewhere safe in case this shell dies):"
echo "    $PG_PASSWORD"

# Start PG in the foreground for password-set, in a transient way:
pg_ctl -D "$HOME/.local/share/beads/postgres/data" start

# Connect via Unix socket on /tmp (peer auth, no password needed
# at this stage). DO NOT use -h 127.0.0.1 here — TCP requires
# scram-sha-256 and we have not set the password yet.
psql -h /tmp -p 5433 -U "$USER" -d postgres \
    -c "ALTER ROLE \"$USER\" WITH PASSWORD '$PG_PASSWORD';"

# Stop the transient instance; systemd will start it again in §6.
pg_ctl -D "$HOME/.local/share/beads/postgres/data" stop
```

The password is held only in the `PG_PASSWORD` shell variable
for the duration of this session and copied into the credentials
file in §7. Do not commit it anywhere; do not print it to a log.
The single `echo` to stdout is your safety net in case the
shell dies between this step and §7.

## 6. Install the systemd-user unit

Resolve the absolute path to the `postgres` binary, then write a
systemd-user service that runs it under your account. The
binary path is resolved at install time (not via `$(which
postgres)` inside the unit file, which systemd does not expand)
and substituted into the unit file via shell variable expansion.
The unit file path is
`~/.config/systemd/user/beads-postgres.service`.

```bash
PG_BIN="$(command -v postgres)"
if [ -z "${PG_BIN}" ]; then
    echo "FATAL: 'postgres' binary not found on PATH." >&2
    echo "       install PostgreSQL >= 14 (the same major version as pg_ctl in §3)." >&2
    exit 1
fi

mkdir -p "$HOME/.config/systemd/user"
cat > "$HOME/.config/systemd/user/beads-postgres.service" <<EOF
[Unit]
Description=beads-postgres (private PostgreSQL instance for bd PG-backed scopes)
Documentation=file://$HOME/.local/share/beads/postgres/data/postgresql.conf
After=network.target

[Service]
Type=notify
ExecStart=${PG_BIN} -D %h/.local/share/beads/postgres/data
KillMode=mixed
KillSignal=SIGINT
Restart=on-failure
RestartSec=5s
TimeoutStopSec=120s

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now beads-postgres
```

`Type=notify` requires PostgreSQL to support sd_notify. All
distribution packages of PostgreSQL ≥ 14 (Debian, Fedora, Arch,
Alpine, etc.) ship with `--with-systemd`; if your `postgres`
binary was built without it, switch the unit to `Type=simple`.

`%h` is systemd's expansion for `$HOME` (used inside the unit
file because systemd evaluates it at runtime; the literal home
path is not baked in).

`Documentation=file://...` is a courtesy: `systemctl --user
status beads-postgres` shows it, pointing operators at the
running config.

## 7. Populate the credentials file

Write the slice-2 INI-format credentials file. Mode `600`. The
file format is `[<host>:<port>]` section header with
`password=<password>` body — exactly the format the slice-2
credentials reader consumes.

```bash
mkdir -p "$HOME/.config/beads"
cat > "$HOME/.config/beads/credentials" <<EOF
[127.0.0.1:5433]
password=$PG_PASSWORD
EOF
chmod 600 "$HOME/.config/beads/credentials"

unset PG_PASSWORD
```

The final `unset PG_PASSWORD` clears the shell variable. The
password is now persisted only in the credentials file (mode
`600`) and in PostgreSQL's role catalogue (encrypted with
scram-sha-256).

If `~/.config/beads/credentials` did not exist before this
bootstrap, the `cat >` is the first writer. If it did exist,
§2c stopped you before reaching this step.

## 8. Configure the user-services environment

Write a single-line `environment.d` snippet so user services
(systemd-user units) see `BEADS_CREDENTIALS_FILE` at startup.
This is required only if you run bd or gc as a systemd-user
service; for shells, your rc file (`~/.bashrc`, `~/.zshrc`)
handles it.

```bash
mkdir -p "$HOME/.config/environment.d"
cat > "$HOME/.config/environment.d/beads-postgres.conf" <<'EOF'
BEADS_CREDENTIALS_FILE=%h/.config/beads/credentials
EOF
```

The current shell does **not** see this until next login.
Export it manually for the rest of this session, and add it to
your shell rc so future shells pick it up:

```bash
export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"
```

The `%h` placeholder is systemd's `$HOME` expansion (just like
the unit file in §6). Shell rc lines should use the literal
`$HOME` form instead.

## 9. Enable linger (mandatory for boot survival)

Without `loginctl enable-linger`, your `systemd --user` instance
stops at logout. The PostgreSQL unit you installed in §6 stops
with it; on next reboot, nothing starts the server.

This step requires `sudo` and modifies system-level state in
`/var/lib/systemd/linger/`. Read the prompt below carefully
before pressing Enter — once enabled, your user's systemd
instance runs continuously, including across reboots, regardless
of whether you log in.

```bash
echo
echo "About to enable systemd-user linger for $USER."
echo
echo "What this does:"
echo "  • runs 'sudo loginctl enable-linger $USER'"
echo "  • creates /var/lib/systemd/linger/$USER (system-level state)"
echo "  • makes your user's systemd instance start at boot, even when you are not logged in"
echo "  • makes your beads-postgres unit survive reboots"
echo
echo "What this does NOT do:"
echo "  • does NOT modify your shell, login, or PAM configuration"
echo "  • does NOT change your user's privileges"
echo "  • does NOT install any system service"
echo
echo "To disable later: sudo loginctl disable-linger $USER (see §11)."
echo
echo "Press Enter to confirm and run the sudo command, or Ctrl-C to abort."
read -r _

sudo loginctl enable-linger "$USER"
```

If you abort here (`Ctrl-C`) the bootstrap is incomplete: PG is
running for this session but will stop at logout. You can re-run
just this step later by running `sudo loginctl enable-linger
$USER` directly.

## 10. Verify

Run `gc doctor` in the city or rig that targets the PG-backed
scope. Expected output:

```
✓ postgres-server: reachable at 127.0.0.1:5433
✓ postgres-auth:   credentials resolved from ~/.config/beads/credentials
```

If `postgres-server` reports a `⚠` warning about boot-survival,
re-run §9 (linger was not enabled or did not take effect).

If `postgres-server` reports a `✗` error, the most common causes
are:

| Symptom                                                 | Likely cause                                                               | Fix                                                                       |
|---------------------------------------------------------|----------------------------------------------------------------------------|---------------------------------------------------------------------------|
| `server not reachable at 127.0.0.1:5433`                | unit failed to start                                                       | `journalctl --user -u beads-postgres -n 50`                               |
| `metadata missing postgres host/port; cannot probe`     | scope `metadata.json` does not have `PostgresHost`/`PostgresPort` set yet  | edit the scope's metadata, or re-run `gc init` for the scope               |
| `local PG not installed yet — see engdocs/postgres-local-bootstrap.md for one-time setup` | this doc was not run | run this doc                                                              |
| `auth failed`                                           | `postgres-auth` cannot read the credentials file                           | confirm `chmod 600 ~/.config/beads/credentials` and `BEADS_CREDENTIALS_FILE` is exported |

`gc doctor --explain-postgres-bootstrap` reprints this entire
document as a single copy-pastable script, in case you need to
re-bootstrap on a fresh machine.

## 11. Uninstallation

Reverse the bootstrap in the inverse order. The data directory
removal is destructive; back up first if the database has
anything you want to keep.

```bash
# 11a. Stop and disable the unit.
systemctl --user disable --now beads-postgres

# 11b. Remove the unit file.
rm -f "$HOME/.config/systemd/user/beads-postgres.service"

# 11c. Remove the data directory (DESTRUCTIVE — back up first if needed).
rm -rf "$HOME/.local/share/beads/postgres"

# 11d. Remove the credentials file (this destroys the password).
rm -f "$HOME/.config/beads/credentials"

# 11e. Remove the environment.d snippet.
rm -f "$HOME/.config/environment.d/beads-postgres.conf"

# 11f. Reload systemd-user so the unit is forgotten.
systemctl --user daemon-reload

# 11g. Optionally disable linger if you have no other reason to
# keep it. Skip this step if other systemd-user services (e.g.
# tmux-resurrect, syncthing) require linger to be on.
read -r -p "Also disable systemd-user linger for $USER? [y/N] " ANS
if [ "${ANS:-N}" = "y" ] || [ "${ANS:-N}" = "Y" ]; then
    sudo loginctl disable-linger "$USER"
fi

# 11h. Remember to remove BEADS_CREDENTIALS_FILE from your shell
# rc file if you added it in §8.
echo "remove 'export BEADS_CREDENTIALS_FILE=...' from your shell rc if present."
```

After uninstallation, `gc doctor` against any remaining
PG-backed scope will report `✗ postgres-server: server not
reachable at 127.0.0.1:5433` with the FixHint pointing back at
this document — the symmetric inverse of the install case.
