package rcon

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// fakeListServer answers auth then "list" forever on every connection it
// accepts, and counts how many connections it has accepted.
func fakeListServer(t *testing.T, reply string) (addr string, connects *int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var count int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&count, 1)
			go serveOneConn(conn, reply)
		}
	}()
	return ln.Addr().String(), &count
}

func serveOneConn(conn net.Conn, reply string) {
	defer func() { _ = conn.Close() }()
	for {
		id, typ, body, err := readPacket(conn)
		if err != nil {
			return
		}
		switch typ {
		case rconTypeAuth:
			if body != "secret" {
				_ = writeRaw(conn, -1, rconTypeCommand, "")
				return
			}
			_ = writeRaw(conn, id, rconTypeCommand, "")
		case rconTypeCommand:
			// gorcon's Execute uses SERVERDATA_EXECCOMMAND_ID (0) for every
			// command and checks the reply mirrors it, so echo id rather than
			// help.go's own rconCmdID (a constant meaningful only to Help's
			// hand-rolled protocol client, not gorcon's).
			_ = writeRaw(conn, id, rconTypeResponse, reply)
		}
	}
}

func TestClientPollReusesConnection(t *testing.T) {
	addr, connects := fakeListServer(t, "There are 1 of a max of 20 players online: Steve")

	c, err := Dial(addr, "secret")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	for i := 0; i < 5; i++ {
		snap, err := c.Poll()
		if err != nil {
			t.Fatalf("Poll #%d: %v", i, err)
		}
		if snap.Online != 1 || snap.Max != 20 {
			t.Fatalf("Poll #%d: got %+v", i, snap)
		}
	}

	if got := atomic.LoadInt32(connects); got != 1 {
		t.Fatalf("expected exactly one connection across 5 polls, got %d", got)
	}
}

func TestClientPollReconnectsAfterConnectionBreaks(t *testing.T) {
	addr, connects := fakeListServer(t, "There are 0 of a max of 20 players online:")

	c, err := Dial(addr, "secret")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Sever the connection out from under the client without telling it.
	_ = c.conn.Close()

	if _, err := c.Poll(); err != nil {
		t.Fatalf("Poll after the connection broke: %v", err)
	}
	if got := atomic.LoadInt32(connects); got != 2 {
		t.Fatalf("expected a redial after the break, got %d connections", got)
	}
}

func TestDialGivesUpAfterMaxReconnectTries(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing is listening: every dial attempt fails fast

	start := time.Now()
	_, err = Dial(addr, "secret")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected Dial to fail against a closed port")
	}
	// 3 tries means 2 backoff sleeps (500ms, 1s) between them.
	if elapsed < 1400*time.Millisecond {
		t.Fatalf("Dial gave up too fast for 3 tries with backoff: %v", elapsed)
	}
}
