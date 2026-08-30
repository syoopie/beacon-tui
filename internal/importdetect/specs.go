package importdetect

import (
	"maps"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/server"
)

// BuildSpecs assigns collision-free IDs and fills the derived fields. taken is
// not mutated.
func BuildSpecs(dirs config.Dirs, cands []Candidate, taken map[server.ID]bool) []server.Spec {
	// maps.Clone(nil) returns a nil map, which the loop below cannot write to.
	owned := make(map[server.ID]bool, len(taken)+len(cands))
	maps.Copy(owned, taken)

	specs := make([]server.Spec, 0, len(cands))
	for _, c := range cands {
		id := server.NextFreeID(c.Base, owned)
		owned[id] = true
		specs = append(specs, server.Spec{
			ID: id, Dir: c.Dir, Start: c.Start, Script: c.Script, Port: c.Port,
			Session: server.SessionFor(id), LogFile: dirs.LogFile(id), Exec: c.Exec,
			RCON:  c.RCON,
			State: server.State{LastKnown: server.StatusStopped},
		})
	}
	return specs
}
