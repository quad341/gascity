package main

import (
	"io"

	"github.com/gastownhall/gascity/engdocs"
)

func writePostgresNonSystemdBootstrapExplain(w io.Writer) error {
	_, err := io.WriteString(w, engdocs.PostgresNonSystemdLinuxBootstrap())
	return err
}
