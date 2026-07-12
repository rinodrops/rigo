package vault

import "fmt"

// ApplyResult reports what Apply did and what it left alone.
type ApplyResult struct {
	Linked    []string // entries newly linked (pending or unlinked before)
	Conflicts []string // listed only; resolve with rigo link
	Broken    []string // listed only; cleanup belongs to rigo clean
	Excluded  int      // entries not deployed on this host
}

// Apply converges every selected entry: pending and unlinked entries
// are linked (unlinked content is identical to the vault, so nothing
// is lost), conflicts and broken links are reported but never touched,
// and excluded entries are left entirely alone.
func Apply(entries []Entry, sel *Selection) (ApplyResult, error) {
	var res ApplyResult
	for _, e := range entries {
		if !sel.Selected(e) {
			res.Excluded++
			continue
		}
		state, err := Detect(e)
		if err != nil {
			return res, fmt.Errorf("%s: %w", e.Path, err)
		}
		switch state {
		case Pending, Unlinked:
			if err := Link(e); err != nil {
				return res, fmt.Errorf("%s: %w", e.Path, err)
			}
			res.Linked = append(res.Linked, e.Path)
		case Conflict:
			res.Conflicts = append(res.Conflicts, e.Path)
		case Broken:
			res.Broken = append(res.Broken, e.Path)
		case Linked:
			// already converged
		}
	}
	return res, nil
}
