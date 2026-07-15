package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rinodrops/rigo/internal/vault"
)

func tag_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Bulk operations on tagged groups",
	}
	cmd.AddCommand(tag_show_cmd(), tag_link_cmd(), tag_unlink_cmd())
	return cmd
}

func tag_show_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "List a tag's members and their states",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			members, missing, err := vault.Members(s.cfg, s.entries, args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, e := range members {
				state := "excluded"
				if s.sel.Selected(e) {
					st, err := vault.Detect(e)
					if err != nil {
						return err
					}
					state = st.String()
				}
				fmt.Fprintf(out, "%-9s %s\n", state, display(e))
			}
			for _, p := range missing {
				fmt.Fprintf(out, "%-9s %s\n", "missing", p)
			}
			return nil
		},
	}
}

func tag_link_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link <name>",
		Short: "Link every member of a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			members, missing, err := vault.Members(s.cfg, s.entries, args[0])
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			linked, skipped, conflicts, excluded := 0, 0, 0, 0
			for _, e := range members {
				if !s.sel.Selected(e) {
					fmt.Fprintf(out, "excluded  %s\n", display(e))
					excluded++
					continue
				}
				state, err := vault.Detect(e)
				if err != nil {
					return err
				}
				switch state {
				case vault.Linked:
					skipped++
				case vault.Pending, vault.Unlinked:
					if err := vault.Link(e); err != nil {
						return err
					}
					fmt.Fprintf(out, "linked    %s\n", display(e))
					linked++
				case vault.Broken:
					dest, _ := os.Readlink(e.Target)
					fmt.Fprintf(out, "relinked  %s (pointed to %s)\n", display(e), dest)
					if err := vault.Link(e); err != nil {
						return err
					}
					linked++
				case vault.Conflict:
					if !force && !stdin_is_tty() {
						fmt.Fprintf(out, "conflict  %s  (skipped; resolve with \"rigo link %s\" or --force)\n", display(e), e.Path)
						conflicts++
						continue
					}
					if err := resolve_conflict(cmd, e, force); err != nil {
						return err
					}
					linked++
				}
			}
			fmt.Fprintf(out, "\n%d linked, %d already linked, %d conflict, %d excluded, %d missing\n",
				linked, skipped, conflicts, excluded, len(missing))
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "resolve conflicts by taking the vault version")
	return cmd
}

func tag_unlink_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink <name>",
		Short: "Materialize every linked member of a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			members, missing, err := vault.Members(s.cfg, s.entries, args[0])
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			unlinked, skipped, excluded := 0, 0, 0
			for _, e := range members {
				if !s.sel.Selected(e) {
					fmt.Fprintf(out, "excluded  %s\n", display(e))
					excluded++
					continue
				}
				state, err := vault.Detect(e)
				if err != nil {
					return err
				}
				if state != vault.Linked {
					fmt.Fprintf(out, "%-9s %s  (left alone)\n", state, display(e))
					skipped++
					continue
				}
				if err := vault.Unlink(e); err != nil {
					return err
				}
				fmt.Fprintf(out, "unlinked  %s\n", display(e))
				unlinked++
			}
			fmt.Fprintf(out, "\n%d unlinked, %d left alone, %d excluded, %d missing\n",
				unlinked, skipped, excluded, len(missing))
			return nil
		},
	}
}
