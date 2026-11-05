package projection

import (
	"fmt"
	"regexp"
)

// RewriteRule maps a string value matching Match to Replace, a template in
// which $1, ${name} and so on stand for Match's submatches (Go
// regexp.Expand syntax; write ${1} when a letter or digit follows).
type RewriteRule struct {
	Match   *regexp.Regexp
	Replace string
}

// rawRewrite is the JSON shape of one rule: {match: RE2, replace: template}.
type rawRewrite struct {
	Match   string `json:"match"`
	Replace string `json:"replace"`
}

func parseRewrite(raw []rawRewrite) ([]RewriteRule, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("needs at least one {match, replace} rule")
	}
	rules := make([]RewriteRule, 0, len(raw))
	for i, r := range raw {
		if r.Match == "" {
			return nil, fmt.Errorf("rule %d: match is required", i)
		}
		re, err := regexp.Compile(r.Match)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		rules = append(rules, RewriteRule{Match: re, Replace: r.Replace})
	}
	return rules, nil
}

// rewrite applies the first matching rule. ok is false when the arg has rules
// and none matches v (or v is not a string): the value yields no fact. An
// absent value without a default yields no fact either (rowArgs).
func (a ArgSpec) rewrite(v any) (any, bool) {
	if a.Rewrite == nil {
		return v, true
	}
	s, isString := v.(string)
	if !isString {
		return nil, false
	}
	for _, rule := range a.Rewrite {
		if m := rule.Match.FindStringSubmatchIndex(s); m != nil {
			return string(rule.Match.ExpandString(nil, rule.Replace, s, m)), true
		}
	}
	return nil, false
}
