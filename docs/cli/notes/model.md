A model is a registered `#SystemModel` in `.pudl/schema/models/`: a
`populate` collector, its `observation` scope, optional `desired` state,
`checks`, `waivers` and `depends_on`. A model without `populate` is
checks-only. `pudl check MODEL...` runs models; nothing here changes
infrastructure. See [observation](observation-scope.md) for collectors and
[governance](governance.md) for checks and waivers.
