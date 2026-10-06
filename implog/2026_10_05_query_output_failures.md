# Query output failure propagation

Idea 5: JSON uses the checked output writer, tuple marshaling/writing returns
errors, and human counts, empty results, relation listing and topology all return
sink failures. Relation listing assembles its small metadata display only after
successful catalog/rule reads. Public JSON contents and normal human output stay
the same; incomplete delivery now exits with failure. No public Go API change.

Validation: all TestQuery regressions passed, including failing injected sinks for
JSON, tuples, final counts, empty results, lists, topology and empty topology, plus
the exact-number CLI tests and request validation cases.
