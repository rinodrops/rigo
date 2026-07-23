package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
)

// Edit performs format-preserving mutations on a rigo.toml: user
// comments, blank lines, and ordering survive a load/save round trip.
type Edit struct {
	doc  *tomledit.Document
	path string
}

// OpenEdit parses the rigo.toml at path for editing. Pass the
// vault-side real path (the resolved config path), not the symlink.
func OpenEdit(path string) (*Edit, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	doc, err := tomledit.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Edit{doc: doc, path: path}, nil
}

// Save writes the document back atomically (temp file + rename).
func (e *Edit) Save() error {
	tmp, err := os.CreateTemp(filepath.Dir(e.path), ".rigo-toml-*")
	if err != nil {
		return err
	}
	var format tomledit.Formatter
	if err := format.Format(tmp, e.doc); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), e.path)
}

// AppendItem appends a string item to the array at key, creating the
// key (and its table section) when missing. key is either a top-level
// array key like ["dirs"] or a table entry like ["tags", "vim"].
func (e *Edit) AppendItem(key []string, item string) error {
	kv, err := e.ensure_array(key)
	if err != nil {
		return err
	}
	arr, ok := kv.Value.X.(parser.Array)
	if !ok {
		return fmt.Errorf("%s is not an array", strings.Join(key, "."))
	}
	kv.Value.X = append(arr, parser.MustValue(quote(item)))
	return nil
}

// SetKey sets a string value at key (e.g. ["volumes", "data"] = "d"),
// creating the section and key as needed. An existing value is
// overwritten.
func (e *Edit) SetKey(key []string, value string) error {
	if kv := e.find_kv(key); kv != nil {
		kv.Value = parser.MustValue(quote(value))
		return nil
	}
	section, name := e.split(key)
	s, err := e.ensure_section(section)
	if err != nil {
		return err
	}
	s.Items = append(s.Items, &parser.KeyValue{
		Name:  parser.Key{name},
		Value: parser.MustValue(quote(value)),
	})
	return nil
}

// RemoveRefs removes every array item equal to path (modulo a trailing
// slash) from dirs and from every array under [tags], [include],
// [exclude], and [extra]. Keys whose arrays become empty are kept. It
// returns the number of removed items.
func (e *Edit) RemoveRefs(path string) int {
	want := strings.TrimSuffix(path, "/")
	removed := 0

	clean := func(kv *parser.KeyValue) {
		arr, ok := kv.Value.X.(parser.Array)
		if !ok {
			return
		}
		kept := arr[:0]
		for _, item := range arr {
			if v, ok := item.(parser.Value); ok && unquote(v.X.String()) != want &&
				strings.TrimSuffix(unquote(v.X.String()), "/") != want {
				kept = append(kept, item)
				continue
			}
			if _, ok := item.(parser.Value); !ok { // keep embedded comments
				kept = append(kept, item)
				continue
			}
			removed++
		}
		kv.Value.X = kept
	}

	if kv := e.find_kv([]string{"dirs"}); kv != nil {
		clean(kv)
	}
	for _, table := range []string{"tags", "include", "exclude", "extra"} {
		for _, entry := range e.doc.Find(table) {
			if entry.Section == nil {
				continue
			}
			entry.Section.Scan(func(_ parser.Key, item *tomledit.Entry) bool {
				if item.KeyValue != nil {
					clean(item.KeyValue)
				}
				return true
			})
		}
	}
	return removed
}

func (e *Edit) find_kv(key []string) *parser.KeyValue {
	entry := e.doc.First(key...)
	if entry == nil {
		return nil
	}
	return entry.KeyValue
}

// ensure_array returns the KeyValue holding the array at key, creating
// an empty array (and its section) when absent.
func (e *Edit) ensure_array(key []string) (*parser.KeyValue, error) {
	if kv := e.find_kv(key); kv != nil {
		return kv, nil
	}
	section, name := e.split(key)
	s, err := e.ensure_section(section)
	if err != nil {
		return nil, err
	}
	kv := &parser.KeyValue{
		Name:  parser.Key{name},
		Value: parser.Value{X: parser.Array{}},
	}
	s.Items = append(s.Items, kv)
	return kv, nil
}

// split separates a key into its section path and final name.
func (e *Edit) split(key []string) ([]string, string) {
	return key[:len(key)-1], key[len(key)-1]
}

// ensure_section returns the section at path ([] means global),
// creating it when absent.
func (e *Edit) ensure_section(path []string) (*tomledit.Section, error) {
	if len(path) == 0 {
		if e.doc.Global == nil {
			e.doc.Global = &tomledit.Section{}
		}
		return e.doc.Global, nil
	}
	for _, s := range e.doc.Sections {
		if s.Heading != nil && s.TableName().Equals(parser.Key(path)) {
			return s, nil
		}
	}
	s := &tomledit.Section{Heading: &parser.Heading{Name: parser.Key(path)}}
	e.doc.Sections = append(e.doc.Sections, s)
	return s, nil
}

func quote(s string) string {
	return strconv.Quote(s)
}

// unquote strips the TOML string quoting from a formatted scalar.
func unquote(s string) string {
	if u, err := strconv.Unquote(s); err == nil {
		return u
	}
	return strings.Trim(s, `'"`)
}
