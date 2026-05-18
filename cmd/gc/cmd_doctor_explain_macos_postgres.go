package main

import (
	_ "embed"
	"io"
)

//go:embed doctor_docs/postgres-macos-launchd-bootstrap.md
var postgresMacOSBootstrapDoc string

func writePostgresMacOSBootstrapDoc(w io.Writer) error {
	_, err := io.WriteString(w, postgresMacOSBootstrapDoc)
	return err
}
