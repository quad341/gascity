//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagedDoltDataReadDirectDiscoversAndReadsThroughDriver(t *testing.T) {
	log := installDataReadDoltCLI(t, "", "unexpected CLI discovery", "unexpected CLI read")
	t.Setenv("GC_DOLT_PASSWORD", "probe-secret")
	discovery := managedDoltIgnoredTableDiscoverySQL()
	first := "SELECT * FROM `qa-db`.`wisps` LIMIT 1"
	second := "SELECT * FROM `qa-db`.`events` LIMIT 1"
	host, port, queries := serveDataReadMySQL(t, map[string]dataReadMySQLReply{
		discovery: {fields: []string{"table_schema", "table_name"}, rows: [][]string{{"qa-db", "wisps"}, {"qa-db", "events"}}},
		first:     {fields: []string{"id"}, rows: [][]string{{"existing-row"}}},
		second:    {fields: []string{"id"}}, // An empty existing table is healthy too.
	})
	if got := managedDoltDataReadProbe(host, port, "root"); got != (managedDoltDataReadResult{}) {
		t.Fatalf("passworded probe = %+v, want healthy", got)
	}
	if got, want := queries(), []string{discovery, first, second}; !reflect.DeepEqual(got, want) {
		t.Fatalf("driver queries = %q, want %q", got, want)
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("passworded probe invoked CLI: log stat = %v", err)
	}
}

func TestManagedDoltDataReadDirectDiscoveryErrors(t *testing.T) {
	t.Setenv("GC_DOLT_PASSWORD", "probe-secret")
	for _, tc := range []struct {
		name, want string
		reply      dataReadMySQLReply
	}{
		{"query error", "catalog unavailable", dataReadMySQLReply{queryErr: "catalog unavailable"}},
		{"scan error", "Scan", dataReadMySQLReply{fields: []string{"table_schema"}, rows: [][]string{{"qa"}}}},
		{"iteration error after a row", testMissingChunkEr, dataReadMySQLReply{
			fields: []string{"table_schema", "table_name"}, rows: [][]string{{"qa", "wisps"}}, rowErr: testMissingChunkEr,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, port, _ := serveDataReadMySQL(t, map[string]dataReadMySQLReply{managedDoltIgnoredTableDiscoverySQL(): tc.reply})
			ops := managedDoltDataReadOpsDirect(t.Context(), host, port, "root")
			if _, err := ops.listTables(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("discovery error = %v, want %q", err, tc.want)
			}
		})
	}
}

// The fixture sends column metadata successfully, then a MySQL error packet
// where the first row belongs. QueryContext succeeds; only Rows.Next/Err sees
// the chunk signature. This proves the driver boundary without claiming to
// reproduce a physical missing chunk in a running Dolt server.
func TestManagedDoltDataReadDirectFetchesRowAndStopsOnError(t *testing.T) {
	t.Setenv("GC_DOLT_PASSWORD", "probe-secret")
	for _, tc := range []struct {
		name  string
		reply dataReadMySQLReply
	}{
		{"query error", dataReadMySQLReply{queryErr: testEmptyChunkErr}},
		{"first row error", dataReadMySQLReply{fields: []string{"id"}, rowErr: testEmptyChunkErr}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := "SELECT * FROM `qa`.`wisps` LIMIT 1"
			second := "SELECT * FROM `qa`.`events` LIMIT 1"
			host, port, queries := serveDataReadMySQL(t, map[string]dataReadMySQLReply{first: tc.reply})
			ops := managedDoltDataReadOpsDirect(t.Context(), host, port, "root")
			if err := ops.readBatch("qa", []string{first, second}); err == nil || !strings.Contains(err.Error(), testEmptyChunkErr) {
				t.Fatalf("read error = %v, want original chunk error", err)
			}
			if got := queries(); !reflect.DeepEqual(got, []string{first}) {
				t.Fatalf("queries after failure = %q, want only the failing read", got)
			}
		})
	}
}

