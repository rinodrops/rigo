package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/rinodrops/rigo/internal/config"
	"github.com/rinodrops/rigo/internal/vault"
)

// diff_auto_show is the largest diff (in lines) printed without asking.
const diff_auto_show = 40

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
	root.AddCommand(status_cmd(), apply_cmd(), link_cmd(), unlink_cmd(), add_cmd(), forget_cmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "rigo:", err)
		os.Exit(1)
	}
}

// session is everything a command needs: the loaded config, the
// scanned entries for this host, and the selection filter.
type session struct {
	cfg      *config.Config
	cfg_path string // resolved (vault-side) path of rigo.toml
	root     string
	host     vault.Host
	entries  []vault.Entry
	sel      *vault.Selection
	volumes  map[string]string
}

func setup(cmd *cobra.Command) (*session, error) {
	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return nil, err
	}
	var cfg_path, vault_root string
	if file != "" {
		cfg_path, vault_root, err = config.FromFile(file)
	} else {
		cfg_path, vault_root, err = config.Discover()
	}
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cfg_path)
	if err != nil {
		return nil, err
	}
	host, err := vault.Current()
	if err != nil {
		return nil, err
	}
	entries, warnings, err := vault.Scan(vault_root, cfg, host)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
	}
	volumes, err := vault.Volumes(cfg, host)
	if err != nil {
		return nil, err
	}
	return &session{
		cfg:      cfg,
		cfg_path: cfg_path,
		root:     vault_root,
		host:     host,
		entries:  entries,
		sel:      vault.Select(cfg, host.Name),
		volumes:  volumes,
	}, nil
}

// pick resolves a path argument to a selected entry.
func (s *session) pick(path string) (vault.Entry, error) {
	e, ok := vault.Find(s.entries, path)
	if !ok {
		return vault.Entry{}, fmt.Errorf("%s is not a vault entry", path)
	}
	if !s.sel.Selected(e) {
		return vault.Entry{}, fmt.Errorf("%s is excluded on this host (%s, mode: %s)",
			e.Path, s.host.Name, s.sel.Mode)
	}
	return e, nil
}

func display(e vault.Entry) string {
	if e.Dir {
		return e.Path + "/"
	}
	return e.Path
}

func status_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [<path>]",
		Short: "Show the state of managed entries",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			entries := s.entries
			if len(args) == 1 {
				e, ok := vault.Find(entries, args[0])
				if !ok {
					return fmt.Errorf("%s is not a vault entry", args[0])
				}
				entries = []vault.Entry{e}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "host: %s (mode: %s)\n\n", s.host.Name, s.sel.Mode)
			for _, e := range entries {
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
			return nil
		},
	}
}

func apply_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Converge every selected entry (link pending and unlinked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			res := vault.Apply(s.entries, s.sel)

			out := cmd.OutOrStdout()
			for _, p := range res.Linked {
				fmt.Fprintf(out, "linked    %s\n", p)
			}
			for _, p := range res.Conflicts {
				fmt.Fprintf(out, "conflict  %s  (resolve with \"rigo link %s\")\n", p, p)
			}
			for _, p := range res.Broken {
				fmt.Fprintf(out, "broken    %s  (left untouched)\n", p)
			}
			for _, f := range res.Failed {
				fmt.Fprintf(cmd.ErrOrStderr(), "failed    %s\n", f)
			}
			fmt.Fprintf(out, "\n%d linked, %d conflict, %d broken, %d failed, %d excluded\n",
				len(res.Linked), len(res.Conflicts), len(res.Broken), len(res.Failed), res.Excluded)
			if n := len(res.Failed); n > 0 {
				return fmt.Errorf("%d entries failed to converge", n)
			}
			return nil
		},
	}
}

func link_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link <path>",
		Short: "Symlink one entry from the vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			e, err := s.pick(args[0])
			if err != nil {
				return err
			}
			state, err := vault.Detect(e)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			switch state {
			case vault.Linked:
				fmt.Fprintf(out, "%s is already linked\n", display(e))
				return nil
			case vault.Pending, vault.Unlinked:
				if err := vault.Link(e); err != nil {
					return err
				}
			case vault.Broken:
				dest, _ := os.Readlink(e.Target)
				fmt.Fprintf(out, "replacing symlink that pointed to %s\n", dest)
				if err := vault.Link(e); err != nil {
					return err
				}
			case vault.Conflict:
				if err := resolve_conflict(cmd, e, force); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "resolve a conflict by taking the vault version")
	return cmd
}

