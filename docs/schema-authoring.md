# Schema authoring

pudl schemas are [CUE](https://cuelang.org/) definitions. There are two kinds:

- A **type** says what a resource *is*: its shape, identity, owner, the facts
  it projects and the fields that must never be stored. Classification assigns
  exactly one type to every record.
- A **policy** says what a *good* instance of a type looks like. It is never
  assigned. pudl evaluates every applicable policy against every resource and
  stores the results. See [Compliance Policies](#compliance-policies).

## A type schema

```cue
package aws

#Instance: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.instance"
		identity_fields: ["InstanceId"]
		review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
		owner: {paths: ["OwnerTag"], fallback: "platform"}
		sensitive_fields: ["UserData"]
		facts: aws_instance: args: {id: "InstanceId", state: "State.Name"}
	}

	InstanceId:   string
	InstanceType: string
	State: {Name: "pending" | "running" | "stopping" | "stopped" | "terminated", ...}
	Tags?: [...{Key: string, Value: string}]
	...  // accept fields not listed
}
```

The `_pudl` block drives pudl; the rest is ordinary CUE that a record must
unify with.

## The `_pudl` block

| Key | Purpose |
|-----|---------|
| `schema_type` | The schema's role: `base`, `custom`, `policy`, `catchall` or `collection` |
| `resource_type` | Dotted resource kind, such as `gcp.compute.firewall`; helps inference |
| `identity_fields` | Fields that identify one resource across observations |
| `base_schema` | The parent schema: a more general type, or a policy's type |
| `review` | Whether someone has validated the type as complete |
| `owner` | How to derive a resource's owning team |
| `facts` | Relations the records project into the fact store |
| `sensitive_fields` | Paths redacted before anything is stored |
| `secret_scan_ignore` | Paths the likely-secret scan does not report |
| `graph` | How records are labelled in the resource graph, and what refers to them |
| `description` | A free-text note; pudl does not read it |

`review`, `owner`, `facts`, `sensitive_fields`, `secret_scan_ignore` and
`graph` are read strictly: a mistake is a schema load error that `pudl doctor` reports, and
the feature fails closed (redaction refuses to store, projection is disabled)
rather than being silently ignored.

### `schema_type`

| Value | Use |
|-------|-----|
| `"base"` | A resource type. Most schemas |
| `"custom"` | A type outside the built-in hierarchy |
| `"policy"` | A compliance policy; never assigned |
| `"catchall"` | The universal fallback, `pudl/core.#Item` |
| `"collection"` | A collection container, such as `pudl/core.#Collection` |

### `resource_type`

A dotted name for the kind of resource, such as `aws.ec2.instance` or
`k8s.pod`. When two or more of its keywords match an import's origin (the
file name `aws-ec2-instances.json` gives `aws`, `ec2`), the schema's inference
score rises.

### `identity_fields`

The fields whose values identify one logical resource. pudl derives the
**resource ID** from them, so the same resource is tracked across observations
and its versions form a history. They also weigh most in inference.

```cue
identity_fields: ["InstanceId"]                           // EC2 instance
identity_fields: ["metadata.namespace", "metadata.name"]  // Kubernetes object
identity_fields: ["project", "name"]                      // composite
```

Paths nest with dots; quote a key that contains dots:
`metadata.labels."cloud.googleapis.com/location"`. Wildcards are not allowed.
A record whose identity fields do not resolve falls back to content-hash
identity: it forms no version history and projects no facts. `pudl import`
and `--dry-run` report such records. Choose globally unique fields where you
can: selfLinks, ARNs, UUIDs. See [Resource identity](#resource-identity).

### `base_schema`

The parent schema, by name (`"pudl/aws.#SecurityGroup"`). A type that names a
more general type specializes it: inference tries the more specific schema
first, and the child inherits the parent's `owner`, `facts`,
`sensitive_fields` and `secret_scan_ignore`, as well as policies on the
parent. A policy names its type, or a policy it extends, here. A
`base_schema` cycle is a load error.

### `review`

```cue
review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
```

`status` is `draft` or `validated`. Absent means `draft`. `validated` promises
that the type's required fields are always collected, so a policy treats a
missing required field as a violation instead of `unknown`. Results against a
draft type are marked provisional. `pudl schema new` writes `status: "draft"`;
`pudl schema list`, `schema show` and `policy list` display it.

### `owner`

How pudl names a resource's owning team. pudl tries, in order:

1. `paths`: field paths, such as labels or tags; the first that holds a
   non-empty string is the owner.
2. `map`: a table from one field's value to an owner, such as project to team.
3. `fallback`: the owner of everything else.

A resource none of them names is **unowned**.

```cue
_projectOwners: {"shop-prod": "payments", "data-prod": "data"} // once per package

#Bucket: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.storage.bucket"
		identity_fields: ["name"]
		owner: {
			paths: ["labels.team", "labels.owner"]
			map: {path: "project", owners: _projectOwners}
			fallback: "platform"  // omit to report unowned resources
		}
	}
	...
}
```

- Declare a mapping once as a hidden package value and reference it from each
  type, as above. Changing it re-derives every owner on the next import,
  check or `pudl doctor --fix --only projections`.
- The nearest schema in the `base_schema` chain that declares `owner` wins,
  whole.
- `owner` needs `identity_fields`. Unknown keys, paths with `[*]`, empty owners
  and a block with none of the three keys are load errors.
- A path names one value. To own AWS resources by tag, have the collector
  print the tag as a field first (see the
  [AWS pack](../examples/aws/README.md#ownership)).

Owners are projected as `resource_owner` and `resource_unowned` facts
([projection](projection.md#resource-ownership)) and attributed to check
findings ([governance](governance.md#attribute-findings-to-owners)).

### `facts`

Relations a type's records project into the fact store, so rules, queries and
checks can use their fields:

```cue
facts: {
	gcp_firewall: args: {
		project: "project", name: "name"
		disabled: {path: "disabled", default: false}
	}
	gcp_firewall_allow: {
		each: "allowed[*]"  // one fact per element
		args: {proto: "IPProtocol", port: {path: "ports[*]", default: "*"}}
	}
}
```

`facts` needs `identity_fields`. Argument forms, wildcards and how facts stay
current are in [projection](projection.md).

### `sensitive_fields`

```cue
sensitive_fields: ["spec.template.spec.containers[*].env[*].value"]
```

Values at these paths are replaced with `"[REDACTED]"` before a record is
hashed, identified, stored or projected. Redaction fails closed: an invalid
list, an overlap with an identity field, or a YAML or CSV record routed to
such a schema is not stored. Payloads stored before a path was declared are
rewritten only by `pudl doctor --fix --only sensitive-scrub`. See
[projection](projection.md#sensitive-fields-_pudlsensitive_fields).

### `secret_scan_ignore`

```cue
secret_scan_ignore: ["spec.nonce", "status.conditions[*].token"]
```

pudl warns about likely secrets (cloud keys, tokens, private keys, URLs with
credentials, long random strings) outside `sensitive_fields`, by path and
kind, never by value. List a path here when its value is not a secret; it
covers everything below it. To redact a real secret, add it to
`sensitive_fields` instead. See
[projection](projection.md#likely-secret-warnings-_pudlsecret_scan_ignore).

### `graph`

```cue
graph: {
	name:    "name"
	account: "project"
	region:  {path: "region", last_segment: true}
	refs: [{path: "selfLink", trim_prefix: "https://www.googleapis.com/compute/v1/"}]
}
```

`pudl graph export` reads this block to label each resource and resolve
relationships to it. Every key is optional. A value is a field path, or
`{path, trim_prefix?, last_segment?, prepend?}` (`prepend` adds text after
trimming, e.g. `serviceAccount:` to match how IAM bindings name a service
account). Wildcards are not allowed.

| Key | Meaning |
| --- | --- |
| `name` | Display name; defaults to a top-level `name`/`Name` field, then the identity values |
| `account` | Owning AWS account or GCP project |
| `region`, `zone` | Location |
| `refs` | Values other records use to refer to this resource, in addition to a single identity field's value |
| `display` | `{code, label, class}`: how pudl ui draws the type |
| `admin`, `network`, `port_of`, `security`, `inventory`, `application` | The type's roles in pudl ui's views |

`display` and the roles are covered in
[Make a type appear in pudl ui](#make-a-type-appear-in-pudl-ui).

`refs` matters when a type's identity and the references other records hold
are spelled differently. A GCP network's identity is its full `selfLink`, but
a subnet's `network` fact holds the path with the API prefix trimmed.
Relationships are Datalog rules for the `graph_edge` relation; see
[datalog](datalog.md#resource-graph-edges-graph_edge). The block is inherited
along `base_schema`, and the nearest one wins.

## Make a type appear in pudl ui

pudl ui has no type knowledge of its own. Everything it shows about a type
or a relationship comes from CUE, and travels with the graph as its
`vocabulary` (`pudl graph vocab` prints it). A type with nothing declared
still appears, drawn as unknown. Four steps give it a code, a label, a place
in each view and labelled edges, with no TypeScript.

### 1. Display info

In the schema's `_pudl: graph` block:

```cue
graph: display: {code: "SUBN", label: "Subnetwork", class: "network"}
```

- `code`: 1 to 5 characters, drawn on the node.
- `label`: the type's name in the sidebar, legends and inspector.
- `class`: one of `admin`, `network`, `compute`, `data`, `messaging`, `edge`,
  `security`, `identity`, `logical`, `external`. It sets the node's colour.

`pudl schema new` writes this line with guesses from the definition name.

### 2. Roles in the views

All optional. Without roles a type still shows in the inventory view, under
its owner, region and type.

```cue
graph: network: {group: true, via: ["in_network"], region: true}
graph: security: control: true
```

| Key | Effect |
| --- | --- |
| `network: {}` | Shown in the network view, placed under its owner |
| `network: via: [types]` | Placed under the target of the first of these edge types the resource has; under its owner when it has none |
| `network: region: true` | The parent is wrapped in a region group when the resource has a region |
| `network: group: true` | Drawn as a container that other resources sit in |
| `port_of: "type"` | Drawn as a port on the resource this edge type points to (network and security views) |
| `security: control: true` | Always shown in the security view, even with no security edges |
| `inventory: collapsed: true` | Its type bucket starts collapsed in the inventory view |
| `admin: parent: [types]` | An administrative node (project, folder, organization): a group in the network and inventory views, under the target of these edges, or at the top when there are none |
| `application: {group: true, via: [types]}` | A group in the application view, placed under the target of these edges |

Roles name edge types, so they must match the `type` of your `graph_edge`
rules. The GCP and AWS packs (`examples/*/schema/`) declare roles for every
type they ship.

### 3. Edges and their meaning

Edges are `graph_edge` rules ([datalog](datalog.md#resource-graph-edges-graph_edge)).
Each edge type the rules derive needs a `graph_edge_type` fact, in any rule
file, saying what the edge means:

```cue
graph_edge_type_in_vpc: {
	head: {rel: "graph_edge_type", args: {type: "in_vpc", category: "containment",
		label: "in VPC", meaning: "Defined inside this VPC."}}
}
```

`category` is one of pudl's built-in categories: `containment`, `network`,
`traffic`, `dependency`, `security`, `iam`, `other`. It sets the edge's
colour and line, which views draw it, and whether it places resources:
neighbourhood loading follows `containment` edges. `meaning` is the strongest
claim the edge supports; the inspector shows it. pudl ships the edge types
every provider shares (`in_subnet`, `runs_as`, `allows_ingress_from`, ...) in
`pudl/rules/graph_vocabulary.cue`; run `pudl graph vocab` to see them before
declaring your own.

### 4. Provider and account type

A pack for a new provider adds, in a rule file:

```cue
graph_provider_acme: {head: {rel: "graph_provider", args: {provider: "acme", label: "Acme Cloud"}}}
graph_account_type_acme: {head: {rel: "graph_account_type", args: {provider: "acme", type: "acme.account"}}}
graph_resource_type_acme_account: {head: {rel: "graph_resource_type", args: {type: "acme.account", code: "ACCT", label: "Acme account", class: "admin"}}}
```

- `graph_provider`: the provider's label. Without it, pudl ui shows the
  provider id.
- `graph_account_type`: the type a resource's `account` value names. pudl ui
  groups resources under it in every view. It is a group even when the pack
  does not collect accounts.
- `graph_resource_type`: display info for a type with no schema. A schema's
  `display` wins over it.

### Check it

```sh
pudl graph vocab          # every type and edge type, where each was declared, and problems
pudl doctor --health-only # "Graph Vocabulary" lists the same problems
```

The reported problems are:
- a cataloged type with no display info;
- a `graph_edge` rule deriving an undeclared edge type;
- a role naming an undeclared edge type;
- an edge type in an unknown category;
- two declarations of one type that disagree.

## Claiming records: `@pudl(match)`

Mark the fields that only this type's records carry with `@pudl(match)` in
the schema body. The schema's own constraint on the field is the test, so
nothing is stated twice:

```cue
#Firewall: {
	_pudl: {...}
	kind:    "compute#firewall" @pudl(match)
	project: _gcpProjectID
	...
}
#PubSubTopic: {
	_pudl: {...}
	name: =~"^projects/[^/]+/topics/[^/]+$" @pudl(match)
	...
}
```

A record whose values at every marked field satisfy those constraints
**claims** the type. Inference then assigns it this type or one of its
specializations, or rejects the import with the type's validation error. It
never falls back to a generic schema. Mark required fields (an optional field
is ignored), at any depth; with several marks, all must match. Mark fields
that identify the kind of record, such as a provider's `kind`, not ones a
generic record shares (`name: string` would claim everything with a name). The
rest of the body says what a claimed record must satisfy. For example, the GCP pack requires a valid
project ID, so gcloud output imported without `--set project=<id>` is
rejected instead of being cataloged without a project. Observe collectors that
name no schema apply the same rule. A malformed `@pudl(match)` is a schema
load error that `pudl doctor` reports.

## How classification works

Without `--schema`, pudl infers each record's type in two steps:

1. **Score candidates.** Every type is a candidate; policies never are. A
   schema scores when all its `identity_fields` are present (+0.5) and when
   its `resource_type` matches the origin (+0.15). Candidates are ordered most
   specific first along `base_schema`. A record that claims types by
   [`@pudl(match)`](#claiming-records-pudlmatch) considers only
   those types and their specializations, and is rejected if none unifies.
2. **Unify.** pudl tries CUE unification against each candidate, most specific
   first. The first that unifies is assigned. If none does, it walks up the
   `base_schema` chain, and finally assigns the catchall `pudl/core.#Item`.

An explicit schema (`pudl import --schema`, or a model's `populate.schema`) is
strict: a record that does not unify fails, unless `--allow-schema-fallback`
(or `allow_schema_fallback: true`) permits the fallback. Naming a policy is an
error that names its type.

`pudl import --explain` shows the candidates, scores and rejections.

## Resource identity

A resource ID is namespaced by the **root of the schema's `base_schema`
family**, not by the assigned schema:

```
resource_id = SHA256(family_root + "\x00" + identity values)
```

So a resource keeps its ID when it is reclassified to a more specific schema
in the same family. For that to hold, **`identity_fields` must be the same
across a family**: declare them at the root and inherit them unchanged.
Descendants may tighten the constraints on those fields, never change them.
`#Child: #Base & {...}` enforces this in CUE; for families joined only by
`base_schema`, `pudl doctor` warns when a schema's identity fields differ from
its base's.

`pudl doctor --fix --only identity` backfills missing resource IDs and
recomputes IDs that older releases namespaced by the leaf schema.

## Compliance Policies

A policy (`schema_type: "policy"`) says what a good instance of a type looks
like. After every import and observation, pudl evaluates every policy that
applies to each record's type and stores one result per resource and policy:
`compliant`, `noncompliant` or `unknown`, with named violations. A policy
applies when its `base_schema` chain reaches the record's type or one of its
ancestors, so several policies can apply to one resource.

Policies judge one resource at a time. For rules that span resources, write
Datalog rules and checks ([governance](governance.md)).

### Writing a policy

A policy is a standalone definition that names its type in `base_schema`:

```cue
package aws

#S3Bucket: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.s3.bucket"
		identity_fields: ["Name"]
		review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
	}
	Name:   string
	Region: string
	PublicAccessBlock?:    {...}
	ServerSideEncryption?: {...}
	...
}

#S3BucketBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "aws.#S3Bucket"
		description: "Baseline S3 hardening"
	}
	PublicAccessBlock: {
		BlockPublicAcls:       true @pudl(rule="s3-block-public-acls", severity="high")
		RestrictPublicBuckets: true @pudl(rule="s3-restrict-public", severity="high", doc="Public policies are ignored")
	} @pudl(absent="violation")
	ServerSideEncryption: {
		Algorithm: "aws:kms" | "AES256" @pudl(rule="s3-encryption")
	}
}

// Extends the best practice. The chain must end at a type.
#S3BucketPCI: {
	_pudl: {schema_type: "policy", base_schema: "aws.#S3BucketBestPractice"}
	Versioning: Status: "Enabled" @pudl(rule="s3-versioning", severity="medium")
}
```

Do not write `#S3BucketBestPractice: #S3Bucket & {...}`. It does not load:
`schema_type` conflicts, and the closed type rejects fields the policy adds.
pudl assembles the chain from `base_schema` itself.

A policy constrains only the fields it names. Evaluation ignores closedness
at every depth, so extra record fields never cause a violation and nested
policy structs need no `...`. Policies need no `identity_fields`.

### Rule attributes

`@pudl(...)` on a field names the constraint on it:

| Key | Meaning |
|-----|---------|
| `rule` | The rule name reported for a violation at or below this field |
| `severity` | `low`, `medium` (default), `high` or `critical`; requires `rule` |
| `doc` | One-line explanation shown by `pudl policy show`; requires `rule` |
| `absent` | Only `"violation"` (below); applies to the field and everything beneath it |

A violation reports the nearest rule at or above the failing path. A failing
constraint with no rule above it reports its path as the rule, with severity
`medium`. A policy fails to load on an unknown key, an invalid severity,
`severity` or `doc` without `rule`, one rule declared with two different
severities or docs, or a field with two different rules. Attributes are
checked along the whole chain, so a stray `@pudl(...)` key on a type field
also breaks that type's policies.

### Absent fields and status

A field the policy requires (`field:`, not `field?:`) that the record lacks
is **absent**. By default an absence makes the result `unknown`, because the
collector may not have gathered the field. An absence is a **violation**
when:

- `@pudl(absent="violation")` is on the field or an ancestor. Use it where the
  source omits a field to mean "off". Above, a bucket without
  `PublicAccessBlock` violates `s3-block-public-acls` and
  `s3-restrict-public`, one per rule beneath it.
- The field is required by a type in the record's type chain whose `review`
  status is `validated`.

The result is `noncompliant` with any violation, `unknown` with only
absences, and `compliant` otherwise. Each violation records its rule,
severity, path and kind (`conflict` or `absent`), never the record's value.
A result against a draft type is marked `provisional`.

### Extending policies

A policy whose `base_schema` names another policy extends it. Its result
reports every rule, inherited ones included, so each result stands alone, and
each violation names the policy that declared its rule (`declared_by`). Above,
a bucket without `PublicAccessBlock` is noncompliant for both
`#S3BucketBestPractice` and `#S3BucketPCI`, both naming
`#S3BucketBestPractice` as the declarer.

### Sensitive paths

Sensitive values are redacted before storage, so a policy cannot judge them.
A policy that constrains a path the chain declares sensitive, or a path
beneath one, fails to load. A sibling path is fine.

### CUE gotchas

**`&&` and `||` do not short-circuit over absent fields.** `if x == _|_ || x
== false {...}` is undecided when the record lacks `x`, and an undecided
condition applies nothing: the policy silently passes. To treat an absent
field as a value, test the list of present values with `list.Contains`:

```cue
import "list"

#S3NoPublicAcl: {
	_pudl: {schema_type: "policy", base_schema: "aws.#S3Bucket"}
	Archived?: _
	Acl?:      _ @pudl(rule="s3-no-public-acl", severity="high")
	// Unless Archived is true; an absent Archived counts as false.
	if !list.Contains([if Archived != _|_ {Archived}], true) {
		Acl?: !="public-read"
	}
}
```

**Annotate outside comprehensions.** Rules are read from the bare policy,
where a condition on record data is undecided. A field annotated only inside
such a comprehension fails to load. Declare the field with its annotation
outside (`Acl?: _ @pudl(...)` above) and constrain it inside. Declare fields
used in conditions too (`Archived?: _`).

**Conditional absence cannot be expressed.** Absences are found from the bare
policy, so a field required only inside a comprehension is never reported
absent: with `if Website != _|_ {Logging: _}`, a website bucket without
`Logging` is compliant. Instead, put a conflict on a field that is present
whenever the rule matters, and name the rule there:

```cue
#S3WebsiteLogging: {
	_pudl: {schema_type: "policy", base_schema: "aws.#S3Bucket"}
	Logging?: _
	// Reported at Website: a website bucket must log access.
	Website?: _ @pudl(rule="s3-website-logging", severity="medium")
	if Logging == _|_ {Website?: "a website bucket needs access logging"}
}
```

`examples/gcp/schema/gcp/policies.cue` uses all three patterns.

### Known limitations

- Pattern constraints (`[string]: T`) are checked for conflicts but not for
  absences; a violation under one takes the nearest named rule above it.
- A constraint that refers to a field the record lacks (`size: <=max` without
  `max`) is neither a conflict nor an absence.
- Conflicts between members of a chain are not reported at load.

### Policy workflow

```bash
pudl policy list [--type T]               # policies, rule counts, review, result counts, stale results
pudl policy show POLICY [--owner TEAM]    # rules, chain, failing resources and their owners
pudl policy evaluate --what-if draft.cue  # try an uninstalled draft; writes nothing
pudl policy evaluate [POLICY...]          # re-evaluate stored resources from retained payloads
pudl policy evaluate --dry-run            # what would change; writes nothing
pudl show ID                              # one entry's policy results and violations
```

A `--what-if` draft must be standalone CUE (no imports of other workspace
packages); it shadows an installed policy of the same name. A policy that
fails to load is reported by these commands and keeps its existing results.

Results are facts: `policy_status(resource, policy, type, status,
policy_digest)` and `policy_violation(resource, policy, rule, severity, path,
kind, declared_by)`. Checks use the built-in rule
`policy_noncompliant(resource, policy, severity)`; see
[governance](governance.md#use-policies-in-checks) for checks, thresholds and
waivers.

Each result records a digest of the policy's effective CUE; comments and
`description` do not change it. After a policy or its type changes,
`pudl policy list` and `pudl doctor` report stale results, and
`pudl doctor --fix --only policy-results` re-evaluates them.

## Create a schema from data

```bash
pudl import --path users.json
pudl schema new --from <proquint> --path user:#User
pudl schema new --from <proquint> --path user:#User --infer status=enum
```

`--path` is `<package path>:#<Definition>`; the example writes
`.pudl/schema/user/user.cue` and defines `user.#User`. For a collection entry,
`schema new` describes its items; add `--collection` to describe the
collection itself. `--infer field=enum` turns the values seen into a
disjunction.

The generated schema is a draft. Then:

1. check the `_pudl` block, especially `identity_fields`;
2. tighten constraints and mark optional fields with `?`;
3. add `...` if records may carry fields you did not list;
4. set `review: status: "validated"` once it is complete.

## Where schemas live

Schemas live in the workspace's `schema/` directory: `.pudl/schema/` in a
project, `~/.pudl/schema/` in global mode. A schema's name is its package
path and definition: `.pudl/schema/gcp/network.cue` with `package gcp`
defines `gcp.#Firewall`.

```
.pudl/schema/
├── cue.mod/module.cue
├── pudl/            # built-in schemas, rules and #SystemModel; owned by pudl
│   ├── core/        # pudl/core.#Item, pudl/core.#Collection
│   ├── aws/         # pudl/aws.#VPC, ...
│   ├── k8s/
│   └── rules/       # Datalog rules
├── models/          # your #SystemModel instances
└── user/            # your schemas, under any package path you like
```

- Put your schemas under any package path except `pudl/`, which is reserved
  for built-ins: `user/`, a team (`acme/`) or a provider (`gcp/`, as the packs
  do). `pudl doctor` warns about schemas under `pudl/`.
- The `package` declaration must match the directory's last element.
- `pudl schema add user.my-resource my-schema.cue` validates a file and copies
  it into place. Schemas in a project are versioned with your repository's Git;
  pudl does not commit them.

## After changing schemas

```bash
pudl doctor                                # what the change affects
pudl doctor --fix --dry-run                # preview every repair
pudl doctor --fix --only inference         # reclassify entries inference now assigns elsewhere
pudl doctor --fix --only projections       # resync projected facts
pudl doctor --fix --only policy-results    # re-evaluate stale policy results
```

The inference repair reclassifies only inferred entries; add
`--include-explicit` (optionally with `--schema` or `--origin`) to re-infer
explicit assignments too. Imports and checks resync projected facts on their
own, so the last two are needed only when you want them current at once.

## Tips

- **Start permissive.** Accept extra fields with `...` and tighten as you
  learn the data.
- **Identity matters most.** Good identity fields give version history,
  ownership and facts.
- **Validate types you rely on.** A validated type turns missing required
  fields into policy violations.
- **Try policies first.** `pudl policy evaluate --what-if draft.cue` shows what
  a draft would report without storing anything.
