package reconcile

import (
	"fmt"
	"net"
	"time"

	"github.com/sunyupei/beacon-tui/internal/server"
)

// PortBlock is why a port cannot be used for a start.
type PortBlock struct {
	OSListener bool        // something outside beacon is already bound
	Specs      []server.ID // other beacon specs that claim the same port
}

func (b PortBlock) Blocked() bool { return b.OSListener || len(b.Specs) > 0 }

func (b PortBlock) String() string {
	switch {
	case b.OSListener && len(b.Specs) > 0:
		return fmt.Sprintf("port in use by another process and claimed by %v", b.Specs)
	case b.OSListener:
		return "port already in use by another process"
	case len(b.Specs) > 0:
		return fmt.Sprintf("port also claimed by %v", b.Specs)
	default:
		return "port available"
	}
}

// CheckPort reports whether port is free to bind and which other specs claim it.
// candidate is excluded from the spec comparison so a server does not collide
// with its own recorded port.
func CheckPort(port int, candidate server.ID, all []server.Spec) PortBlock {
	var claimants []server.ID
	for _, s := range all {
		if s.ID != candidate && s.Port == port {
			claimants = append(claimants, s.ID)
		}
	}
	return PortBlock{OSListener: osListening(port), Specs: claimants}
}

// osListening reports whether something already accepts TCP on the port locally.
// It dials rather than tries to bind: a bind probe is unreliable on BSD-derived
// stacks, where SO_REUSEADDR lets a 0.0.0.0 bind coexist with a 127.0.0.1 one,
// and a successful connect is also the health signal phase 10 wants. Servers
// beacon runs bind loopback or all interfaces, both reachable at 127.0.0.1.
func osListening(port int) bool {
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 300*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}
