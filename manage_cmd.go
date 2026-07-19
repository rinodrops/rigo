package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rinodrops/rigo/internal/config"
	"github.com/rinodrops/rigo/internal/vault"
)

func add_cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Move real content into the vault and link it back",
		Args:  cobra.ExactArgs(1),
		RunE:  run_add,
	}
	cmd.Flags().Bool("os", false, "place the entry under the OS-specific layer")
	cmd.Flags().Bool("dir", false, "deploy a directory as one symlink (recorded in rigo.toml)")
	cmd.Flags().Bool("files", false, "move and link each file inside individually")
	cmd.Flags().String("tag", "", "record the entry as a member of this tag")
	cmd.Flags().Bool("keep-symlink", false, "store a symlink source as-is instead of adopting its referent")
	cmd.Flags().String("volume", "", "named volume for a Windows path on a non-system drive")
	return cmd
}

func run_add(cmd *cobra.Command, args []string) error {
	flag_os, _ := cmd.Flags().GetBool("os")
	flag_dir, _ := cmd.Flags().GetBool("dir")
	flag_files, _ := cmd.Flags().GetBool("files")
	tag, _ := cmd.Flags().GetString("tag")
	keep, _ := cmd.Flags().GetBool("keep-symlink")
	flag_volume, _ := cmd.Flags().GetString("volume")
	if flag_dir && flag_files {
		return fmt.Errorf("--dir and --files are mutually exclusive")
	}

	s, err := setup(cmd)
	if err != nil {
		return err
	}
	src, err := config.ExpandHome(args[0])
	if err != nil {
		return err
	}
	src, err = filepath.Abs(src)
	if err != nil {
		return err
	}
	src = filepath.Clean(src)
	src_info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(s.root, src); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is inside the vault", src)
	}

	route, err := vault.Plan(s.cfg, s.host, s.volumes, src, flag_os)
	if err != nil {
		return err
	}
	declare_volume := ""
	if route.NeedsVolume {
		name, declare, err := choose_volume(cmd, s, route, flag_volume)
		if err != nil {
			return err
		}
		declare_volume = declare
		route = route.WithVolume(s.cfg, s.host, name)
	}

	if err := check_addable(s, route, src_info.IsDir()); err != nil {
		return err
	}

	// A symlink source adopts its referent, so directory-ness follows
	// the referent unless the symlink itself is being stored.
	is_dir := src_info.IsDir()
	if src_info.Mode()&fs.ModeSymlink != 0 && !keep {
		fi, err := os.Stat(src)
		if err != nil {
			return err
		}
		is_dir = fi.IsDir()
	}

	out := cmd.OutOrStdout()
	edit, err := config.OpenEdit(s.cfg_path)
	if err != nil {
		return err
	}
	dirty := false

	switch {
	case is_dir && !flag_dir && !flag_files:
		mode, err := ask_dir_mode(cmd)
		if err != nil {
			return err
		}
		flag_dir = mode == "d"
		flag_files = mode == "f"
		if !flag_dir && !flag_files {
			fmt.Fprintln(out, "aborted")
			return nil
		}
	}

	switch {
	case is_dir && flag_files:
		moved, err := add_files(s, route, src, keep, out)
		if err != nil {
			return err
		}
		if tag != "" {
			for _, logical := range moved {
				if err := edit.AppendItem([]string{"tags", tag}, logical); err != nil {
					return err
				}
			}
			dirty = len(moved) > 0
		}
	case is_dir:
		e := vault.Entry{Path: route.Logical, Vault: vault_dest(s, route), Target: src, Dir: true}
		if err := vault.Add(e, keep); err != nil {
			return err
		}
		fmt.Fprintf(out, "added %s/ (directory unit)\n", route.Logical)
		key := []string{"dirs"}
		if tag != "" {
			key = []string{"tags", tag}
		}
		if err := edit.AppendItem(key, route.Logical+"/"); err != nil {
			return err
		}
		dirty = true
	default:
		e := vault.Entry{Path: route.Logical, Vault: vault_dest(s, route), Target: src}
		if err := vault.Add(e, keep); err != nil {
			return err
		}
		fmt.Fprintf(out, "added %s\n", route.Logical)
		if tag != "" {
			if err := edit.AppendItem([]string{"tags", tag}, route.Logical); err != nil {
				return err
			}
			dirty = true
		}
	}

	if declare_volume != "" {
		if err := edit.SetKey([]string{"volumes", declare_volume}, route.Drive); err != nil {
			return err
		}
		fmt.Fprintf(out, "declared volume %s = %q in rigo.toml\n", declare_volume, route.Drive)
		dirty = true
	}
	if dirty {
		return edit.Save()
	}
	return nil
}

