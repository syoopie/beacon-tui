package main

import (
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

	fmt.Printf("config dir: %s\n", dirs.Config)
	fmt.Printf("state dir:  %s\n", dirs.State)
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
