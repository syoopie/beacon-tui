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

// ErrNoConfig reports that config.toml does not exist yet. First run is a clear
// error, never a $HOME crawl.
var ErrNoConfig = errors.New("no config file")

const defaultStopTimeout = Duration(60 * time.Second)

func Load(dirs Dirs) (Config, error) {
	path := dirs.ConfigFile()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("%s: %w", path, ErrNoConfig)
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var c Config
	md, err := toml.Decode(string(data), &c)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}

	if len(c.ScanRoots) == 0 {
		return Config{}, fmt.Errorf("%s: scan_roots is required; beacon does not crawl $HOME", path)
	}
	for _, root := range c.ScanRoots {
		if !filepath.IsAbs(root) {
			return Config{}, fmt.Errorf("%s: scan root %q is not an absolute path", path, root)
		}
	}

	if !md.IsDefined("stop_timeout") {
		c.StopTimeout = defaultStopTimeout
	} else if c.StopTimeout <= 0 {
		return Config{}, fmt.Errorf("%s: stop_timeout %s must be positive", path, c.StopTimeout.Std())
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
