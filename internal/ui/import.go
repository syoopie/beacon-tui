package ui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/server"
)

type patchPrompt struct {
	id    server.ID
	patch importdetect.Patch
}

// importCmd scans the configured roots and writes a spec for every server
// directory that is not already in the registry, keyed by directory so a
// re-scan does not duplicate anything.
func (m *model) importCmd() tea.Cmd {
	dirs, roots, known := m.app.Dirs, m.app.Cfg.ScanRoots, m.specs
	return func() tea.Msg {
		cands, err := importdetect.Scan(roots)
		if err != nil {
			return opDoneMsg{label: "import", err: err}
		}

		takenID := make(map[server.ID]bool, len(known))
		takenDir := make(map[string]bool, len(known))
		for _, s := range known {
			takenID[s.ID] = true
			takenDir[s.Dir] = true
		}

		fresh := cands[:0]
		for _, c := range cands {
			if !takenDir[c.Dir] {
				fresh = append(fresh, c)
			}
		}
		if len(fresh) == 0 {
			return opDoneMsg{label: "import: nothing new under the scan roots"}
		}

		specs := importdetect.BuildSpecs(dirs, fresh, takenID)
		for _, s := range specs {
			if err := config.SaveSpec(dirs, s); err != nil {
				return opDoneMsg{label: "import", err: err}
			}
		}
		needPatch := 0
		for _, s := range specs {
			if !s.Exec.Launchable() {
				needPatch++
			}
		}
		label := fmt.Sprintf("imported %d server(s)", len(specs))
		if needPatch > 0 {
			label += fmt.Sprintf("; %d need `exec` patching (select and press p)", needPatch)
		}
		return opDoneMsg{label: label}
	}
}

func (m *model) planPatchCmd(spec server.Spec) tea.Cmd {
	return func() tea.Msg {
		if spec.Script == "" {
			return patchPlannedMsg{id: spec.ID, err: fmt.Errorf("%s launches a jar directly; nothing to patch", spec.ID)}
		}
		path := filepath.Join(spec.Dir, spec.Script)
		patch, needed, err := importdetect.PlanPatch(path)
		return patchPlannedMsg{id: spec.ID, patch: patch, needed: needed, err: err}
	}
}

func (m *model) applyPatchCmd(patch importdetect.Patch) tea.Cmd {
	mgr, specs := m.app.Mgr, m.specs
	return func() tea.Msg {
		for _, s := range specs {
			if s.Script == "" || filepath.Join(s.Dir, s.Script) != patch.Path {
				continue
			}
			patched, err := mgr.ApplyScriptPatch(s, patch)
			if err != nil {
				return opDoneMsg{id: s.ID, label: "patch", err: err}
			}
			return opDoneMsg{id: s.ID, label: string(s.ID) + " patched (" + patched.Exec.String() + ")"}
		}
		return opDoneMsg{label: "patch: no server uses that script"}
	}
}
