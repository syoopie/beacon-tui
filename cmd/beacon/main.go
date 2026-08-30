package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/tmux"
	"github.com/syoopie/beacon-tui/internal/ui"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	configDir := flag.String("config-dir", "", "override the config directory")
	stateDir := flag.String("state-dir", "", "override the state directory")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildVersion())
		return
	}

	if err := run(*configDir, *stateDir); err != nil {
		fmt.Fprintln(os.Stderr, "beacon: "+err.Error())
		os.Exit(1)
	}
}

func run(configDir, stateDir string) error {
	dirs, err := config.DefaultDirs()
	if err != nil {
		return err
	}
	if configDir != "" {
		dirs.Config = configDir
	}
	if stateDir != "" {
		dirs.State = stateDir
	}

	if _, err := exec.LookPath("tmux"); err != nil {
		return errors.New("tmux is not on PATH; install it (brew install tmux, or apt-get install tmux)")
	}

	cfg, err := config.Load(dirs)
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			return fmt.Errorf("no config at %s: create it with scan_roots = [\"/absolute/path/to/your/servers\"]", dirs.ConfigFile())
		}
		return err
	}

	sup := &tmux.Client{}
	mgr := lifecycle.NewManager(sup, dirs, cfg.StopTimeout.Std())
	return ui.Run(ui.App{Dirs: dirs, Cfg: cfg, Sup: sup, Mgr: mgr})
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
