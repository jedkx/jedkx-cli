package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/config"
)

// object is a JSON object that keeps its key order and the file's indentation,
// so changing a few keys in someone else's settings file leaves the rest as it was.
type object struct {
	keys   []string
	vals   map[string]json.RawMessage
	indent string
	dirty  map[string]bool // keys we changed; the rest is written back byte for byte
	noEOL  bool            // the file had no trailing newline
}

func (o *object) get(k string) (json.RawMessage, bool) {
	v, ok := o.vals[k]
	return v, ok
}

func (o *object) set(k string, v json.RawMessage) {
	o.put(k, v)
	if o.dirty == nil {
		o.dirty = map[string]bool{}
	}
	o.dirty[k] = true
}

// put stores a value as read from the file (not marked as changed).
func (o *object) put(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// getPath / setPath / delPath address nested objects with dots: "profiles.defaults.colorScheme".
func (o *object) getPath(path string) (json.RawMessage, bool) {
	head, rest, nested := strings.Cut(path, ".")
	v, ok := o.get(head)
	if !ok || !nested {
		return v, ok
	}
	child, err := parseObject(v)
	if err != nil {
		return nil, false
	}
	return child.getPath(rest)
}

func (o *object) setPath(path string, v json.RawMessage) error {
	head, rest, nested := strings.Cut(path, ".")
	if !nested {
		o.set(head, v)
		return nil
	}
	cur, ok := o.get(head)
	child := &object{vals: map[string]json.RawMessage{}}
	if ok {
		var err error
		if child, err = parseObject(cur); err != nil {
			return fmt.Errorf("%s is not an object", head)
		}
	}
	if err := child.setPath(rest, v); err != nil {
		return err
	}
	b, err := child.marshal("", "  ")
	if err != nil {
		return err
	}
	o.set(head, b)
	return nil
}

func (o *object) delPath(path string) {
	head, rest, nested := strings.Cut(path, ".")
	if !nested {
		o.del(head)
		return
	}
	cur, ok := o.get(head)
	if !ok {
		return
	}
	child, err := parseObject(cur)
	if err != nil {
		return
	}
	child.delPath(rest)
	if len(child.keys) == 0 { // we created it; leave no empty shell behind
		o.del(head)
		return
	}
	if b, err := child.marshal("", "  "); err == nil {
		o.set(head, b)
	}
}

func parseObject(b []byte) (*object, error) {
	o := &object{vals: map[string]json.RawMessage{}, indent: detectIndent(b)}
	if len(bytes.TrimSpace(b)) == 0 {
		return o, nil
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		k, _ := t.(string)
		var v json.RawMessage
		if err := d.Decode(&v); err != nil {
			return nil, err
		}
		o.put(k, v)
	}
	return o, nil
}

func readObject(path string) (*object, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &object{vals: map[string]json.RawMessage{}, indent: "  "}, nil
	}
	if err != nil {
		return nil, err
	}
	o, err := parseObject(b)
	if err == nil {
		o.noEOL = !bytes.HasSuffix(b, []byte("\n"))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %v (comments are not supported) — left untouched", path, err)
	}
	return o, nil
}

func (o *object) marshal(prefix, indent string) ([]byte, error) {
	var b bytes.Buffer
	if len(o.keys) == 0 {
		return []byte("{}"), nil
	}
	b.WriteString("{\n")
	for i, k := range o.keys {
		kb, _ := json.Marshal(k)
		val := o.vals[k]
		if o.dirty[k] {
			var v bytes.Buffer
			if err := json.Indent(&v, val, prefix+indent, indent); err != nil {
				return nil, err
			}
			val = v.Bytes()
		}
		b.WriteString(prefix + indent + string(kb) + ": " + string(val))
		if i < len(o.keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(prefix + "}")
	return b.Bytes(), nil
}

func writeObject(path string, o *object) error {
	ind := o.indent
	if ind == "" {
		ind = "  "
	}
	b, err := o.marshal("", ind)
	if err != nil {
		return err
	}
	if !o.noEOL {
		b = append(b, '\n')
	}
	return config.WriteAtomic(path, b)
}

// detectIndent returns the leading whitespace of the first indented line.
func detectIndent(b []byte) string {
	for _, line := range bytes.Split(b, []byte("\n")) {
		t := bytes.TrimLeft(line, " \t")
		if len(t) > 0 && len(t) < len(line) {
			return string(line[:len(line)-len(t)])
		}
	}
	return "  "
}
