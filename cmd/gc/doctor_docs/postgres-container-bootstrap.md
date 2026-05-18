---
title: "Local PostgreSQL bootstrap for bd PG-backed scopes (docker-compose / podman-compose)"
description: "Container-runtime bootstrap of a private PostgreSQL 16 instance on 127.0.0.1:5433 for local development of bd PG-backed scopes."
---

This document describes how to run a private PostgreSQL instance via a container
runtime (`docker compose` or `podman-compose`) as the storage backend for one or
more PG-backed bd scopes. The result is a PostgreSQL 16 container listening on
`127.0.0.1:5433`, restarting automatically with the container runtime, and reachable
via the slice-2 INI credentials file at `~/.config/beads/credentials`.

This bootstrap is **opt-in** and **manual**. `gc init` and `gc destroy` do not
invoke any of the steps below and do not require this server to exist.

This doc is the container-runtime alternative to `engdocs/postgres-local-bootstrap.md`
(Linux systemd-user host-installed) and
`engdocs/postgres-macos-launchd-bootstrap.md` (macOS launchd host-installed). The
port (5433) and credentials file format are identical. No host-side `initdb`,
`postgresql.conf` edits, or service unit installation is needed — the official
`postgres:16` image handles all of that.

`gc doctor --explain-postgres-container-bootstrap` prints the same sequence as a
copy-pastable shell script (see §10).

## 1. Audience and prerequisites

**Audience.** Developers who have Docker Desktop or a compatible Docker Engine /
Podman setup and prefer not to install PostgreSQL directly on the host.

**Outcome.** A `beads-postgres` container running `postgres:16`, publishing port
5433 on `127.0.0.1`, backed by a bind-mounted data directory at
`~/.local/share/beads/postgres/data`, with a slice-2 credentials file at
`~/.config/beads/credentials`.

**What you must have.**

- `docker compose` (Docker Desktop 4.x+ or Docker Engine with Compose Plugin v2+),
  OR `podman-compose` 1.x+ with Podman 4.x+.
  Verify: `docker compose version` or `podman-compose --version`.
- `openssl` for password generation.
- No PostgreSQL binaries (`pg_ctl`, `psql`) are needed on the host.

**Platform.** Linux or macOS. On Linux, Docker Engine (or Podman) with the Compose
Plugin. On macOS, Docker Desktop.

**Boot survival semantics.**

- *Linux (Docker Engine with systemd):* `restart: unless-stopped` restarts the container
  when Docker starts. Docker itself starts at boot if `systemctl enable docker` (or
  `systemctl enable podman.socket`) was run. The doc notes this in §9.
- *macOS (Docker Desktop):* Docker Desktop starts at login and respects
  `restart: unless-stopped`. Boot survival is login-scoped, matching the macOS launchd
  doc.

**What this doc does NOT modify.**

- Any system-installed PostgreSQL instance or data directory.
- Your shell rc files (except the OPTIONAL export in §8).
- `gc` configuration or any city/rig directory.
- Host network configuration beyond binding `127.0.0.1:5433`.

## 2. Detect existing state