// add_files moves each file inside src into the vault individually
// (the directory itself stays real) and returns the logical paths.
func add_files(s *session, route vault.Route, src string, keep bool, out io.Writer) ([]string, error) {
	var moved []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		ignored, err := vault.Ignored(s.cfg, route.VaultRel+"/"+rel, false)
		if err != nil {
			return err
		}
		if ignored {
			return nil
		}
		e := vault.Entry{
			Path:   route.Logical + "/" + rel,
			Vault:  filepath.Join(vault_dest(s, route), filepath.FromSlash(rel)),
			Target: p,
		}
		if err := vault.Add(e, keep); err != nil {
			return fmt.Errorf("%s: %w", e.Path, err)
		}
		fmt.Fprintf(out, "added %s\n", e.Path)
		moved = append(moved, e.Path)
		return nil
	})
	return moved, err
}

func vault_dest(s *session, route vault.Route) string {
	return filepath.Join(s.root, filepath.FromSlash(route.VaultRel))
}

// check_addable refuses destinations that scanning could not manage.
func check_addable(s *session, route vault.Route, is_dir bool) error {
	if _, ok := vault.Find(s.entries, route.Logical); ok {
		return fmt.Errorf("%s is already managed", route.Logical)
	}
	if e, ok := vault.Covered(s.entries, route.Logical); ok {
		return fmt.Errorf("%s is inside %s, which deploys as a single directory symlink", route.Logical, e.Path)
	}
	ignored, err := vault.Ignored(s.cfg, route.VaultRel, is_dir)
	if err != nil {
		return err
	}
	if ignored {
		return fmt.Errorf("%s matches an ignore pattern and would be invisible to rigo", route.Logical)
	}
	if _, err := os.Lstat(vault_dest(s, route)); err == nil {
		return fmt.Errorf("the vault already contains %s", route.VaultRel)
	}
	return nil
}

// choose_volume resolves the volume for a non-system drive: from
// --volume, or interactively with a suggested default. It returns the
// volume name and, when undeclared, the name to write into [volumes].
func choose_volume(cmd *cobra.Command, s *session, route vault.Route, flag string) (string, string, error) {
	name := flag
	if name == "" {
		if !stdin_is_tty() {
			return "", "", fmt.Errorf("drive %s: is not the system drive; pass --volume <name> to pick the volume", strings.ToUpper(route.Drive))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "volume name for drive %s: [%s]: ", strings.ToUpper(route.Drive), route.Suggest)
		in := bufio.NewScanner(cmd.InOrStdin())
		if !in.Scan() {
			return "", "", in.Err()
		}
		name = strings.TrimSpace(in.Text())
		if name == "" {
			name = route.Suggest
		}
	}
	letter, declared := s.volumes[name]
	if declared && letter != route.Drive {
		return "", "", fmt.Errorf("volume %s resolves to drive %s: on this host, not %s:",
			name, strings.ToUpper(letter), strings.ToUpper(route.Drive))
	}
	if declared {
		return name, "", nil
	}
	return name, name, nil
}

// ask_dir_mode asks whether a directory deploys whole or as individual files.
func ask_dir_mode(cmd *cobra.Command) (string, error) {
	if !stdin_is_tty() {
		return "", fmt.Errorf("the path is a directory; pass --dir (one symlink) or --files (link each file)")
	}
	out := cmd.OutOrStdout()
	in := bufio.NewScanner(cmd.InOrStdin())
	for {
		fmt.Fprintln(out, "  d) whole directory: one symlink, new files follow automatically")
		fmt.Fprintln(out, "  f) files: move and link each file individually")
		fmt.Fprintln(out, "  a) abort")
		fmt.Fprint(out, "choice: ")
		if !in.Scan() {
			return "a", in.Err()
		}
		switch strings.TrimSpace(in.Text()) {
		case "d", "D":
			return "d", nil
		case "f", "F":
			return "f", nil
		case "a", "A", "q", "":
			return "a", nil
		}
	}
}

func forget_cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forget <path>",
		Short: "Stop managing an entry (materialize it; vault copy goes to trash)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := setup(cmd)
			if err != nil {
				return err
			}
			// forget is a vault-level operation, so entries excluded on
			// this host are still eligible; they are just not
			// materialized here.
			e, ok := vault.ResolveArg(s.entries, args[0])
			if !ok {
				return fmt.Errorf("%s is not a vault entry", args[0])
			}
			if e.Path == ".config/rigo/rigo.toml" {
				return fmt.Errorf("refusing to forget rigo.toml itself")
			}

			trash_rel, err := vault.Forget(e, s.root, s.cfg, s.sel.Selected(e))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "forgot %s (vault copy moved to %s)\n", display(e), trash_rel)

			edit, err := config.OpenEdit(s.cfg_path)
			if err != nil {
				return err
			}
			if n := edit.RemoveRefs(e.Path); n > 0 {
				if err := edit.Save(); err != nil {
					return err
				}
				fmt.Fprintf(out, "removed %d reference(s) from rigo.toml\n", n)
			}
			return nil
		},
	}
}
