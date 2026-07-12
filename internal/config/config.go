// Package config loads rigo.toml and discovers the vault location.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Default names of the special vault directories, overridable in
// rigo.toml via os_dir / abs_dir / trash_dir.
const (
	DefaultOSDir    = ".os"
	DefaultAbsDir   = ".abs"
	DefaultTrashDir = ".trash"
)

// Secret is one [secrets] entry: a backend reference (op:// etc.)
// materialized at the entry's home-relative path.
type Secret struct {
	Ref  string
	Mode fs.FileMode
	OS   []string
}

// Config is the typed content of rigo.toml. Paths are kept as written
// (including any trailing slash); normalization against the vault tree
// happens at scan time.
type Config struct {
	Dirs     []string
	Ignore   []string
	Distros  []string
	OSDir    string
	AbsDir   string
	TrashDir string
	Tags     map[string][]string
	Groups   map[string][]string
	Include  map[string][]string
	Exclude  map[string][]string
	Secrets  map[string]Secret
}

// raw mirrors the TOML document. Secrets values are decoded loosely
// because an entry is either a ref string or an inline table.
type raw struct {
	Dirs     []string            `toml:"dirs"`
	Ignore   []string            `toml:"ignore"`
	Distros  []string            `toml:"distros"`
	OSDir    string              `toml:"os_dir"`
	AbsDir   string              `toml:"abs_dir"`
	TrashDir string              `toml:"trash_dir"`
	Tags     map[string][]string `toml:"tags"`
	Groups   map[string][]string `toml:"groups"`
	Include  map[string][]string `toml:"include"`
	Exclude  map[string][]string `toml:"exclude"`
	Secrets  map[string]any      `toml:"secrets"`
}

// Load reads and validates the rigo.toml at path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := toml.NewDecoder(f)
	dec.DisallowUnknownFields()
	var r raw
	if err := dec.Decode(&r); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return nil, fmt.Errorf("%s: unknown keys:\n%s", path, strict.String())
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	c := &Config{
		Dirs:     r.Dirs,
		Ignore:   r.Ignore,
		Distros:  r.Distros,
		OSDir:    r.OSDir,
		AbsDir:   r.AbsDir,
		TrashDir: r.TrashDir,
		Tags:     r.Tags,
		Groups:   r.Groups,
		Include:  r.Include,
		Exclude:  r.Exclude,
	}
	if c.OSDir == "" {
		c.OSDir = DefaultOSDir
	}
	if c.AbsDir == "" {
		c.AbsDir = DefaultAbsDir
	}
	if c.TrashDir == "" {
		c.TrashDir = DefaultTrashDir
	}

	if c.Secrets, err = parse_secrets(r.Secrets); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func (c *Config) validate() error {
	for _, name := range []struct{ key, val string }{
		{"os_dir", c.OSDir}, {"abs_dir", c.AbsDir}, {"trash_dir", c.TrashDir},
	} {
		if strings.ContainsAny(name.val, `/\`) || name.val == "." || name.val == ".." {
			return fmt.Errorf("%s: %q must be a plain directory name", name.key, name.val)
		}
	}

	if err := check_paths("dirs", c.Dirs); err != nil {
		return err
	}
	// Distro overlay names follow os-release IDs: lowercase,
	// machine-readable, and never dot-prefixed.
	for _, d := range c.Distros {
		if d == "" || strings.HasPrefix(d, ".") || strings.ContainsAny(d, `/\`) || d != strings.ToLower(d) {
			return fmt.Errorf("distros: %q must be a lowercase os-release ID (no dots at the start, no path separators)", d)
		}
	}
	for tag, paths := range c.Tags {
		if tag == "" {
			return errors.New("tags: empty tag name")
		}
		if err := check_paths(fmt.Sprintf("tags.%s", tag), paths); err != nil {
			return err
		}
	}

	// Group members and include/exclude keys identify hosts (or groups)
	// and share one namespace: lowercase, no dots, so that runtime
	// hostname matching never depends on case folding.
	hosts := map[string]string{}
	for group, members := range c.Groups {
		if err := check_name("groups", group); err != nil {
			return err
		}
		for _, m := range members {
			if err := check_name(fmt.Sprintf("groups.%s", group), m); err != nil {
				return err
			}
			hosts[m] = group
		}
	}
	for group := range c.Groups {
		if _, ok := hosts[group]; ok {
			return fmt.Errorf("groups: %q is both a group name and a host name", group)
		}
	}
	for _, section := range []struct {
		key     string
		entries map[string][]string
	}{{"include", c.Include}, {"exclude", c.Exclude}} {
		for name := range section.entries {
			if err := check_name(section.key, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// check_paths validates home-relative logical paths (dirs, tag members).
func check_paths(key string, paths []string) error {
	for _, p := range paths {
		switch {
		case p == "":
			return fmt.Errorf("%s: empty path", key)
		case strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~"):
			return fmt.Errorf("%s: %q must be relative to the home directory", key, p)
		case p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/.."):
			return fmt.Errorf("%s: %q must not contain \"..\"", key, p)
		}
	}
	return nil
}

// check_name validates host and group identifiers.
func check_name(key, name string) error {
	if name == "" {
		return fmt.Errorf("%s: empty name", key)
	}
	if strings.Contains(name, ".") || name != strings.ToLower(name) {
		return fmt.Errorf("%s: %q must be lowercase and contain no dots", key, name)
	}
	return nil
}

func parse_secrets(entries map[string]any) (map[string]Secret, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	secrets := make(map[string]Secret, len(entries))
	for path, value := range entries {
		s := Secret{Mode: 0o600}
		switch v := value.(type) {
		case string:
			s.Ref = v
		case map[string]any:
			for key, field := range v {
				switch key {
				case "ref":
					ref, ok := field.(string)
					if !ok {
						return nil, fmt.Errorf("secrets.%q: ref must be a string", path)
					}
					s.Ref = ref
				case "mode":
					mode, ok := field.(int64)
					if !ok || mode < 0 || mode > 0o777 {
						return nil, fmt.Errorf("secrets.%q: mode must be a permission like 0o600", path)
					}
					s.Mode = fs.FileMode(mode)
				case "os":
					list, ok := field.([]any)
					if !ok {
						return nil, fmt.Errorf("secrets.%q: os must be an array of strings", path)
					}
					for _, item := range list {
						goos, ok := item.(string)
						if !ok || (goos != "darwin" && goos != "linux" && goos != "windows") {
							return nil, fmt.Errorf("secrets.%q: os entries must be \"darwin\", \"linux\", or \"windows\"", path)
						}
						s.OS = append(s.OS, goos)
					}
				default:
					return nil, fmt.Errorf("secrets.%q: unknown key %q", path, key)
				}
			}
		default:
			return nil, fmt.Errorf("secrets.%q: value must be a ref string or a table", path)
		}
		if !strings.Contains(s.Ref, "://") {
			return nil, fmt.Errorf("secrets.%q: ref %q has no backend scheme (expected e.g. op://...)", path, s.Ref)
		}
		if err := check_paths("secrets", []string{path}); err != nil {
			return nil, err
		}
		secrets[path] = s
	}
	return secrets, nil
}
