package database

import "testing"

func TestFTSQueryQuotesPunctuatedTerms(t *testing.T) {
	cases := map[string]string{
		"circular":              "circular",
		"rate login":            "rate login",
		"auth*":                 "auth*",
		`"circular dependency"`: `"circular dependency"`,
		"prod-nat":              `"prod-nat"`,
		"prod-nat*":             `"prod-nat"*`,
		"10.0.0.1 OR prod-nat":  `"10.0.0.1" OR "prod-nat"`,
		"(a-b OR c)":            `("a-b" OR c)`,
		`say"hi`:                `"say""hi"`,
	}
	for in, want := range cases {
		if got := ftsQuery(in); got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFactSearchHyphenatedTerm(t *testing.T) {
	db := newFTSTestDB(t)
	nat, _ := db.AddFact(Fact{Relation: "gateway", Args: `{"name":"prod-nat","ip":"10.0.0.1"}`, Source: "x"})
	other, _ := db.AddFact(Fact{Relation: "gateway", Args: `{"name":"staging-nat","ip":"10.0.0.2"}`, Source: "x"})
	for _, query := range []string{"prod-nat", "10.0.0.1", "prod-n*"} {
		got, err := db.SearchCurrentFacts(query, "", 0)
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		if m := ids(got); !m[nat.ID] || m[other.ID] {
			t.Errorf("search %q = %v, want only %s", query, m, nat.ID[:8])
		}
	}
}
