package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rinodrops/rigo/internal/secrets"
)

func secrets_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Materialize secrets from password-manager backends",
	}
	cmd.AddCommand(secrets_apply_cmd(), secrets_status_cmd(), secrets_remove_cmd())
	return cmd
}

// secrets_plan loads the session and resolves the applicable items.
func secrets_plan(cmd *cobra.Command, args []string) (*session, []secrets.Item, error) {
	s, err := setup(cmd)
	if err != nil {
		return nil, nil, err
	}
	only := ""
	if len(args) == 1 {
		only = args[0]
	}
	items, err := secrets.Plan(s.cfg, s.host, s.sel, s.entries, only)
	if err != nil {
		return nil, nil, err
	}
	return s, items, nil
}

func secrets_apply_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply [<path>]",
		Short: "Fetch every applicable secret and write it to its path",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, items, err := secrets_plan(cmd, args)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(items) == 0 {
				fmt.Fprintln(out, "no secrets apply to this host")
				return nil
			}
			for _, item := range items {
				if err := secrets.Apply(nil, item); err != nil {
					return err
				}
				fmt.Fprintf(out, "applied  %s\n", item.Path)
			}
			fmt.Fprintf(out, "\n%d secret(s) written\n", len(items))
			return nil
		},
	}
}

func secrets_status_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [<path>]",
		Short: "Report applied/missing for each secret (existence only)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, items, err := secrets_plan(cmd, args)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(items) == 0 {
				fmt.Fprintln(out, "no secrets apply to this host")
				return nil
			}
			for _, item := range items {
				state := "missing"
				if secrets.Applied(item) {
					state = "applied"
				}
				fmt.Fprintf(out, "%-8s %s\n", state, item.Path)
			}
			return nil
		},
	}
}

func secrets_remove_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [<path>]",
		Short: "Delete the written secret files (the reverse of apply)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, items, err := secrets_plan(cmd, args)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			removed := 0
			for _, item := range items {
				gone, err := secrets.Remove(item)
				if err != nil {
					return err
				}
				if gone {
					fmt.Fprintf(out, "removed  %s\n", item.Path)
					removed++
				}
			}
			fmt.Fprintf(out, "\n%d file(s) removed", removed)
			if removed > 0 {
				fmt.Fprint(out, " (note: secure erase is not guaranteed on SSDs)")
			}
			fmt.Fprintln(out)
			return nil
		},
	}
}
