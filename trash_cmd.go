package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rinodrops/rigo/internal/vault"
)

func trash_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trash",
		Short: "Inspect, restore, or purge trashed vault entries",
	}
	cmd.AddCommand(trash_ls_cmd(), trash_restore_cmd(), trash_empty_cmd())
	return cmd
}

func trash_ls_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List trashed entries, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			trash, err := vault.TrashList(s.root, s.cfg, s.host)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(trash) == 0 {
				fmt.Fprintln(out, "trash is empty")
				return nil
			}
			for _, t := range trash {
				when, err := time.Parse("20060102T150405Z", t.Stamp)
				stamp := t.Stamp
				if err == nil {
					stamp = when.Local().Format(time.DateTime)
				}
				shown := t.Logical
				if shown == "" {
					shown = t.VaultRel + "  (not deployed on this host)"
				}
				fmt.Fprintf(out, "%s  %s\n", stamp, shown)
			}
			return nil
		},
	}
}

func trash_restore_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <path>",
		Short: "Move the newest trashed copy of a path back into the vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			trash, err := vault.TrashList(s.root, s.cfg, s.host)
			if err != nil {
				return err
			}
			t, ok := vault.Newest(trash, args[0])
			if !ok {
				return fmt.Errorf("no trashed copy of %s", args[0])
			}
			if err := vault.TrashRestore(s.root, s.cfg, t); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "restored %s to the vault (from %s)\n", t.Logical, t.Stamp)
			fmt.Fprintln(out, "note: references removed from rigo.toml by forget (dirs/tags/include/exclude) are not restored; re-add them if needed")
			fmt.Fprintf(out, "run \"rigo apply\" or \"rigo link %s\" to deploy it again\n", t.Logical)
			return nil
		},
	}
}

func trash_empty_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "empty",
		Short: "Permanently delete trashed entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			older, _ := cmd.Flags().GetString("older-than")
			cutoff := time.Duration(0)
			if older != "" {
				d, err := parse_age(older)
				if err != nil {
					return err
				}
				cutoff = d
			}

			s, err := setup(cmd)
			if err != nil {
				return err
			}
			trash, err := vault.TrashList(s.root, s.cfg, s.host)
			if err != nil {
				return err
			}
			if len(trash) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "trash is empty")
				return nil
			}

			if !force {
				if !stdin_is_tty() {
					return fmt.Errorf("pass --force to empty the trash without a prompt")
				}
				scope := "all trashed entries"
				if cutoff > 0 {
					scope = fmt.Sprintf("trashed entries older than %s", older)
				}
				fmt.Fprintf(cmd.OutOrStdout(),
					"permanently delete %s? Other machines lose this safety net. [y/N]: ", scope)
				in := bufio.NewScanner(cmd.InOrStdin())
				if !in.Scan() || !strings.EqualFold(strings.TrimSpace(in.Text()), "y") {
					fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
			}

			deleted, err := vault.TrashEmpty(s.root, s.cfg, cutoff)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %d generation(s)\n", len(deleted))
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "skip the confirmation prompt")
	cmd.Flags().String("older-than", "", "only delete generations older than this age (e.g. 30d, 72h)")
	return cmd
}

// parse_age parses a Go duration plus a "d" suffix for days.
func parse_age(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid age %q (use e.g. 30d or 72h)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid age %q (use e.g. 30d or 72h)", s)
	}
	return d, nil
}

func clean_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Clean up broken symlinks (offering trash restores)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			trash, err := vault.TrashList(s.root, s.cfg, s.host)
			if err != nil {
				return err
			}
			stale, err := vault.FindStale(s.entries, trash, s.root)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(stale) == 0 {
				fmt.Fprintln(out, "nothing to clean")
				return nil
			}

			interactive := stdin_is_tty()
			if !interactive {
				for _, st := range stale {
					fmt.Fprintf(out, "broken  %s -> %s\n", st.Logical, st.Dest)
				}
				fmt.Fprintf(out, "\n%d broken link(s); re-run interactively to clean them\n", len(stale))
				return nil
			}

			in := bufio.NewScanner(cmd.InOrStdin())
			for _, st := range stale {
				fmt.Fprintf(out, "broken: %s (pointed to %s)\n", st.Logical, st.Dest)
				if err := clean_one(in, out, st); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func clean_one(in *bufio.Scanner, out interface{ Write([]byte) (int, error) }, st vault.Stale) error {
	for {
		if st.HasCopy {
			fmt.Fprintf(out, "  r) restore the trash copy (%s) as a real file\n", st.Trash.Stamp)
		}
		fmt.Fprintln(out, "  d) delete the symlink")
		fmt.Fprintln(out, "  s) skip")
		fmt.Fprint(out, "choice: ")
		if !in.Scan() {
			return in.Err()
		}
		switch strings.TrimSpace(in.Text()) {
		case "r", "R":
			if !st.HasCopy {
				continue
			}
			if err := vault.RestoreLocal(st.Trash); err != nil {
				return err
			}
			fmt.Fprintf(out, "restored %s as a real file (trash copy kept)\n", st.Logical)
			return nil
		case "d", "D":
			if err := vault.RemoveLink(st.Target); err != nil {
				return err
			}
			fmt.Fprintf(out, "deleted %s\n", st.Logical)
			return nil
		case "s", "S", "", "q":
			fmt.Fprintln(out, "skipped")
			return nil
		}
	}
}
