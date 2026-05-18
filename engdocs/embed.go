// Package engdocs exposes embedded engineering documentation used by gc commands.
package engdocs

import _ "embed"

//go:embed postgres-non-systemd-linux-bootstrap.md
var postgresNonSystemdLinuxBootstrap string

// PostgresNonSystemdLinuxBootstrap returns the non-systemd Linux PostgreSQL bootstrap runbook.
func PostgresNonSystemdLinuxBootstrap() string {
	return postgresNonSystemdLinuxBootstrap
}
