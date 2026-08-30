// Package rcon asks a running Minecraft server who is online over its RCON port.
// It opens a fresh connection per poll and closes it straight away: beacon does
// not own the server, so it should not sit on a socket against it.
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

// Poll dials addr, authenticates with password, runs "list", and hangs up.
func Poll(addr, password string) (Snapshot, error) {
	conn, err := gorcon.Dial(addr, password,
		gorcon.SetDialTimeout(timeout), gorcon.SetDeadline(timeout))
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = conn.Close() }()

	out, err := conn.Execute("list")
	if err != nil {
		return Snapshot{}, err
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
