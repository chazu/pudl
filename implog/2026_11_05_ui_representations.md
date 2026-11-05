# pudl ui: a node's representations (2026-11-05)

Issue: `pudl-h7s.3` (epic `pudl-h7s`). Design:
`docs/design/2026-11-05-shared-identity-across-schemas.md` §5.

The inspector shows every cataloged record of a resource that several schemas
describe (graph export's `representations`, from `pudl-h7s.2`).

## Public API (web)

- `web/src/model/canonical.ts`: `Representation {id, schema, type, entryId,
  origin}`; `Resource.representations?: Representation[]`.
- `web/src/ui/inspectorParts.tsx`: `RepresentationList({reps})`, one row per
  record: schema, origin and the `pudl show <entryId>` command.
- `web/src/ui/ResourcePanel.tsx`: a "Records N" section after the details
  when a resource has more than one record, noting that the details come
  from the first.
- `web/src/styles.css`: `.representations` list; `h3 .count`.

## Build

- `internal/webui/static` rebuilt with `npm run build`.

## Tests

- `web/src/data/validate.test.ts`: graph parsing keeps `representations`.
- `npx tsc -b` clean; vitest 64 passed; `CGO_ENABLED=0 go test ./...` passes.
- Not checked in a browser: the panel only appears for grouped resources,
  which need a pack declaring `_pudl.same_resource` (`pudl-h7s.4`,
  `pudl-3q5`).
