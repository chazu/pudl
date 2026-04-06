package datalog

import (
	"sort"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Evaluator tests ---

func TestTransitiveClosure(t *testing.T) {
	// Classic Datalog: transitive closure of a dependency graph
	//   depends(api, db)
	//   depends(db, cache)
	//   depends(cache, redis)
	//   depends_transitive(X, Z) :- depends(X, Z)
	//   depends_transitive(X, Z) :- depends(X, Y), depends_transitive(Y, Z)

	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "depends", Args: map[string]interface{}{"from": "api", "to": "db"}})
	edb.Add(Tuple{Relation: "depends", Args: map[string]interface{}{"from": "db", "to": "cache"}})
	edb.Add(Tuple{Relation: "depends", Args: map[string]interface{}{"from": "cache", "to": "redis"}})

	rules := []Rule{
		{
			Name: "base",
			Head: Atom{Rel: "depends_transitive", Args: map[string]Term{"from": Var("X"), "to": Var("Z")}},
			Body: []Atom{
				{Rel: "depends", Args: map[string]Term{"from": Var("X"), "to": Var("Z")}},
			},
		},
		{
			Name: "recursive",
			Head: Atom{Rel: "depends_transitive", Args: map[string]Term{"from": Var("X"), "to": Var("Z")}},
			Body: []Atom{
				{Rel: "depends", Args: map[string]Term{"from": Var("X"), "to": Var("Y")}},
				{Rel: "depends_transitive", Args: map[string]Term{"from": Var("Y"), "to": Var("Z")}},
			},
		},
	}

	eval := NewEvaluator(rules, edb)
	results, err := eval.Evaluate()
	require.NoError(t, err)

	// Should derive 6 transitive deps:
	// api->db, api->cache, api->redis, db->cache, db->redis, cache->redis
	assert.Len(t, results, 6)

	// Query with constraint
	apiDeps, err := eval.Query("depends_transitive", map[string]interface{}{"from": "api"})
	require.NoError(t, err)
	assert.Len(t, apiDeps, 3) // db, cache, redis
}

func TestGroundTermFilter(t *testing.T) {
	// Rule with a ground term in the body: only match specific values
	//   observation(kind=obstacle, scope=X) => at_risk(scope=X)

	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "observation", Args: map[string]interface{}{"kind": "obstacle", "scope": "pkg/auth"}})
	edb.Add(Tuple{Relation: "observation", Args: map[string]interface{}{"kind": "pattern", "scope": "pkg/api"}})
	edb.Add(Tuple{Relation: "observation", Args: map[string]interface{}{"kind": "obstacle", "scope": "pkg/db"}})

	rules := []Rule{
		{
			Name: "obstacles_at_risk",
			Head: Atom{Rel: "at_risk", Args: map[string]Term{"scope": Var("X")}},
			Body: []Atom{
				{Rel: "observation", Args: map[string]Term{"kind": Val("obstacle"), "scope": Var("X")}},
			},
		},
	}

	eval := NewEvaluator(rules, edb)
	results, err := eval.Evaluate()
	require.NoError(t, err)
	assert.Len(t, results, 2) // pkg/auth and pkg/db, not pkg/api
}

func TestJoinAcrossRelations(t *testing.T) {
	// Join: observation + catalog_entry on scope/origin
	//   flagged_origin(origin=O) :- observation(kind=obstacle, scope=S), catalog_entry(origin=O, schema=S)

	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "observation", Args: map[string]interface{}{"kind": "obstacle", "scope": "aws"}})
	edb.Add(Tuple{Relation: "observation", Args: map[string]interface{}{"kind": "obstacle", "scope": "k8s"}})
	edb.Add(Tuple{Relation: "catalog_entry", Args: map[string]interface{}{"origin": "prod-aws", "schema": "aws"}})
	edb.Add(Tuple{Relation: "catalog_entry", Args: map[string]interface{}{"origin": "prod-k8s", "schema": "k8s"}})
	edb.Add(Tuple{Relation: "catalog_entry", Args: map[string]interface{}{"origin": "staging", "schema": "gcp"}})

	rules := []Rule{
		{
			Name: "flagged",
			Head: Atom{Rel: "flagged_origin", Args: map[string]Term{"origin": Var("O")}},
			Body: []Atom{
				{Rel: "observation", Args: map[string]Term{"kind": Val("obstacle"), "scope": Var("S")}},
				{Rel: "catalog_entry", Args: map[string]Term{"origin": Var("O"), "schema": Var("S")}},
			},
		},
	}

	eval := NewEvaluator(rules, edb)
	results, err := eval.Evaluate()
	require.NoError(t, err)
	assert.Len(t, results, 2)

	origins := make([]string, 0)
	for _, r := range results {
		origins = append(origins, r.Args["origin"].(string))
	}
	sort.Strings(origins)
	assert.Equal(t, []string{"prod-aws", "prod-k8s"}, origins)
}

