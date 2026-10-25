package main

import (
	"encoding/csv"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestManagedDoltTableRefsFromCSV(t *testing.T) {
	for _, tc := range []struct {
		name string
		csv  string
		want []managedDoltTableRef
	}{
		{name: "empty"},
		{name: "header only", csv: "table_schema,table_name\n"},
		{
			name: "multiple databases and CRLF",
			csv:  "table_schema,table_name\r\nqa,wisps\r\nother,events\r\n",
			want: []managedDoltTableRef{{DB: "qa", Table: "wisps"}, {DB: "other", Table: "events"}},
		},
		{
			name: "quoted identifiers",
			csv:  "table_schema,table_name\n\"db,one\",\"table\"\"quoted\"\n\"db\n二\",events\n",
			want: []managedDoltTableRef{{DB: "db,one", Table: "table\"quoted"}, {DB: "db\n二", Table: "events"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := managedDoltTableRefsFromCSV(tc.csv)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("discovery refs = %#v, %v; want %#v, nil", got, err, tc.want)
			}
		})
	}
}

func TestManagedDoltTableRefsFromCSVRejectsMalformedOutput(t *testing.T) {
	for _, tc := range []struct{ name, csv string }{
		{"short header", "table_schema\n"},
		{"short row", "table_schema,table_name\nqa\n"},
		{"long row", "table_schema,table_name\nqa,wisps,extra\n"},
		{"unclosed quote", "table_schema,table_name\nqa,\"wisps\n"},
		{"valid prefix then malformed row", "table_schema,table_name\nqa,wisps\ninvalid\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := managedDoltTableRefsFromCSV(tc.csv)
			var parseErr *csv.ParseError
			if !errors.As(err, &parseErr) || !strings.Contains(err.Error(), "parse dolt-ignored table list csv") {
				t.Fatalf("error = %v, want contextual wrapped CSV parse error", err)
			}
			if got != nil {
				t.Fatalf("malformed discovery returned partial refs: %#v", got)
			}
		})
	}
}
