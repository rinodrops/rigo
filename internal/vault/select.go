package vault

import (
	"strings"

	"github.com/rinodrops/rigo/internal/config"
)

// Mode is the selection mode a host operates in.
type Mode int

const (
	ModeExclude Mode = iota // blacklist: everything minus effective exclude
	ModeInclude             // whitelist: effective include minus effective exclude
)

func (m Mode) String() string {
	if m == ModeInclude {
		return "include"
	}
	return "exclude"
}

// Selection is the per-machine deployment filter derived from
// [groups] / [include] / [exclude] and the runtime host name.
type Selection struct {
	Mode    Mode
	cfg     *config.Config
	include map[string]bool
	exclude map[string]bool
}

// Select computes the effective selection for host: the host's own
// include/exclude entries united with those of every group the host
// belongs to.
func Select(cfg *config.Config, host string) *Selection {
	groups := []string{}
	for group, members := range cfg.Groups {
		for _, m := range members {
			if m == host {
				groups = append(groups, group)
			}
		}
	}
	collect := func(section map[string][]string) map[string]bool {
		eff := map[string]bool{}
		for _, name := range append([]string{host}, groups...) {
			for _, item := range section[name] {
				eff[item] = true
			}
		}
		return eff
	}
	s := &Selection{
		cfg:     cfg,
		include: collect(cfg.Include),
		exclude: collect(cfg.Exclude),
	}
	if len(s.include) > 0 {
		s.Mode = ModeInclude
	}
	return s
}

// Selected reports whether the entry deploys on this host.
func (s *Selection) Selected(e Entry) bool {
	if s.matches(s.exclude, e) {
		return false
	}
	if s.Mode == ModeInclude {
		return s.matches(s.include, e)
	}
	return true
}

// matches checks an effective include/exclude set: each item is a tag
// name or a path. A path item matches the entry itself or, for
// container directories, anything beneath it.
func (s *Selection) matches(set map[string]bool, e Entry) bool {
	for item := range set {
		if paths, ok := s.cfg.Tags[item]; ok {
			for _, p := range paths {
				if path_covers(norm(p), e.Path) {
					return true
				}
			}
			continue
		}
		if path_covers(norm(item), e.Path) {
			return true
		}
	}
	return false
}

func path_covers(p, entry string) bool {
	return entry == p || strings.HasPrefix(entry, p+"/")
}
