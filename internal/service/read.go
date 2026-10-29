package service

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
)

// BerthdName is berthd's service name here: the launchd label
// dev.berth.berthd on macOS, the systemd unit berthd elsewhere. `berthd
// install` writes it, and the laptop agent looks for it to find a berthd
// already serving this computer.
func BerthdName() string {
	if goos == "darwin" {
		return "dev.berth.berthd"
	}
	return "berthd"
}

// Unit is what an installed unit or plist runs.
type Unit struct {
	Path    string
	Program string
	Args    []string
	Env     map[string]string
}

// Arg is the value after flag in the unit's arguments, as in --listen ADDR.
func (u Unit) Arg(flag string) string {
	for i, a := range u.Args {
		if a == flag && i+1 < len(u.Args) {
			return u.Args[i+1]
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v
		}
	}
	return ""
}

// Read reads the unit installed under name; ok is false when there is none.
// It understands what Render writes, which is all Install ever installs.
func Read(name string) (u Unit, ok bool, err error) {
	path, err := unitPath(Spec{Name: name})
	if err != nil {
		return Unit{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Unit{}, false, nil
	}
	if err != nil {
		return Unit{}, false, err
	}
	if goos == "windows" {
		// A task's XML is not read back: callers fall back to what they
		// do without a service (start the program themselves).
		return Unit{}, false, nil
	}
	if goos == "darwin" {
		u, err = parsePlist(data)
	} else {
		u, err = parseSystemd(data)
	}
	if err != nil {
		return Unit{}, false, fmt.Errorf("%s: %w", path, err)
	}
	u.Path = path
	return u, true, nil
}

// plistValue is a string, an array or a dict; other values are kept empty.
type plistValue struct {
	str  string
	arr  []plistValue
	dict map[string]plistValue
}

func parsePlist(data []byte) (Unit, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	var root plistValue
	found := false
	for !found {
		tok, err := d.Token()
		if err != nil {
			return Unit{}, fmt.Errorf("not a plist: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "dict" {
			if root, err = readPlistValue(d, se); err != nil {
				return Unit{}, err
			}
			found = true
		}
	}
	argv := root.dict["ProgramArguments"].arr
	if len(argv) == 0 {
		return Unit{}, errors.New("no ProgramArguments")
	}
	u := Unit{Program: argv[0].str, Env: map[string]string{}}
	for _, a := range argv[1:] {
		u.Args = append(u.Args, a.str)
	}
	for k, v := range root.dict["EnvironmentVariables"].dict {
		u.Env[k] = v.str
	}
	return u, nil
}

func readPlistValue(d *xml.Decoder, start xml.StartElement) (plistValue, error) {
	var v plistValue
	switch start.Name.Local {
	case "dict":
		v.dict = map[string]plistValue{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return v, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					if err := d.DecodeElement(&key, &t); err != nil {
						return v, err
					}
					continue
				}
				child, err := readPlistValue(d, t)
				if err != nil {
					return v, err
				}
				v.dict[key] = child
			case xml.EndElement:
				return v, nil
			}
		}
	case "array":
		for {
			tok, err := d.Token()
			if err != nil {
				return v, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				child, err := readPlistValue(d, t)
				if err != nil {
					return v, err
				}
				v.arr = append(v.arr, child)
			case xml.EndElement:
				return v, nil
			}
		}
	case "string":
		err := d.DecodeElement(&v.str, &start)
		return v, err
	}
	return v, d.Skip()
}

// parseSystemd reads ExecStart and Environment from a unit Render wrote.
func parseSystemd(data []byte) (Unit, error) {
	u := Unit{Env: map[string]string{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "ExecStart="); ok {
			argv := systemdSplit(rest)
			if len(argv) == 0 {
				return Unit{}, errors.New("an empty ExecStart")
			}
			u.Program, u.Args = argv[0], argv[1:]
		}
		if rest, ok := strings.CutPrefix(line, "Environment="); ok {
			for _, kv := range systemdSplit(rest) {
				if k, v, ok := strings.Cut(kv, "="); ok {
					u.Env[k] = v
				}
			}
		}
	}
	if u.Program == "" {
		return Unit{}, errors.New("no ExecStart")
	}
	return u, nil
}

// systemdSplit undoes systemdArgs: words split on spaces, double-quoted
// words with \\, \" and $$ / %% escapes.
func systemdSplit(s string) []string {
	var out []string
	var b strings.Builder
	inWord, quoted := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quoted && c == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case quoted && (c == '$' || c == '%') && i+1 < len(s) && s[i+1] == c:
			i++
			b.WriteByte(c)
		case c == '"':
			quoted = !quoted
			inWord = true
		case !quoted && (c == ' ' || c == '\t'):
			if inWord {
				out = append(out, b.String())
				b.Reset()
				inWord = false
			}
		default:
			b.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		out = append(out, b.String())
	}
	return out
}
