// Package rcon asks a running Minecraft server who is online over its RCON port.
package rcon

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	gorcon "github.com/gorcon/rcon"
)

// Snapshot is one poll of a server's player list.
type Snapshot struct {
	Online  int
	Max     int
	Players []string
}

const timeout = 3 * time.Second

// maxReconnectTries caps how many times Poll redials a broken connection
// before giving up and returning the failure.
const maxReconnectTries = 3

// reconnectBackoff is the delay before the first redial; it doubles after
// each failed try.
const reconnectBackoff = 500 * time.Millisecond

// Client holds one RCON connection to a server, reused across polls so a live
// console does not open and close a connection, and so log two lines on the
// server, every cycle. A broken connection is redialed with exponential
// backoff inside Poll, invisibly to the caller unless every try fails.
type Client struct {
	conn     *gorcon.Conn
	addr     string
	password string
}

// Dial opens the connection a Client polls over, retrying with exponential
// backoff up to maxReconnectTries: a server that has just started often has
// not opened its RCON port yet.
func Dial(addr, password string) (*Client, error) {
	conn, err := dialWithBackoff(addr, password)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, addr: addr, password: password}, nil
}

func dialOnce(addr, password string) (*gorcon.Conn, error) {
	return gorcon.Dial(addr, password,
		gorcon.SetDialTimeout(timeout), gorcon.SetDeadline(timeout))
}

// dialWithBackoff tries dialOnce up to maxReconnectTries, sleeping
// reconnectBackoff (doubling each time) between attempts.
func dialWithBackoff(addr, password string) (*gorcon.Conn, error) {
	backoff := reconnectBackoff
	var lastErr error
	for i := 0; i < maxReconnectTries; i++ {
		if i > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}
		conn, err := dialOnce(addr, password)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("rcon: dial: %w", lastErr)
}

// Close releases the held connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Poll runs "list" over the held connection. If the connection has gone bad
// it is redialed with exponential backoff, up to maxReconnectTries, before
// Poll gives up and returns the last error.
func (c *Client) Poll() (Snapshot, error) {
	out, err := c.conn.Execute("list")
	if err != nil {
		_ = c.conn.Close()
		conn, dialErr := dialWithBackoff(c.addr, c.password)
		if dialErr != nil {
			return Snapshot{}, dialErr
		}
		c.conn = conn
		out, err = c.conn.Execute("list")
		if err != nil {
			return Snapshot{}, err
		}
	}
	return parseList(out)
}

// listRE matches both "There are 2 of a max of 20 players online: a, b" and the
// older "There are 2/20 players online: a, b". The trailing group is the roster,
// which is empty when nobody is on.
var listRE = regexp.MustCompile(`There are (\d+)(?:/| of a max of )(\d+) players online:?\s*(.*)`)

func parseList(out string) (Snapshot, error) {
	out = strings.TrimSpace(stripCodes(out))
	m := listRE.FindStringSubmatch(out)
	if m == nil {
		return Snapshot{}, fmt.Errorf("rcon: could not read the player list from %q", out)
	}
	online, _ := strconv.Atoi(m[1])
	maxPlayers, _ := strconv.Atoi(m[2])

	var players []string
	for _, name := range strings.Split(m[3], ",") {
		if name = strings.TrimSpace(name); name != "" {
			players = append(players, name)
		}
	}
	return Snapshot{Online: online, Max: maxPlayers, Players: players}, nil
}

// stripCodes drops Minecraft section-sign colour codes, which some servers leave
// in RCON output.
func stripCodes(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '§' && i+1 < len(runes) {
			i++
			continue
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}
