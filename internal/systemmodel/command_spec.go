package systemmodel

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// SplitCommandLine splits a command line into argv the way a POSIX shell
// would, without any expansion: whitespace separates words, single quotes
// keep everything literally, double quotes keep everything except \" \\ \$
// and \` escapes, and a backslash outside quotes escapes the next character.
// It is how `--populate command:<cmdline>` becomes a #CommandRun argv, which
// pudl then runs directly — no shell, so nothing in the line is interpreted.
func SplitCommandLine(line string) ([]string, error) {
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

// ParsePopulateSpec parses a `--populate command:<cmdline>` value into the
// argv of one #CommandRun.
func ParsePopulateSpec(spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if line, ok := strings.CutPrefix(spec, "command:"); ok {
		argv, err := SplitCommandLine(line)
		if err != nil {
			return nil, fmt.Errorf("populate command: %w", err)
		}
		return argv, nil
	}
	if strings.HasPrefix(spec, "plugin:") {
		return nil, fmt.Errorf("--populate plugin:<name> was retired with mu observers; run the program as command:<cmdline> (see docs/retired-commands.md)")
	}
	return nil, fmt.Errorf("populate must use command:<cmdline> (got %q)", spec)
}

var nonIdentifier = regexp.MustCompile(`[^[:alnum:]]+`)

// AdHocModel constructs the same model shape a registered model has from a
// `--populate command:<cmdline>` spec. The model is intentionally not written
// to the schema repository; only its run and snapshot artifacts are durable.
func AdHocModel(spec string) (*SystemModel, error) {
	argv, err := ParsePopulateSpec(spec)
	if err != nil {
		return nil, err
	}
	name := "adhoc-" + strings.ToLower(nonIdentifier.ReplaceAllString(filepath.Base(argv[0]), "-"))
	return &SystemModel{
		Name:     name,
		Populate: Populate{Runs: []CommandRun{{Argv: argv}}},
	}, nil
}