// resolve_conflict handles the interactive part of rigo link.
func resolve_conflict(cmd *cobra.Command, e vault.Entry, force bool) error {
	out := cmd.OutOrStdout()
	if force {
		if err := vault.Link(e); err != nil {
			return err
		}
		fmt.Fprintf(out, "linked %s (vault version; local content replaced)\n", display(e))
		return nil
	}
	if !stdin_is_tty() {
		return fmt.Errorf("%s conflicts with the vault; re-run interactively or use --force to take the vault version", e.Path)
	}

	d, err := vault.Compare(e)
	if err != nil {
		return err
	}
	print_conflict(out, e, d)

	in := bufio.NewScanner(cmd.InOrStdin())
	choice, err := prompt_choice(in, out, d)
	if err != nil {
		return err
	}
	switch choice {
	case 1:
		if err := vault.Link(e); err != nil {
			return err
		}
		fmt.Fprintf(out, "linked %s (vault version)\n", display(e))
	case 2:
		if err := vault.Adopt(e); err != nil {
			return err
		}
		fmt.Fprintf(out, "linked %s (local content adopted into the vault)\n", display(e))
	default:
		fmt.Fprintln(out, "aborted")
	}
	return nil
}

func print_conflict(out io.Writer, e vault.Entry, d vault.Diff) {
	fmt.Fprintf(out, "conflict: %s\n", display(e))
	fmt.Fprintf(out, "  vault: %s\n", describe_file(e.Vault))
	fmt.Fprintf(out, "  local: %s\n", describe_file(e.Target))
	fmt.Fprintf(out, "  %s\n", d.Stat)
	if d.Unified != "" && d.Lines <= diff_auto_show {
		fmt.Fprintf(out, "\n%s\n", d.Unified)
	}
}

func describe_file(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return err.Error()
	}
	when := fi.ModTime().Format(time.DateTime)
	if fi.IsDir() {
		return fmt.Sprintf("directory, modified %s", when)
	}
	return fmt.Sprintf("%d B, modified %s", fi.Size(), when)
}

func prompt_choice(in *bufio.Scanner, out io.Writer, d vault.Diff) (int, error) {
	show_diff_option := d.Unified != "" && d.Lines > diff_auto_show
	for {
		fmt.Fprintln(out, "  1) take the vault version (local content is replaced)")
		fmt.Fprintln(out, "  2) adopt the local content into the vault, then link")
		fmt.Fprintln(out, "  3) abort")
		if show_diff_option {
			fmt.Fprintln(out, "  d) show diff")
		}
		fmt.Fprint(out, "choice: ")
		if !in.Scan() {
			return 3, in.Err()
		}
		switch in.Text() {
		case "1":
			return 1, nil
		case "2":
			return 2, nil
		case "3", "q", "":
			return 3, nil
		case "d", "D":
			if show_diff_option {
				fmt.Fprintf(out, "\n%s\n", d.Unified)
			}
		}
	}
}

func stdin_is_tty() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func unlink_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink <path>",
		Short: "Materialize one entry locally (the vault copy is kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			e, err := s.pick(args[0])
			if err != nil {
				return err
			}
			state, err := vault.Detect(e)
			if err != nil {
				return err
			}
			switch state {
			case vault.Linked:
				if err := vault.Unlink(e); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "unlinked %s (real copy in place; still managed)\n", display(e))
				return nil
			case vault.Pending:
				return fmt.Errorf("%s is not deployed on this machine; nothing to materialize", e.Path)
			case vault.Unlinked:
				return fmt.Errorf("%s is already a real file identical to the vault", e.Path)
			case vault.Conflict:
				return fmt.Errorf("%s is already a real file that differs from the vault; see \"rigo link %s\"", e.Path, e.Path)
			default:
				return fmt.Errorf("%s is a broken symlink; use \"rigo link %s\" to relink it", e.Path, e.Path)
			}
		},
	}
}
