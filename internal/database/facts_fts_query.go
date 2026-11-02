package database

import (
	"regexp"
	"strings"
)

// ftsBareTerm is a term FTS5 parses as written: word characters with an
// optional trailing prefix *.
var ftsBareTerm = regexp.MustCompile(`^[\p{L}\p{N}_]+\*?$`)

// ftsQuery makes a user query safe for FTS5 MATCH while keeping its syntax:
// "quoted phrases", AND/OR/NOT, parentheses and a trailing * pass through. A
// term with punctuation, such as prod-nat or 10.0.0.1, is quoted as a phrase;
// unquoted, FTS5 reads - as an operator and errors.
func ftsQuery(query string) string {
	var out []string
	for _, token := range ftsTokens(query) {
		out = append(out, ftsQuoteTerm(token))
	}
	return strings.Join(out, " ")
}

// ftsTokens splits on whitespace outside double-quoted phrases.
func ftsTokens(query string) []string {
	var tokens []string
	var current strings.Builder
	inPhrase := false
	for _, r := range query {
		switch {
		case r == '"':
			inPhrase = !inPhrase
			current.WriteRune(r)
		case !inPhrase && (r == ' ' || r == '\t' || r == '\n'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// ftsQuoteTerm quotes one token unless FTS5 already parses it as written.
// Leading ( and trailing ) are kept outside the quotes.
func ftsQuoteTerm(token string) string {
	core := strings.TrimLeft(token, "(")
	open := token[:len(token)-len(core)]
	trimmed := strings.TrimRight(core, ")")
	closing := core[len(trimmed):]
	core = trimmed
	switch {
	case core == "", core == "AND", core == "OR", core == "NOT", ftsBareTerm.MatchString(core):
		return token
	case len(core) >= 2 && strings.HasPrefix(core, `"`) && (strings.HasSuffix(core, `"`) || strings.HasSuffix(core, `"*`)):
		return token
	}
	prefix := ""
	if strings.HasSuffix(core, "*") {
		core, prefix = strings.TrimSuffix(core, "*"), "*"
	}
	return open + `"` + strings.ReplaceAll(core, `"`, `""`) + `"` + prefix + closing
}