func TestNoRules(t *testing.T) {
	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "fact", Args: map[string]interface{}{"x": "y"}})

	eval := NewEvaluator(nil, edb)
	results, err := eval.Evaluate()
	require.NoError(t, err)
	assert.Len(t, results, 0) // no rules = no derived facts
}

func TestEmptyEDB(t *testing.T) {
	rules := []Rule{
		{
			Name: "derive",
			Head: Atom{Rel: "derived", Args: map[string]Term{"x": Var("X")}},
			Body: []Atom{
				{Rel: "base", Args: map[string]Term{"x": Var("X")}},
			},
		},
	}

	eval := NewEvaluator(rules, NewMemoryEDB())
	results, err := eval.Evaluate()
	require.NoError(t, err)
	assert.Len(t, results, 0) // no base facts = no derivations
}

func TestFixedPointTerminates(t *testing.T) {
	// Self-referential rule should reach fixed point, not loop forever
	//   reachable(X) :- edge(X, Y), reachable(Y)
	//   reachable(X) :- start(X)

	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "start", Args: map[string]interface{}{"node": "a"}})
	edb.Add(Tuple{Relation: "edge", Args: map[string]interface{}{"from": "a", "to": "b"}})
	edb.Add(Tuple{Relation: "edge", Args: map[string]interface{}{"from": "b", "to": "c"}})
	edb.Add(Tuple{Relation: "edge", Args: map[string]interface{}{"from": "c", "to": "a"}}) // cycle!

	rules := []Rule{
		{
			Name: "base_reach",
			Head: Atom{Rel: "reachable", Args: map[string]Term{"node": Var("X")}},
			Body: []Atom{
				{Rel: "start", Args: map[string]Term{"node": Var("X")}},
			},
		},
		{
			Name: "step_reach",
			Head: Atom{Rel: "reachable", Args: map[string]Term{"node": Var("Y")}},
			Body: []Atom{
				{Rel: "edge", Args: map[string]Term{"from": Var("X"), "to": Var("Y")}},
				{Rel: "reachable", Args: map[string]Term{"node": Var("X")}},
			},
		},
	}

	eval := NewEvaluator(rules, edb)
	results, err := eval.Evaluate()
	require.NoError(t, err)
	assert.Len(t, results, 3) // a, b, c all reachable
}

func TestQueryWithConstraints(t *testing.T) {
	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "person", Args: map[string]interface{}{"name": "alice", "role": "admin"}})
	edb.Add(Tuple{Relation: "person", Args: map[string]interface{}{"name": "bob", "role": "user"}})
	edb.Add(Tuple{Relation: "person", Args: map[string]interface{}{"name": "carol", "role": "admin"}})

	eval := NewEvaluator(nil, edb)
	admins, err := eval.Query("person", map[string]interface{}{"role": "admin"})
	require.NoError(t, err)
	assert.Len(t, admins, 2)
}

func TestMultiEDB(t *testing.T) {
	edb1 := NewMemoryEDB()
	edb1.Add(Tuple{Relation: "fact", Args: map[string]interface{}{"x": "from_edb1"}})

	edb2 := NewMemoryEDB()
	edb2.Add(Tuple{Relation: "fact", Args: map[string]interface{}{"x": "from_edb2"}})

	multi := NewMultiEDB(edb1, edb2)
	tuples, err := multi.Scan("fact")
	require.NoError(t, err)
	assert.Len(t, tuples, 2)
}

// --- CUE loader tests ---

func TestParseCUERule(t *testing.T) {
	ctx := cuecontext.New()

	source := `
transitiveDep: {
	name: "transitive_dep"
	head: { rel: "depends_transitive", args: { from: "$X", to: "$Z" } }
	body: [
		{ rel: "depends", args: { from: "$X", to: "$Y" } },
		{ rel: "depends_transitive", args: { from: "$Y", to: "$Z" } },
	]
}
`
	rules, err := ParseRules(ctx, source)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	r := rules[0]
	assert.Equal(t, "transitive_dep", r.Name)
	assert.Equal(t, "depends_transitive", r.Head.Rel)
	assert.True(t, r.Head.Args["from"].IsVariable())
	assert.Equal(t, "$X", r.Head.Args["from"].Variable)
	assert.Len(t, r.Body, 2)
	assert.Equal(t, "depends", r.Body[0].Rel)
}

