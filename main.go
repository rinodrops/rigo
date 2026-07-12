package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:     "rigo",
		Short:   "Dotfiles manager: vault + symlink, synced by Syncthing",
		Version: Version,
	}
	root.PersistentFlags().StringP("file", "f", "",
		"path to rigo.toml inside the vault (first-run bootstrap)")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
