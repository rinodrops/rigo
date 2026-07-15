package vault

import "fmt"

// ApplyResult reports what Apply did and what it left alone.
type ApplyResult struct {
	Linked    []string // entries newly linked (pending or unlinked before)
	Conflicts []string // listed only; resolve with rigo link
	Broken    []string // listed only; cleanup belongs to rigo clean
	Failed    []string // "path: error" for entries that could not converge
	Excluded  int      // entries not deployed on this host
}

// Apply converges every selected entry: pending and unlinked entries
// are linked (unlinked content is identical to the vault, so nothing
// is lost), conflicts and broken links are reported but never touched,
// and excluded entries are left entirely alone. A failing entry (e.g.
// its drive does not exist) is recorded and the rest still converge.
func Apply(entries []Entry, sel *Selection) ApplyResult {
	var res ApplyResult
	for _, e := range entries {
		if !sel.Selected(e) {
			res.Excluded++
			continue
		}
		state, err := Detect(e)
		if err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", e.Path, err))
			continue
		}
		switch state {
		case Pending, Unlinked:
			if err := Link(e); err != nil {
				res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", e.Path, err))
				continue
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
	return res
}
