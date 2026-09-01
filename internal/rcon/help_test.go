package rcon

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
)

// fakeRCON answers one auth and one command, splitting the reply into 4096-byte
// packets the way Minecraft does.
func fakeRCON(t *testing.T, reply string, goodPassword string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		id, _, body, err := readPacket(conn)
		if err != nil {
			return
		}
		if body != goodPassword {
			_ = writeRaw(conn, -1, rconTypeCommand, "")
			return
		}
		_ = writeRaw(conn, id, rconTypeCommand, "") // auth ok

		if _, _, _, err := readPacket(conn); err != nil { // the "help" command
			return
		}
		for i := 0; i < len(reply); i += maxBody {
			end := i + maxBody
			if end > len(reply) {
				end = len(reply)
			}
			_ = writeRaw(conn, rconCmdID, rconTypeResponse, reply[i:end])
		}
	}()

	return ln.Addr().String()
}

// writeRaw is writePacket without the client-side id/type conventions.
func writeRaw(w io.Writer, id, typ int32, body string) error {
	payload := make([]byte, 8+len(body)+2)
	binary.LittleEndian.PutUint32(payload[0:4], uint32(id))
	binary.LittleEndian.PutUint32(payload[4:8], uint32(typ))
	copy(payload[8:], body)
	frame := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(payload)))
	copy(frame[4:], payload)
	_, err := w.Write(frame)
	return err
}

func TestHelpReassemblesSplitResponse(t *testing.T) {
	want := "/advancement (grant|revoke)\n" + strings.Repeat("/x"+strings.Repeat("y", 40)+"\n", 300)
	addr := fakeRCON(t, want, "secret")

	got, err := Help(addr, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSpace(want) {
		t.Fatalf("Help lost data across the packet split: got %d bytes, want %d", len(got), len(strings.TrimSpace(want)))
	}
}

func TestHelpRejectsBadPassword(t *testing.T) {
	addr := fakeRCON(t, "/help\n", "secret")
	if _, err := Help(addr, "wrong"); err == nil {
		t.Fatal("Help accepted a bad password")
	}
}
