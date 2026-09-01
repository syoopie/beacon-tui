package rcon

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// The Source RCON wire protocol. A packet is a little-endian int32 length
// (counting everything after itself), then int32 id, int32 type, the body as a
// null-terminated ASCII string, and one more null byte.
const (
	rconTypeResponse = 0 // SERVERDATA_RESPONSE_VALUE
	rconTypeCommand  = 2 // SERVERDATA_EXECCOMMAND (and AUTH_RESPONSE on the way back)
	rconTypeAuth     = 3 // SERVERDATA_AUTH

	rconAuthID = 1
	rconCmdID  = 2

	// maxBody is the largest body Minecraft puts in one packet. A response
	// longer than this is split, so a body shorter than this is the last one.
	maxBody = 4096
)

// helpReadWait bounds the wait for each further packet of a split response.
// Minecraft does not mark the final packet; it just stops sending, so a read
// that times out after we already have content is the normal end of the list.
const helpReadWait = 1500 * time.Millisecond

// errAuthFailed is a wrong RCON password.
var errAuthFailed = errors.New("rcon: authentication failed")

// Help runs "help" over RCON against a running server and returns its whole
// command list, modded commands included.
//
// gorcon's Execute reads only the first response packet, and a real modpack's
// "/help" spans several, so Help speaks the (tiny, frozen since 2004) protocol
// itself to reassemble them. Like [Poll] it opens a fresh connection and hangs
// up straight away: beacon does not own the server.
func Help(addr, password string) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * timeout))

	if err := writePacket(conn, rconAuthID, rconTypeAuth, password); err != nil {
		return "", err
	}
	id, _, _, err := readPacket(conn)
	if err != nil {
		return "", fmt.Errorf("rcon: help auth: %w", err)
	}
	if id == -1 {
		return "", errAuthFailed
	}

	if err := writePacket(conn, rconCmdID, rconTypeCommand, "help"); err != nil {
		return "", err
	}

	var out strings.Builder
	for {
		_ = conn.SetReadDeadline(time.Now().Add(helpReadWait))
		_, _, body, err := readPacket(conn)
		if err != nil {
			// Minecraft does not frame the last packet of a split response; it
			// just stops. Once we have content, a read that times out or hits
			// EOF is that stop, not a failure.
			var ne net.Error
			timedOut := errors.As(err, &ne) && ne.Timeout()
			if out.Len() > 0 && (timedOut || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
				break
			}
			return "", fmt.Errorf("rcon: reading help: %w", err)
		}
		out.WriteString(body)
		if len(body) < maxBody {
			break
		}
	}
	return strings.TrimSpace(stripCodes(out.String())), nil
}

func writePacket(w io.Writer, id, typ int32, body string) error {
	payload := make([]byte, 8+len(body)+2)
	binary.LittleEndian.PutUint32(payload[0:4], uint32(id))
	binary.LittleEndian.PutUint32(payload[4:8], uint32(typ))
	copy(payload[8:], body)
	// last two bytes stay zero: the body's null terminator and the trailing null

	frame := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(len(payload)))
	copy(frame[4:], payload)

	_, err := w.Write(frame)
	return err
}

func readPacket(r io.Reader) (id, typ int32, body string, err error) {
	var sizeBuf [4]byte
	if _, err = io.ReadFull(r, sizeBuf[:]); err != nil {
		return 0, 0, "", err
	}
	size := int(binary.LittleEndian.Uint32(sizeBuf[:]))
	if size < 10 || size > maxBody+64 {
		return 0, 0, "", fmt.Errorf("rcon: packet size %d out of range", size)
	}
	buf := make([]byte, size)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, 0, "", err
	}
	id = int32(binary.LittleEndian.Uint32(buf[0:4]))
	typ = int32(binary.LittleEndian.Uint32(buf[4:8]))
	return id, typ, string(buf[8 : size-2]), nil
}
