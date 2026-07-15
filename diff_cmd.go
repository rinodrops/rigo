package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rinodrops/rigo/internal/vault"
)

// err_differs signals a diff(1)-style exit code 1 without an error
// message; main treats it as "differences were shown".
var err_differs = errors.New("differences found")

func diff_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [<path>]",
		Short: "Show differences between local content and the vault (read-only)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if len(args) == 1 {
				e, ok := vault.ResolveArg(s.entries, args[0])
				if !ok {
					return fmt.Errorf("%s is not a vault entry", args[0])
				}
				if !s.sel.Selected(e) {
					fmt.Fprintf(out, "%s is excluded on this host (%s, mode: %s)\n",
						display(e), s.host.Name, s.sel.Mode)
					return nil
				}
				differs, err := diff_entry(out, e)
				if err != nil {
					return err
				}
				if differs {
					return err_differs
				}
				return nil
			}

			differs := false
			for _, e := range s.entries {
				if !s.sel.Selected(e) {
					continue
				}
				state, err := vault.Detect(e)
				if err != nil {
					return err
				}
				if state != vault.Conflict {
					continue
				}
				if err := print_diff(out, e); err != nil {
					return err
				}
				differs = true
			}
			if differs {
				return err_differs
			}
			return nil
		},
	}
}

// diff_entry handles an explicitly named entry: a diff when it
// conflicts, a state note otherwise. It reports whether a difference
// was shown.
func diff_entry(out io.Writer, e vault.Entry) (bool, error) {
	state, err := vault.Detect(e)
	if err != nil {
		return false, err
	}
	switch state {
	case vault.Conflict:
		return true, print_diff(out, e)
	case vault.Linked:
		fmt.Fprintf(out, "%s is linked; local content and vault are the same file\n", display(e))
	case vault.Unlinked:
		fmt.Fprintf(out, "%s is a real copy identical to the vault\n", display(e))
	case vault.Pending:
		fmt.Fprintf(out, "%s is not deployed on this machine; nothing to compare\n", display(e))
	case vault.Broken:
		fmt.Fprintf(out, "%s is a broken symlink; nothing to compare (see \"rigo clean\")\n", display(e))
	}
	return false, nil
}

// print_diff writes one conflicting entry's header and diff.
func print_diff(out io.Writer, e vault.Entry) error {
	d, err := vault.Compare(e)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "conflict: %s (%s)\n", display(e), d.Stat)
	if d.Unified != "" {
		fmt.Fprintf(out, "%s\n", d.Unified)
	}
	return nil
}
