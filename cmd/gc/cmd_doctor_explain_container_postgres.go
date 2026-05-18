package main

import (
	_ "embed"
	"io"
)

//go:embed doctor_docs/postgres-container-bootstrap.md
var postgresContainerBootstrapDoc string

func writePostgresContainerBootstrapDoc(w io.Writer) error {
	_, err := io.WriteString(w, postgresContainerBootstrapDoc)
	return err
}
