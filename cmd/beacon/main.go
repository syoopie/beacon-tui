package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"

	"golang.org/x/mod/semver"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/selfupdate"
	"github.com/syoopie/beacon-tui/internal/tmux"
	"github.com/syoopie/beacon-tui/internal/ui"
)

// version is set by the release build via -ldflags -X main.version.
var version = "dev"

// repoSlug is where beacon checks for a newer release.
const repoSlug = "syoopie/beacon-tui"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	configDir := flag.String("config-dir", "", "override the config directory")
	stateDir := flag.String("state-dir", "", "override the state directory")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildVersion())
		return
	}

	cmd := run
	if flag.NArg() == 1 && flag.Arg(0) == "update" {
		cmd = func(string, string) error { return update() }
	}
	if err := cmd(*configDir, *stateDir); err != nil {
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
	if err != nil && !errors.Is(err, config.ErrNoConfig) {
		return err
	}

	// `beacon /path/to/servers` seeds a scan root without touching config.toml
	// by hand. Without an argument beacon starts empty and the operator adds a
	// folder from inside the TUI.
	if root := flag.Arg(0); root != "" {
		cfg, err = config.AddScanRoot(dirs, root)
		if err != nil {
			return err
		}
	}

	sup := &tmux.Client{}
	mgr := lifecycle.NewManager(sup, dirs, cfg.StopTimeout.Std())
	return ui.Run(ui.App{
		Dirs:    dirs,
		Cfg:     cfg,
		Sup:     sup,
		Mgr:     mgr,
		Version: buildVersion(),
		Repo:    repoSlug,

		ScanOnStart: flag.Arg(0) != "",
	})
}

// update replaces this binary with the newest release. Servers keep running:
// tmux owns them, not this process.
func update() error {
	current := buildVersion()
	if !semver.IsValid(current) {
		return fmt.Errorf("this is a development build (%s), so there is no release to update from; install one with:\n  %s", current, selfupdate.InstallCommand(repoSlug))
	}

	ctx := context.Background()
	res, err := selfupdate.Check(ctx, repoSlug, current)
	if err != nil {
		return fmt.Errorf("checking for a new release: %w", err)
	}
	if !res.Available {
		fmt.Printf("beacon %s is the newest version.\n", current)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}

	fmt.Printf("Updating beacon %s to %s...\n", current, res.Latest)
	if err := selfupdate.Apply(ctx, repoSlug, res.Latest, exe); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("cannot write %s; run: sudo beacon update", exe)
		}
		return err
	}
	fmt.Printf("Updated %s to %s. Running servers were not touched; restart beacon to use the new version.\n", exe, res.Latest)
	return nil
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
