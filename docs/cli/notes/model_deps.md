`deps` records each registered model's declared `depends_on` as
`model_depends_on` facts without running any model, so impact queries
(`pudl query impacted_by changed=MODEL`) cover models that were never
checked. Edges recorded by retired features are labeled by their source,
such as `binding`. See [dependencies and impact](cross-model-dependencies.md).
