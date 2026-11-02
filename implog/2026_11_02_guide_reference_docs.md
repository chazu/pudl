# Guide and reference documentation rewrite (2026-11-02)

Rewrote pudl's guide and reference docs for the discover, catalog and govern
model, following the shared docs brief. No code changed; `docs/cli-reference.md`
is regenerated from the notes.

## Structure

- **New `docs/governance.md`**: the task-first guide to checks, outcomes,
  witnesses and findings, policies in checks (`policy_noncompliant`, severity
  thresholds), waivers, finding changes (`--fail-on new`, `report --since`),
  ownership (`--owner`, `resource_unowned`), failed inputs and completeness,
  and CI exit codes. It replaces the duplicated text in the `check` and
  `report` notes, `evidence.md` and `projection.md`.
- **`docs/observation-scope.md`** now also holds the `#CommandObserve`
  reference (runs, `set`, `each`, failed inputs, `fail_fast`, `timeout`),
  moved from the `check` note.
- **`docs/cross-model-dependencies.md`** is kept (code comments link to it)
  but shrank to `depends_on`, `model_depends_on`, the dependency rules,
  `--check-upstream`, `query --topo` and the pack impact rules.
- **`docs/schema-authoring.md`** documents every `_pudl` key, types versus
  policies, classification, family identity and the Compliance Policies
  section (anchor kept).
- **`docs/facts.md`** leads with where facts come from and how to read them;
  hand-written facts are an advanced section. Migration history and internal
  Go calls were removed.
- **CLI notes**: added `policy.md`, `snapshot.md`, `model_deps.md`; rewrote
  `check`, `report`, `doctor`, `model`, `query`, `status`; trimmed retired
  references from `_intro`, `init`, `facts_search`.
- `docs/README.md` lists governance and the renamed guides.

## Corrections found while verifying

- `pudl schema new --path user/#X` fails; the path form is `user:#X`.
- `schema new --collection` describes the collection, not its items.
- Global rules do not apply inside a project; rules come from the project
  and its vendored dependencies.
- `pudl facts search "0.0.0.0/0"` is an FTS5 syntax error; phrases with
  punctuation must be quoted.
- `_pudl.description` is not displayed anywhere.

## Line counts (before -> after)

| File | Before | After |
| --- | --- | --- |
| docs/schema-authoring.md | 616 | 522 |
| docs/observation-scope.md | 107 | 142 |
| docs/evidence.md | 175 | 113 |
| docs/projection.md | 265 | 240 |
| docs/facts.md | 364 | 141 |
| docs/datalog.md | 375 | 229 |
| docs/cross-model-dependencies.md | 340 | 91 |
| docs/library-api.md | 241 | 243 |
| docs/gcp-network-hygiene.md | 109 | 96 |
| docs/governance.md | 0 | 273 |
| docs/cli/notes/*.md (21 -> 24 files) | 439 | 314 |
| examples/gcp/README.md | 253 | 255 |
| examples/aws/README.md | 315 | 317 |
| **Total** | **3,599** | **2,976** |

`docs/cli-reference.md` (generated) went from 2,210 to 2,088 lines.