func TestManagedDoltDataReadDirectCancelledBeforeQuery(t *testing.T) {
	t.Setenv("GC_DOLT_PASSWORD", "probe-secret")
	host, port, queries := serveDataReadMySQL(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ops := managedDoltDataReadOpsDirect(ctx, host, port, "root")
	if _, err := ops.listTables(); !errors.Is(err, context.Canceled) {
		t.Fatalf("discovery error = %v, want cancellation", err)
	}
	if err := ops.readBatch("qa", []string{"SELECT * FROM `qa`.`wisps` LIMIT 1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("read error = %v, want cancellation", err)
	}
	if got := queries(); len(got) != 0 {
		t.Fatalf("cancelled adapter issued queries: %q", got)
	}
}

type dataReadMySQLReply struct {
	fields           []string
	rows             [][]string
	queryErr, rowErr string
}

// serveDataReadMySQL speaks the small text-query subset the real pooled MySQL
// driver uses. It can place an error after column metadata, which a scripted
// managedDoltDataReadOps cannot prove. Every fixture has its own endpoint and
// closes its listener and accepted sockets, then joins its workers at cleanup.
func serveDataReadMySQL(t *testing.T, replies map[string]dataReadMySQLReply) (string, string, func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	var mu sync.Mutex
	var queries []string
	var conns []net.Conn
	var workers sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close() //nolint:errcheck
				deadline, _ := ctx.Deadline()
				if err := conn.SetDeadline(deadline); err != nil {
					t.Errorf("fixture deadline: %v", err)
					return
				}
				err := runDataReadMySQL(conn, func(query string) dataReadMySQLReply {
					mu.Lock()
					queries = append(queries, query)
					mu.Unlock()
					if reply, ok := replies[query]; ok {
						return reply
					}
					return dataReadMySQLReply{queryErr: "unexpected fixture query: " + query}
				})
				if err != nil && ctx.Err() == nil && !errors.Is(err, io.EOF) {
					t.Errorf("MySQL fixture: %v", err)
				}
			}()
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = ln.Close()
		<-done
		mu.Lock()
		for _, conn := range conns {
			_ = conn.Close()
		}
		mu.Unlock()
		workers.Wait()
	})
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return host, port, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), queries...)
	}
}

func runDataReadMySQL(conn net.Conn, replyFor func(string) dataReadMySQLReply) error {
	// Protocol 4.1, secure native-password auth, no TLS or multi-statements.
	const capabilities = uint32(1 | 4 | 512 | 32768 | 524288)
	hello := append([]byte{10}, []byte("8.0.36-probe-fixture\x00")...)
	hello = append(hello, 1, 0, 0, 0)
	hello = append(hello, []byte("12345678\x00")...)
	hello = binary.LittleEndian.AppendUint16(hello, uint16(capabilities&0xffff))
	hello = append(hello, 45, 2, 0)
	hello = binary.LittleEndian.AppendUint16(hello, uint16(capabilities>>16))
	hello = append(hello, 21)
	hello = append(hello, make([]byte, 10)...)
	hello = append(hello, []byte("abcdefghijkl\x00mysql_native_password\x00")...)
	if err := writeDataReadPacket(conn, 0, hello); err != nil {
		return err
	}
	if _, err := readDataReadPacket(conn); err != nil {
		return err
	}
	if err := writeDataReadPacket(conn, 2, []byte{0, 0, 0, 2, 0, 0, 0}); err != nil {
		return err
	}
	for {
		packet, err := readDataReadPacket(conn)
		if err != nil {
			return err
		}
		if len(packet) == 0 {
			return fmt.Errorf("empty command packet")
		}
		switch packet[0] {
		case 1: // COM_QUIT
			return nil
		case 14: // COM_PING
			err = writeDataReadPacket(conn, 1, []byte{0, 0, 0, 2, 0, 0, 0})
		case 3: // COM_QUERY
			err = writeDataReadReply(conn, replyFor(string(packet[1:])))
		default:
			return fmt.Errorf("unexpected command %d", packet[0])
		}
		if err != nil {
			return err
		}
	}
}

func writeDataReadReply(conn net.Conn, reply dataReadMySQLReply) error {
	seq := byte(1)
	write := func(data []byte) error {
		err := writeDataReadPacket(conn, seq, data)
		seq++
		return err
	}
	errorPacket := func(message string) []byte {
		return append([]byte{0xff, 0x51, 0x04, '#', 'H', 'Y', '0', '0', '0'}, []byte(message)...)
	}
	if reply.queryErr != "" {
		return write(errorPacket(reply.queryErr))
	}
	if err := write([]byte{byte(len(reply.fields))}); err != nil {
		return err
	}
	for _, name := range reply.fields {
		var field []byte
		for _, value := range []string{"def", "", "", "", name, ""} {
			field = append(field, byte(len(value)))
			field = append(field, value...)
		}
		// Fixed-length metadata: UTF-8 VARCHAR, no flags or decimals.
		field = append(field, 12, 33, 0, 0, 4, 0, 0, 0xfd, 0, 0, 0, 0, 0)
		if err := write(field); err != nil {
			return err
		}
	}
	eof := []byte{0xfe, 0, 0, 2, 0}
	if err := write(eof); err != nil {
		return err
	}
	for _, values := range reply.rows {
		var row []byte
		for _, value := range values {
			row = append(row, byte(len(value)))
			row = append(row, value...)
		}
		if err := write(row); err != nil {
			return err
		}
	}
	if reply.rowErr != "" {
		return write(errorPacket(reply.rowErr))
	}
	return write(eof)
}

func writeDataReadPacket(conn net.Conn, seq byte, data []byte) error {
	header := []byte{byte(len(data)), byte(len(data) >> 8), byte(len(data) >> 16), seq}
	_, err := io.Copy(conn, bytes.NewReader(append(header, data...)))
	return err
}

func readDataReadPacket(conn net.Conn) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	n := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if n > 1<<20 {
		return nil, fmt.Errorf("oversized fixture packet: %d bytes", n)
	}
	data := make([]byte, n)
	_, err := io.ReadFull(conn, data)
	return data, err
}
