package server

import "fmt"

// ExecState records whether a server's start script hands its PID to Java.
// A script whose last effective command is not `exec java ...` leaves the JVM as
// a grandchild of the tmux pane, so beacon cannot signal or measure it, and
// launching such a spec must be refused later.
type ExecState uint8

const (
	ExecUnknown ExecState = iota // never inspected
	ExecOK                       // last effective command is `exec ... java ...`
	ExecMissing                  // inspected, and it is not
)

var execStateNames = [...]string{
	ExecUnknown: "unknown",
	ExecOK:      "ok",
	ExecMissing: "missing",
}

func ParseExecState(s string) (ExecState, error) {
	for i, name := range execStateNames {
		if name == s {
			return ExecState(i), nil
		}
	}
	return 0, fmt.Errorf("exec_state %q: not one of %v", s, execStateNames)
}

func (e ExecState) String() string {
	if int(e) >= len(execStateNames) {
		return fmt.Sprintf("exec_state(%d)", uint8(e))
	}
	return execStateNames[e]
}

func (e ExecState) MarshalText() ([]byte, error) {
	if int(e) >= len(execStateNames) {
		return nil, fmt.Errorf("exec_state(%d): out of range", uint8(e))
	}
	return []byte(execStateNames[e]), nil
}

func (e *ExecState) UnmarshalText(b []byte) error {
	parsed, err := ParseExecState(string(b))
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}

// Launchable reports whether a spec in this state may be started.
func (e ExecState) Launchable() bool { return e == ExecOK }
