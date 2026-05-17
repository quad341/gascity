package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/gastownhall/gascity/engdocs"
)

var postgresBootstrapDoc = engdocs.PostgresLocalBootstrap()

func printPostgresBootstrapDoc(w io.Writer) error {
	doc := postgresBootstrapDoc
	if !strings.HasSuffix(doc, "\n") {
		doc += "\n"
	}
	_, err := fmt.Fprint(w, doc)
	return err
}
