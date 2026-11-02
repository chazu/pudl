# Incremental layout (pudl-nb3)

Collapse, expand and inventory refresh used to relay out every level from
scratch: ELK's hierarchical layer sweep found a different optimum after a
small change. ELK's interactive strategies, which keep a previous order, are
rejected under INCLUDE_CHILDREN.

## Design

- `web/src/layout/elkIncremental.ts` (`ElkIncrementalEngine`): every group
  is laid out on its own (SEPARATE_CHILDREN). Each visual edge is split at
  every group boundary it crosses: an exit port on each group up from the
  source to just below the lowest common ancestor, one segment at the
  ancestor (which carries the label), and entry ports down to the target.
  The segments' sections are joined into the edge's route.
- With a previous layout, each level where at least half the children are
  known gets interactive layering, crossing minimisation and cycle breaking.
  Children are seeded at their previous centre relative to their parent, and
  new children after everything else along the flow. Node placement stays
  BK. Spikes showed interactive placement drifts (positions read as
  minimums), and NETWORK_SIMPLEX made reruns less stable.
- `layout/engine.ts` `createLayoutEngine`: incremental for layered views,
  single-pass for packed views or with `?engine=classic`.
- `layout/layoutMemory.ts`: per-view last result plus a signature-keyed
  cache (`layoutSignature`, FNV-1a over nodes, parents, sizes, ports, edges,
  labels and config; up to 24 per view). The pipeline reuses a cached
  layout for an identical signature. Relayout forgets the view; this fixes a
  regression where the relayout nonce no longer cleared memory.

## Findings (spikes)

- Under SEPARATE_CHILDREN with ROOT shape coordinates, input positions are
  read relative to the parent; feeding absolute ones made layouts drift.
- Interactive node placement grows the diagram on every pass; interactive
  layering and crossing minimisation alone reproduce a layout from its own
  positions.

## Verification

- Layout tests run against both engines: nested containment, orthogonal
  routes starting at ports, and determinism. Incremental-only tests: order
  kept ≥ 0.95 when relaid out from its own result; segments of cross-group
  edges meet at boundary ports; across collapse-largest-group and
  drop-one-resource cases in three views, incremental beats single-pass in
  total, with no case below 0.6. Signature tests: equal graphs match;
  collapse and direction change differ.
- Browser (demo network view): collapsing `prod-core` left every other
  top-level group at the same position, and expanding it restored all 60
  positions.
