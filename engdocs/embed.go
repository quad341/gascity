// Package engdocs exposes embedded copies of operator runbooks.
package engdocs

import _ "embed"

//go:embed postgres-local-bootstrap.md
var postgresLocalBootstrap string

// PostgresLocalBootstrap returns the embedded local PostgreSQL bootstrap runbook.
func PostgresLocalBootstrap() string {
	return postgresLocalBootstrap
}
