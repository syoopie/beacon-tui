package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/sunyupei/beacon-tui/internal/config"
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

	if _, err := config.Load(dirs); err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			return fmt.Errorf("no config at %s: create it with scan_roots = [\"/absolute/path/to/your/servers\"]", dirs.ConfigFile())
		}
		return err
	}

	specs, err := config.LoadSpecs(dirs)
	if err != nil {
		return err
	}

	fmt.Printf("config dir: %s\n", dirs.Config)
	fmt.Printf("state dir:  %s\n", dirs.State)
	fmt.Printf("specs:      %d\n", len(specs))
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