```bash
set -euo pipefail

# 2a. Refuse if the beads-postgres container already exists (running or stopped).
if docker inspect beads-postgres >/dev/null 2>&1; then
    echo "FATAL: a container named 'beads-postgres' already exists." >&2
    echo "       stop and remove it (docker rm -f beads-postgres) before re-bootstrapping." >&2
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
    echo "       remove it (and any associated container) before re-bootstrapping." >&2
    exit 1
fi

# 2d. Refuse if our compose file already exists.
if [ -e "$HOME/.config/beads/docker-compose.yml" ]; then
    echo "FATAL: $HOME/.config/beads/docker-compose.yml already exists." >&2
    echo "       remove it before re-bootstrapping." >&2
    exit 1
fi

# 2e. Refuse if port 5433 is in use.
if docker run --rm --network host busybox sh -c 'nc -z 127.0.0.1 5433' 2>/dev/null; then
    echo "FATAL: port 5433 is already in use." >&2
    echo "       another process is bound to 127.0.0.1:5433. stop it before re-running." >&2
    exit 1
fi

# 2f. WARN if port 5432 is in use.
if docker run --rm --network host busybox sh -c 'nc -z 127.0.0.1 5432' 2>/dev/null; then
    echo "WARNING: a PostgreSQL instance appears active on port 5432."
    echo "         this bootstrap installs a SEPARATE container on port 5433."
    echo "         the two will coexist."
    echo "         press Enter to continue, or Ctrl-C to abort."
    read -r _
fi

# 2g. Confirm the compose command is available.
COMPOSE_CMD=""
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
elif command -v podman-compose >/dev/null 2>&1; then
    COMPOSE_CMD="podman-compose"
else
    echo "FATAL: neither 'docker compose' nor 'podman-compose' found on PATH." >&2
    echo "       install Docker Desktop or 'pip install podman-compose' before re-running." >&2
    exit 1
fi
```

Note on port detection: `docker run --rm --network host busybox sh -c 'nc -z ...'`
is used instead of `ss -tln` (Linux) or `lsof` (macOS) so the same script works on
both platforms. The one-shot busybox container adds a short startup overhead; the port
check is non-interactive and the overhead is acceptable at bootstrap time.

## 3. Generate the password and write the compose file

```bash
PG_PASSWORD="$(openssl rand -base64 24)"
echo "Generated PG password (copy this somewhere safe in case this shell dies):"
echo "    $PG_PASSWORD"

mkdir -p "$HOME/.local/share/beads/postgres"
chmod 700 "$HOME/.local/share/beads/postgres"
mkdir -p "$HOME/.config/beads"

# Write the compose file with paths resolved at install time.
cat > "$HOME/.config/beads/docker-compose.yml" <<EOF
services:
  postgres:
    image: postgres:16
    container_name: beads-postgres
    environment:
      POSTGRES_USER: "${USER}"
      POSTGRES_PASSWORD: "\${POSTGRES_PASSWORD}"
      POSTGRES_DB: postgres
      POSTGRES_INITDB_ARGS: "--auth-host=scram-sha-256 --encoding=UTF8 --locale=C"
    ports:
      - "127.0.0.1:5433:5432"
    volumes:
      - "${HOME}/.local/share/beads/postgres/data:/var/lib/postgresql/data"
    restart: unless-stopped
EOF

# Write the .env file alongside the compose file (mode 600 — contains the password).
cat > "$HOME/.config/beads/.env" <<EOF
POSTGRES_PASSWORD=${PG_PASSWORD}
EOF
chmod 600 "$HOME/.config/beads/.env"
```

The compose file references `${POSTGRES_PASSWORD}` (a single `$` in the heredoc,
escaped as `"\${POSTGRES_PASSWORD}"` so the shell does not expand it while writing).
When `docker compose` runs, it loads the adjacent `.env` file and substitutes the
variable.

The `.env` file is the sole persistent store of the password for this bootstrap (mode
`600`). The compose file itself is `644` — it contains no sensitive material.

On container restart, `postgres:16` detects a non-empty data directory and skips
`POSTGRES_PASSWORD` processing. The password is baked into the PostgreSQL role catalogue
(scram-sha-256 hash in the data dir). The `.env` file is loaded by compose but the env
var is ignored by postgres after first init.

## 4. Start the container and write the credentials file

