package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration is a time.Duration that round-trips through TOML as "60s".
type Duration time.Duration

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

func (d *Duration) UnmarshalText(b []byte) error {
	parsed, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("duration %q: %w", b, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

// Config is config.toml, parsed once at the boundary.
type Config struct {
	ScanRoots   []string `toml:"scan_roots"`
	StopTimeout Duration `toml:"stop_timeout"`
}

// ErrNoConfig reports that config.toml does not exist yet. The returned Config
// still carries defaults, so a caller that is fine starting empty can use it and
// ignore the sentinel. beacon never falls back to crawling $HOME: with no scan
// roots it simply finds nothing until the operator adds one.
var ErrNoConfig = errors.New("no config file")

const defaultStopTimeout = Duration(60 * time.Second)

// Default is the configuration beacon runs with before config.toml exists.
func Default() Config { return Config{StopTimeout: defaultStopTimeout} }

func Load(dirs Dirs) (Config, error) {
	path := dirs.ConfigFile()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), fmt.Errorf("%s: %w", path, ErrNoConfig)
	}
	if err != nil {
		return Default(), fmt.Errorf("reading %s: %w", path, err)
	}

	var c Config
	md, err := toml.Decode(string(data), &c)
	if err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Default(), fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}

	for _, root := range c.ScanRoots {
		if !filepath.IsAbs(root) {
			return Default(), fmt.Errorf("%s: scan root %q is not an absolute path", path, root)
		}
	}

	if !md.IsDefined("stop_timeout") {
		c.StopTimeout = defaultStopTimeout
	} else if c.StopTimeout <= 0 {
		return Default(), fmt.Errorf("%s: stop_timeout %s must be positive", path, c.StopTimeout.Std())
	}

	return c, nil
}

// AddScanRoot resolves dir to an absolute path and adds it to config.toml,
// creating the file if it does not exist. It is idempotent: adding a root that
// is already listed is a no-op that still returns the current config.
func AddScanRoot(dirs Dirs, dir string) (Config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Config{}, fmt.Errorf("resolving %q: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Config{}, fmt.Errorf("scan root %s: %w", abs, err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("scan root %s: not a directory", abs)
	}

	c, err := Load(dirs)
	if err != nil && !errors.Is(err, ErrNoConfig) {
		return Config{}, err
	}
	for _, root := range c.ScanRoots {
		if root == abs {
			return c, nil
		}
	}
	c.ScanRoots = append(c.ScanRoots, abs)
	if err := Save(dirs, c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func Save(dirs Dirs, c Config) error {
	path := dirs.ConfigFile()

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return WriteFileAtomic(path, buf.Bytes(), 0o644)
}
