package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rinodrops/rigo/internal/config"
	"github.com/rinodrops/rigo/internal/vault"
)

func main() {
	root := &cobra.Command{
		Use:           "rigo",
		Short:         "Dotfiles manager: vault + symlink",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringP("file", "f", "",
		"path to rigo.toml inside the vault (first-run bootstrap)")
	root.AddCommand(status_cmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "rigo:", err)
		os.Exit(1)
	}
}

// load resolves the config path (the -f flag or symlink discovery),
// loads rigo.toml, and returns it with the vault root.
func load(cmd *cobra.Command) (*config.Config, string, error) {
	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return nil, "", err
	}
	var cfg_path, vault_root string
	if file != "" {
		cfg_path, vault_root, err = config.FromFile(file)
	} else {
		cfg_path, vault_root, err = config.Discover()
	}
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(cfg_path)
	if err != nil {
		return nil, "", err
	}
	return cfg, vault_root, nil
}

func status_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [<path>]",
		Short: "Show the state of managed entries",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, vault_root, err := load(cmd)
			if err != nil {
				return err
			}
			host, err := vault.Current()
			if err != nil {
				return err
			}
			entries, warnings, err := vault.Scan(vault_root, cfg, host)
			if err != nil {
				return err
			}
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
			}
			sel := vault.Select(cfg, host.Name)

			if len(args) == 1 {
				e, ok := vault.Find(entries, args[0])
				if !ok {
					return fmt.Errorf("%s is not a vault entry", args[0])
				}
				entries = []vault.Entry{e}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "host: %s (mode: %s)\n\n", host.Name, sel.Mode)
			for _, e := range entries {
				state := "excluded"
				if sel.Selected(e) {
					st, err := vault.Detect(e)
					if err != nil {
						return err
					}
					state = st.String()
				}
				name := e.Path
				if e.Dir {
					name += "/"
				}
				fmt.Fprintf(out, "%-9s %s\n", state, name)
			}
			return nil
		},
	}
}