```bash
$COMPOSE_CMD \
    --file "$HOME/.config/beads/docker-compose.yml" \
    --project-directory "$HOME/.config/beads" \
    up -d

# Wait for postgres to accept connections (up to 30 s).
echo "waiting for beads-postgres to accept connections on 127.0.0.1:5433..."
for i in $(seq 1 30); do
    if docker exec beads-postgres pg_isready -h 127.0.0.1 -p 5432 -U "$USER" \
           >/dev/null 2>&1; then
        echo "postgres is ready."
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "FATAL: beads-postgres did not become ready within 30 seconds." >&2
        echo "       check logs: docker logs beads-postgres" >&2
        exit 1
    fi
    sleep 1
done

# Write the slice-2 credentials file.
cat > "$HOME/.config/beads/credentials" <<EOF
[127.0.0.1:5433]
password=${PG_PASSWORD}
EOF
chmod 600 "$HOME/.config/beads/credentials"

unset PG_PASSWORD
```

The port-readiness check uses `docker exec ... pg_isready -h 127.0.0.1 -p 5432` (port
5432 inside the container) rather than a host-side TCP probe. This avoids the need for
`pg_isready` on the host and tests the container's internal state directly.

The readiness loop is bounded at 30 iterations × 1 s sleep = 30 s maximum. On typical
hardware, `postgres:16` starts in 2–5 s.

## 5. Configure the shell environment

```bash
# Add to ~/.zshrc (macOS default shell since Catalina, or Fedora with zsh):
echo 'export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"' >> "$HOME/.zshrc"

# If you use bash:
# echo 'export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"' >> "$HOME/.bashrc"
```

Export manually for the current session:

```bash
export BEADS_CREDENTIALS_FILE="$HOME/.config/beads/credentials"
```

## 6. Boot survival

**Linux (Docker Engine with systemd).** The container restarts when Docker starts, and
Docker starts at boot if you ran `sudo systemctl enable docker` (or the equivalent for
your distribution). Verify with:

```bash
systemctl is-enabled docker  # should print "enabled"
```

If `docker` is not enabled at boot, the container will not start until you run
`$COMPOSE_CMD --file ~/.config/beads/docker-compose.yml up -d` manually.

**macOS (Docker Desktop).** Docker Desktop starts at login. The container restarts with
Docker Desktop on next login. Boot survival is login-scoped (same as the macOS launchd
runbook).

**Podman rootless (Linux).** Podman does not start automatically at boot by default.
Enable podman socket at boot:

```bash
systemctl --user enable --now podman.socket
```

Then ensure `lingering` is set (analogous to the Linux systemd-user runbook):

```bash
loginctl enable-linger "$USER"
```

After both steps, `podman-compose up -d` containers with `restart: unless-stopped`
survive reboot.

## 7. Verify

Run `gc doctor` in the city or rig targeting the PG-backed scope:

```
✓ postgres-server: reachable at 127.0.0.1:5433
✓ postgres-auth:   credentials resolved from ~/.config/beads/credentials
```

If `postgres-server` reports a `✗` error:

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| `server not reachable at 127.0.0.1:5433` | Container is not running | `docker ps -a` then `docker logs beads-postgres` |
| `local container runtime PG not running — see engdocs/postgres-container-bootstrap.md for setup` | This doc was not run | Run this doc |
| `auth failed` | Credentials file unreadable or wrong password | Confirm `chmod 600 ~/.config/beads/credentials`; recreate if needed |

`gc doctor --explain-postgres-container-bootstrap` reprints this document as a
copy-pastable shell script.

## 8. Uninstallation

```bash
# 8a. Stop and remove the container.
docker rm -f beads-postgres || true

# 8b. Remove the data directory (DESTRUCTIVE — back up first if needed).
rm -rf "$HOME/.local/share/beads/postgres"

# 8c. Remove the credentials file.
rm -f "$HOME/.config/beads/credentials"

# 8d. Remove the compose and .env files.
rm -f "$HOME/.config/beads/docker-compose.yml" "$HOME/.config/beads/.env"

# 8e. Remove BEADS_CREDENTIALS_FILE from shell rc if you added it in §5.
echo "remember to remove 'export BEADS_CREDENTIALS_FILE=...' from your shell rc if present."
```
