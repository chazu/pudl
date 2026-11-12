# Datalog rules and queries

`pudl query` evaluates Datalog rules over the [fact store](facts.md) and the
catalog. Model checks run the same evaluator. Rules are CUE; facts come mostly
from schema [projection](projection.md), policy results and model
dependencies.

```bash
pudl query --list                             # relations you can query, with their argument keys
pudl query gcp_address status=RESERVED       # facts, filtered
pudl query policy_noncompliant severity=high  # a built-in rule
pudl query impacted_by changed=network --json
```

## Write a rule

A rule is a top-level CUE field with a `head` (the derived fact) and a `body`
(conditions that must all hold). Arguments are named. Strings starting with
`$` are variables.

This rule joins the firewall facts declared in the
[projection example](projection.md#declaring-facts-_pudlfacts):

```cue
package rules

// Firewalls open to the internet on SSH.
open_ssh: {
	head: {rel: "open_ssh", args: {project: "$P", name: "$N"}}
	body: [
		{rel: "gcp_firewall", args: {entry_id: "$E", project: "$P", name: "$N", direction: "INGRESS", disabled: false}},
		{rel: "gcp_firewall_source", args: {entry_id: "$E", range: "0.0.0.0/0"}},
		{rel: "gcp_firewall_allow", args: {entry_id: "$E", proto: "tcp", port: "22"}},
	]
}
```

- **Variables** that appear in several body atoms must bind to the same
  value; that is the join. Every head variable must appear in the body.
- **Ground values** (`"INGRESS"`, `false`, `22`) match exactly.
- **Comparisons** in a body argument filter instead of matching exactly:
  `">0.25"`, `"<=10"`, `"!=80"` compare a number; `"<now-90d"`,
  `">=now-12h"`, `"<now"` compare a date (an RFC 3339 string) with the
  evaluation time minus days (`d`), hours (`h`) or minutes (`m`). The
  evaluation time is the `--as-of` time of a time-travel query, else the
  current time. A value that is not a date string never matches a date
  comparison. Comparisons are body-only.

  ```cue
  // User-managed keys created more than 90 days ago.
  stale_key: {
  	head: {rel: "stale_key", args: {name: "$N"}}
  	body: [{rel: "gcp_service_account_key", args: {name: "$N", valid_after: "<now-90d"}}]
  }
  ```
- **Constant head arguments** become part of every derived row and can be
  filtered on: a head `{id: "$X", severity: "high"}` matches
  `pudl query flagged severity=high`.
- **Head keys** are a relation's columns. Every rule deriving one relation must
  use the same keys.
- **Several rules with one head relation** are a union; a row derived twice is
  returned once.
- **Recursion** is allowed, including rules with several recursive atoms.
- **Aggregates** (`count($X)`, `sum`, `min`, `max`) are allowed in the head only,
  and not in a rule that is part of a cycle.
- **Argument keys** may be quoted CUE labels (`"app.kubernetes.io/name"`). A key
  is one top-level argument, never a nested path.
- **No negation.** To express "has no X", project `{exists: "X"}` or a
  `default`, then match the value (see [projection](projection.md)).

The rule's name is its field name, or its optional `name` field.

### Facts

A rule with no `body` (or `body: []`) is a fact: it holds unconditionally.
Every head argument must be a constant; a variable has nothing to bind it.

```cue
graph_edge_type_in_vpc: {
	head: {rel: "graph_edge_type", args: {type: "in_vpc", category: "containment",
		label: "in VPC", meaning: "Defined inside this VPC."}}
}
```

Facts can share a relation with derived rules, and rule bodies can join on
them. pudl's graph vocabulary is written this way (see
[Resource graph vocabulary](#resource-graph-vocabulary)). A long list of facts
reads better as a table and a comprehension:

```cue
_edgeTypes: {
	in_vpc: {category: "containment", label: "in VPC", meaning: "Defined inside this VPC."}
}
for id, e in _edgeTypes {
	"graph_edge_type_\(id)": {head: {rel: "graph_edge_type", args: {type: id, category: e.category, label: e.label, meaning: e.meaning}}}
}
```

Hidden fields such as `_edgeTypes` are shared by every file of a CUE package,
so give each file's tables distinct names.

### Recursive example

```cue
reach_base: {
	head: {rel: "reach", args: {from: "$A", to: "$B"}}
	body: [{rel: "edge", args: {from: "$A", to: "$B"}}]
}
reach_step: {
	head: {rel: "reach", args: {from: "$A", to: "$C"}}
	body: [
		{rel: "edge",  args: {from: "$A", to: "$B"}},
		{rel: "reach", args: {from: "$B", to: "$C"}},
	]
}
```

Given `edge(api, db)` and `edge(db, cache)`, `pudl query reach from=api`
returns `db` and `cache`. The built-in `depends_transitive` rule is this
pattern over `model_depends_on`.

### Invalid rules fail loudly

A rule that cannot be read as written (a missing `rel`, a variable in a
fact's head, an unbound head variable, an aggregate in the body, mismatched head keys) is
loaded as invalid, with its source position, never skipped:

- `pudl rule add` refuses the file and lists each problem;
- `pudl rule new` warns about invalid rules already installed;
- `pudl doctor` lists every invalid rule under **Datalog Rules**;
- a query or check whose relation depends on an invalid rule fails. An invalid
  rule whose head cannot be read blocks every query.

A silently dropped rule would make a check read nothing and pass.

## Where rules live

| Workspace | Rule directories |
|-----------|------------------|
| Project | `.pudl/schema/pudl/rules/`, after the rules of vendored dependencies |
| Global mode | `~/.pudl/schema/pudl/rules/` |

A rule in a later directory shadows an earlier one with the same name, so
project rules win. A model's checks also load the `rules/` directory next to
the model file. `pudl query -f FILE` adds rules from a file for one query;
`--all-workspaces` also includes global rules and every workspace's data.

```bash
pudl rule new open-ssh              # scaffold .pudl/schema/pudl/rules/open-ssh.cue
pudl rule add standards.cue         # validate a file and install it into the project
pudl rule add standards.cue --global
```

## Built-in relations

| Relation | Kind | Arguments |
|----------|------|-----------|
| `catalog_entry` | catalog view (join-only) | see below |
| `policy_status` | facts | `resource`, `policy`, `type`, `status`, `policy_digest` |
| `policy_violation` | facts | `resource`, `policy`, `rule`, `severity`, `path`, `kind`, `declared_by` |
| `policy_noncompliant` | rule | `resource`, `policy`, `severity` |
| `resource_owner` | facts | `entry_id`, `resource_id`, `type`, `owner`, `via` |
| `resource_unowned` | facts | `entry_id`, `resource_id`, `type` |
| `model_depends_on` | facts | `from`, `to` |
| `same_resource_key` | facts (projection only) | `entry_id`, `resource_id`, `kind`, `key` |
| `same_resource` | rules only | `a`, `b`, `kind` |
| `same_resource_kind` | facts in rule files | `kind`, `label`, `meaning` |
| `depends_transitive`, `impacted_by`, `cyclic` | rules | see [dependencies](cross-model-dependencies.md) |

Every projected fact also carries `entry_id` and `resource_id`.

### Resource graph edges (`graph_edge`)

`pudl graph export` derives relationships from rules whose head is
`graph_edge`, with four arguments:

| Argument | Value |
| --- | --- |
| `type` | Relationship type, declared by a `graph_edge_type` fact ([vocabulary](#resource-graph-vocabulary)): `in_subnet`, `protected_by`, `encrypted_by`, ... An undeclared type renders as "other" and is reported |
| `source`, `target` | A `resource_id`, or a reference value such as an AWS ID or a GCP path |
| `basis` | `observed`, `declared`, `inferred` or `unknown` |

```cue
aws_graph_instance_sg: {
	head: {rel: "graph_edge", args: {type: "protected_by", source: "$R", target: "$G", basis: "observed"}}
	body: [{rel: "aws_instance_sg", args: {resource_id: "$R", group_id: "$G"}}]
}
```

A reference resolves through each resource's identity value and its
`_pudl.graph.refs`. If nothing matches, the edge points at `ref:<value>`, and
the explorer shows that resource as not in the inventory. A reference that
matches several resources is left unresolved and reported. A row whose
target is null (an absent optional field) is no edge. Each rule is evaluated
separately, so every relationship names the rule that derived it. The AWS and
GCP packs ship their edges in `schema/pudl/rules/*_graph.cue`.

Two kinds of value are not resources, so they become **external** nodes
rather than "not in inventory" references:

- an address range (anything that parses as a CIDR, such as `0.0.0.0/0`)
  becomes an `external.cidr` node;
- `*` becomes the `external.principal` node "Anyone".

Edges can carry details through `graph_edge_property(type, source, target,
key, value)`, with `type`, `source` and `target` naming the edge as in its
`graph_edge` row. Values for one key collect into a sorted list, so a
security group that allows ports 80 and 443 from the same range is one edge
with `port: [80, 443]`. A property row whose edge no rule derived is
reported.

```cue
aws_graph_sg_cidr_port: {
	head: {rel: "graph_edge_property", args: {type: "allows_ingress_from", source: "$R", target: "$C", key: "port", value: "$F"}}
	body: [{rel: "aws_sg_ingress_cidr", args: {resource_id: "$R", cidr: "$C", from_port: "$F"}}]
}
```

### Resource inspector details (`graph_detail`)

`graph_detail(resource_id, section, key, value, rank?)` adds rows to a
resource's inspector in pudl ui, under a heading named by `section`. A
`resource_id` that is a grouped member ([`same_resource`](#shared-identity-same_resource))
resolves to its group, so a record that draws no node of its own
(`_pudl.graph.member_only`) can still describe the node it belongs to.
Rows sort by section, key, `rank` and value; several values of one key with
ranks are shown as a path. A row naming no resource in the graph is reported.

```cue
pulumi_detail_stack: {
	head: {rel: "graph_detail", args: {resource_id: "$R", section: "Pulumi", key: "stack", value: "$S"}}
	body: [{rel: "pulumi_resource", args: {resource_id: "$R", stack: "$S"}}]
}
```

### Resource graph vocabulary

pudl ui knows what a type or edge type means only from these relations,
declared as [facts](#facts) in rule files. `pudl graph export` sends them as
the graph's `vocabulary`, and `pudl graph vocab` lists them with their
problems. pudl's own are in `pudl/rules/graph_vocabulary.cue`.

| Relation | Arguments | Meaning |
| --- | --- | --- |
| `graph_edge_category` | `category, label, meaning, order` | The fixed categories pudl ui styles; pudl built-ins only |
| `graph_edge_type` | `type, category, label, meaning` | What an edge type `graph_edge` rules derive means |
| `graph_provider` | `provider, label` | A provider's label (the first segment of a resource type) |
| `graph_account_type` | `provider, type` | The type a provider's `account` values name |
| `graph_resource_type` | `type, code, label, class` | Display info for a type with no schema |

These relations take facts only. The first declaration of a key wins, and a
later one that disagrees is reported. A resource type's display info and view
roles normally live in its schema; see
[schema authoring](schema-authoring.md#make-a-type-appear-in-pudl-ui).

### Shared identity (`same_resource`)

`same_resource(a, b, kind)` says resources `a` and `b` (resource IDs) are
two records of one real resource. Schemas declare keys in
[`_pudl.same_resource`](schema-authoring.md#same_resource); projection writes
each as `same_resource_key(kind, key, resource_id)`, where `key` is the
canonical JSON of the key's parts, and the built-in rule
`same_resource_by_key` links resources holding an equal key. The relation
also has `a = b` rows, because rules cannot say "different"; consumers drop
them.

A pack writes its own `same_resource` rules when no field of the record is a
shared key, and the link needs a join. Rules may read `same_resource`
itself, so a link can build on another link. For example, objects of two
sources that only share a name could be linked through their linked
clusters (the relations here are illustrative):

```cue
same_object_via_cluster: {
	head: {rel: "same_resource", args: {a: "$A", b: "$B", kind: "k8s.object"}}
	body: [
		{rel: "a_object", args: {resource_id: "$A", cluster: "$CA", namespace: "$NS", name: "$N"}},
		{rel: "same_resource", args: {a: "$CA", b: "$CB", kind: "k8s.cluster"}},
		{rel: "b_object", args: {resource_id: "$B", cluster: "$CB", namespace: "$NS", name: "$N"}},
	]
}
```

A relation with no facts matches nothing, so such a rule is harmless until
both packs are installed. Prefer a key in `_pudl.same_resource` when one
exists: Kubernetes objects key on `metadata.uid`.

Kinds are `same_resource_kind` facts. pudl ships `k8s.cluster` and
`k8s.object`; a pack declares its own the same way:

```cue
same_resource_kind_aws_account: {
	head: {rel: "same_resource_kind", args: {kind: "aws.account", label: "AWS account",
		meaning: "An AWS account, keyed by its account ID."}}
}
```

`same_resource` and `same_resource_key` are reserved: `pudl facts add`
rejects them. `pudl doctor` ("Shared Identity") reports kinds no fact
declares and records that were not grouped because two of one schema family
share a key.

### Catalog (`catalog_entry`)

The catalog is the built-in relation `catalog_entry`, read from native
columns:

| Field | Value |
|-------|-------|
| `id` | Entry ID |
| `schema` | Assigned schema |
| `origin` | Import origin or workspace |
| `format` | File format |
| `status` | Model status: `unknown`, `clean`, `drifted`, `failed` |
| `entry_type` | `import`, `observe` (older catalogs also hold legacy types) |
| `target` | The model target that observed it, e.g. `//models/<name>:populate` |
| `run_id` | The run that observed it |
| `resource_id` | Stable resource identity |
| `content_hash` | SHA256 of the stored data |
| `version` | Version number within its `resource_id` |
| `collection_id`, `collection_type`, `item_id` | Collection membership |

An argument that is not one of its columns is a load error naming the rule
and the valid columns.

Join it to facts through `entry_id`:

```cue
firewall_origin: {
	head: {rel: "firewall_origin", args: {name: "$N", origin: "$O"}}
	body: [
		{rel: "gcp_firewall",  args: {entry_id: "$E", name: "$N"}},
		{rel: "catalog_entry", args: {id: "$E", origin: "$O"}},
	]
}
```

`catalog_entry` is join-only: `pudl query catalog_entry` is an error. List
entries with `pudl list`, or `Store.ListCatalog` in Go. The name is reserved;
facts cannot be written under it.

## Query

```bash
pudl query RELATION [field=value ...]
```

- **Constraints** are `field=value`. Booleans and numbers are typed:
  `disabled=false`, `n=9007199254740993`. Keep JSON quotes to match a string
  that looks like a number: `'id="123"'`. Repeating a field is an error.
- **`--list`** prints derived relations with their argument keys, stored fact
  relations, and built-ins.
- **`--topo`** reads a relation's `from`/`to` rows as a run order,
  dependencies first, and errors on a cycle.
- **`--timeout`** bounds the query; a cancelled query returns an error, never
  a partial answer.
- **`--max-iterations`** caps the rounds of each recursive cycle (default 100),
  which bounds the longest chain a recursive rule can follow. Model checks
  use the default.
- **`--json`** prints `[{"relation": ..., "args": {...}}]`.

`pudl query` writes nothing. It warns when projected facts are out of date;
`pudl doctor --fix --only projections` syncs them.

### Numbers

Queries keep signed 64-bit integers exact, including values above 2^53.
Equivalent spellings compare equal (`9007199254740993` and
`9.007199254740993e15`). A non-integral decimal must survive a `float64`
round trip. Any other number fails with `unsupported query number` rather
than being rounded; the raw value is still readable with `pudl facts list`.
See the [numeric contract](library-api.md#numeric-query-contract).

## Time travel

By default rules see current facts: valid now and not retracted. Two flags
move the whole evaluation in time:

```bash
# What was true on April 15, by what we know now?
pudl query depends_transitive --as-of-valid 2026-04-15T00:00:00Z

# What did the store believe at that moment, including facts retracted since?
pudl query depends_transitive --as-of-tx 2026-04-15T00:00:00Z

# What did we believe on May 1 about April 15?
pudl query depends_transitive --as-of-valid 2026-04-15T00:00:00Z --as-of-tx 2026-05-01T00:00:00Z
```

Times are RFC3339 or Unix seconds. Every atom of every rule sees the same
moment, so joins and recursion describe one consistent past state.
`pudl facts list` takes the same flags without evaluating rules. See
[facts](facts.md#time-travel) for the two time axes.

## How evaluation works

Rules compile to parameterized SQL that runs inside SQLite. Each body atom is
a join over the current-facts table (or the full history table for time
travel), arguments are read from JSON, and shared variables become join
conditions.

A query evaluates only the rules the queried relation depends on. Without
recursion the whole query is one SQL statement, so a passing check (an empty
result) is cheap. Recursive cycles run as semi-naive fixpoints in temporary
tables until no new rows appear. Joins on fact arguments are not indexed, so
large joins over many facts dominate cost.

The compiler reads arguments with SQLite's `->` operator and the
`pudl_query_value` function, which pudl registers on its own connections.
Other SQLite clients reading pudl's query views need that function.
