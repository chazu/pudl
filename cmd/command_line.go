package cmd

import (
	"fmt"
	"strings"
)

// splitCommandLine splits a command line into argv the way a POSIX shell
// would, without any expansion: whitespace separates words, single quotes
// keep everything literally, double quotes keep everything except \" \\ \$
// and \` escapes, and a backslash outside quotes escapes the next character.
// It is how `--populate command:<cmdline>` becomes a #CommandRun argv, which
// pudl then runs directly — no shell, so nothing in the line is interpreted.
func splitCommandLine(line string) ([]string, error) {
	var argv []string
	var word strings.Builder
	inWord := false
	const (
		plain = iota
		single
		double
	)
	state := plain
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch state {
		case single:
			if r == '\'' {
				state = plain
			} else {
				word.WriteRune(r)
			}
		case double:
			switch {
			case r == '"':
				state = plain
			case r == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[i+1]):
				i++
				word.WriteRune(runes[i])
			default:
				word.WriteRune(r)
			}
		default:
			switch r {
			case ' ', '\t', '\n':
				if inWord {
					argv = append(argv, word.String())
					word.Reset()
					inWord = false
				}
				continue
			case '\'':
				state = single
			case '"':
				state = double
			case '\\':
				if i+1 == len(runes) {
					return nil, fmt.Errorf("command line ends with a backslash")
				}
				i++
				word.WriteRune(runes[i])
			default:
				word.WriteRune(r)
			}
			inWord = true
		}
	}
	if state != plain {
		return nil, fmt.Errorf("command line has an unterminated quote")
	}
	if inWord {
		argv = append(argv, word.String())
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("command line is empty")
	}
	return argv, nil
}

// populateSpec is a parsed `--populate` value: plugin:<name> or
// command:<cmdline>.
type populateSpec struct {
	plugin string
	argv   []string
}

func parsePopulateSpec(spec string) (populateSpec, error) {
	spec = strings.TrimSpace(spec)
	if line, ok := strings.CutPrefix(spec, "command:"); ok {
		argv, err := splitCommandLine(line)
		if err != nil {
			return populateSpec{}, fmt.Errorf("populate command: %w", err)
		}
		return populateSpec{argv: argv}, nil
	}
	plugin, err := parsePluginSpec(spec)
	if err != nil {
		return populateSpec{}, fmt.Errorf("populate must use plugin:<name> or command:<cmdline> (got %q)", spec)
	}
	return populateSpec{plugin: plugin}, nil
}