func TestParseCUEMultipleRules(t *testing.T) {
	ctx := cuecontext.New()

	source := `
rule1: {
	head: { rel: "derived", args: { x: "$X" } }
	body: [{ rel: "base", args: { x: "$X" } }]
}
rule2: {
	head: { rel: "other", args: { y: "$Y" } }
	body: [{ rel: "source", args: { y: "$Y" } }]
}
`
	rules, err := ParseRules(ctx, source)
	require.NoError(t, err)
	assert.Len(t, rules, 2)
}

func TestParseCUERuleWithGroundTerms(t *testing.T) {
	ctx := cuecontext.New()

	source := `
obstacleRule: {
	head: { rel: "at_risk", args: { scope: "$S" } }
	body: [{ rel: "observation", args: { kind: "obstacle", scope: "$S" } }]
}
`
	rules, err := ParseRules(ctx, source)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	bodyAtom := rules[0].Body[0]
	assert.False(t, bodyAtom.Args["kind"].IsVariable())
	assert.Equal(t, "obstacle", bodyAtom.Args["kind"].Value)
	assert.True(t, bodyAtom.Args["scope"].IsVariable())
}

func TestParseCUENonRuleFieldsSkipped(t *testing.T) {
	ctx := cuecontext.New()

	source := `
someConfig: "not a rule"
aNumber: 42
actualRule: {
	head: { rel: "derived", args: { x: "$X" } }
	body: [{ rel: "base", args: { x: "$X" } }]
}
`
	rules, err := ParseRules(ctx, source)
	require.NoError(t, err)
	assert.Len(t, rules, 1)
	assert.Equal(t, "derived", rules[0].Head.Rel)
}

// --- Integration: CUE-loaded rules + evaluator ---

func TestCUERulesToEvaluator(t *testing.T) {
	ctx := cuecontext.New()

	source := `
baseDep: {
	head: { rel: "depends_transitive", args: { from: "$X", to: "$Z" } }
	body: [{ rel: "depends", args: { from: "$X", to: "$Z" } }]
}
recursiveDep: {
	head: { rel: "depends_transitive", args: { from: "$X", to: "$Z" } }
	body: [
		{ rel: "depends", args: { from: "$X", to: "$Y" } },
		{ rel: "depends_transitive", args: { from: "$Y", to: "$Z" } },
	]
}
`
	rules, err := ParseRules(ctx, source)
	require.NoError(t, err)

	edb := NewMemoryEDB()
	edb.Add(Tuple{Relation: "depends", Args: map[string]interface{}{"from": "api", "to": "db"}})
	edb.Add(Tuple{Relation: "depends", Args: map[string]interface{}{"from": "db", "to": "cache"}})

	eval := NewEvaluator(rules, edb)
	results, err := eval.Query("depends_transitive", map[string]interface{}{"from": "api"})
	require.NoError(t, err)
	assert.Len(t, results, 2) // api->db, api->cache
}

// --- Type tests ---

func TestTupleKeyDeterministic(t *testing.T) {
	t1 := Tuple{Relation: "r", Args: map[string]interface{}{"a": "1", "b": "2"}}
	t2 := Tuple{Relation: "r", Args: map[string]interface{}{"b": "2", "a": "1"}}
	assert.Equal(t, t1.Key(), t2.Key(), "key order should not matter")

	t3 := Tuple{Relation: "r", Args: map[string]interface{}{"a": "1", "b": "3"}}
	assert.NotEqual(t, t1.Key(), t3.Key(), "different values should differ")
}

func TestBindingApply(t *testing.T) {
	b := Binding{"$X": "api", "$Y": "db"}
	atom := Atom{
		Rel:  "depends",
		Args: map[string]Term{"from": Var("X"), "to": Var("Y")},
	}

	tuple, err := b.Apply(atom)
	require.NoError(t, err)
	assert.Equal(t, "depends", tuple.Relation)
	assert.Equal(t, "api", tuple.Args["from"])
	assert.Equal(t, "db", tuple.Args["to"])
}

func TestBindingApplyUnbound(t *testing.T) {
	b := Binding{"$X": "api"}
	atom := Atom{
		Rel:  "depends",
		Args: map[string]Term{"from": Var("X"), "to": Var("Y")},
	}

	_, err := b.Apply(atom)
	assert.Error(t, err, "should fail on unbound $Y")
}
