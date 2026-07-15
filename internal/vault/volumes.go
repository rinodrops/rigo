package vault

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rinodrops/rigo/internal/config"
)

// resolve_volumes maps volume names to drive letters for this host:
// the host's own entry wins over its groups' entries (which must not
// conflict), which win over the default letter. The built-in volume
// "system" resolves to the system drive unless overridden. Volumes
// with no resolution for this host are absent from the result.
func resolve_volumes(cfg *config.Config, host Host) (map[string]string, error) {
	resolved := map[string]string{"system": system_letter(host)}
	for name, vol := range cfg.Volumes {
		letter := vol.Default
		if own, ok := vol.Hosts[host.Name]; ok {
			letter = own
		} else if from_groups := group_letters(cfg, vol, host.Name); len(from_groups) > 0 {
			if len(from_groups) > 1 {
				return nil, fmt.Errorf("volumes.%s: groups of host %s map it to different letters (%s)",
					name, host.Name, strings.Join(from_groups, ", "))
			}
			letter = from_groups[0]
		}
		if letter != "" {
			resolved[name] = letter
		}
	}
	return resolved, nil
}

// group_letters collects the distinct letters that the host's groups
// assign to the volume.
func group_letters(cfg *config.Config, vol config.Volume, host string) []string {
	seen := map[string]bool{}
	for group, members := range cfg.Groups {
		letter, ok := vol.Hosts[group]
		if !ok {
			continue
		}
		for _, m := range members {
			if m == host {
				seen[letter] = true
			}
		}
	}
	letters := make([]string, 0, len(seen))
	for l := range seen {
		letters = append(letters, l)
	}
	sort.Strings(letters)
	return letters
}

func system_letter(host Host) string {
	if host.SysDrive != "" {
		return strings.ToLower(host.SysDrive[:1])
	}
	return "c"
}
