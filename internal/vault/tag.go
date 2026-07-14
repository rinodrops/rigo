package vault

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rinodrops/rigo/internal/config"
)

// Members resolves a tag's declared paths to scanned entries, in
// declaration order. Declared paths without a vault entry are returned
// separately (scanning already warns about them).
func Members(cfg *config.Config, entries []Entry, name string) ([]Entry, []string, error) {
	paths, ok := cfg.Tags[name]
	if !ok {
		known := make([]string, 0, len(cfg.Tags))
		for tag := range cfg.Tags {
			known = append(known, tag)
		}
		sort.Strings(known)
		if len(known) == 0 {
			return nil, nil, fmt.Errorf("tag %q is not defined (no tags declared in rigo.toml)", name)
		}
		return nil, nil, fmt.Errorf("tag %q is not defined (known tags: %s)", name, strings.Join(known, ", "))
	}

	var members []Entry
	var missing []string
	for _, p := range paths {
		if e, ok := Find(entries, p); ok {
			members = append(members, e)
		} else {
			missing = append(missing, p)
		}
	}
	return members, missing, nil
}
