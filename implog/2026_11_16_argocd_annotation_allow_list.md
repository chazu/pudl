# Argo CD pack: allow-list annotations (2026-11-16)

The Argo CD transforms kept `metadata.annotations` of Applications and
ApplicationSets whole, and the ApplicationSet template's annotations too.
Only `kubectl.kubernetes.io/last-applied-configuration` was redacted. An
annotation can hold anything: Image Updater's
`argocd-image-updater.argoproj.io/git-repository` holds a git URL that can
carry `user:TOKEN@`, notification annotations name channels, and teams add
their own. This is `pudl-4a4l`.

## The fix (`packs/argocd/schema/models/argocd_shape.cue`)

- `_argocdAnnotationPrefixes: ["argocd.argoproj.io/", "argocd-image-updater.argoproj.io/"]`
- `_argocdAnnotations: {_src: _, out: {...}}` keeps the keys of `_src`
  that start with a prefix on the list and drops every other key. Each
  kept value goes through `_argocdRepoURL`, so a URL with a scheme loses
  its userinfo (`ssh://` keeps the user name) and any other value is kept.
- `_argocdMetadataAnnotations: {_src: _, out: {annotations?: {...}}}` sets
  `annotations` to the kept annotations of metadata `_src`, and leaves it
  out when none are kept.
- `_argocdMetadata` (Application and ApplicationSet metadata) and the
  ApplicationSet template's metadata no longer pick `annotations`; they
  use `_argocdMetadataAnnotations`.
- `_argocdRepoURLPattern` starts with `(?s)`, so a value with a newline
  (a YAML block scalar ends with one) still matches and loses its
  userinfo.

Labels are unchanged.

## Redaction

`_argocdLastApplied` and its `sensitive_fields` entries in
`argocd.#Application` and `argocd.#ApplicationSet` are removed: the allow
list drops the annotation before redaction runs. The README says so.

## Facts

`argocd_app_image_updater` reads `image-list`, `write-back-method` and
`write-back-target` under `argocd-image-updater.argoproj.io/`, which the
list keeps. The `argocd.argoproj.io/tracking-id` annotation the rules use
is read from `gcp.#K8sDeployment` records, not from Argo CD records. No
other pack CUE reads an Argo CD record's annotations.

## Tests

The hostile fixtures plant `FAKE-ARGO-ANNOTATION-OTHER` in
`example.com/note` and `FAKE-ARGO-ANNOTATION-NOTIFY` in
`notifications.argoproj.io/subscribe.on-sync-succeeded.slack`, on
`hostile-app`, `hostile-set` and the `hostile-set` template, and
`FAKE-ARGO-UPDATER-USER:FAKE-ARGO-UPDATER-TOKEN@` in
`argocd-image-updater.argoproj.io/git-repository` on `hostile-app` and the
template. `hostile-app` also gets Image Updater's image list, write-back
method `git` and a write-back target. All four values are on
`argocdPlanted`. `TestArgoCDPackHostileFixturesPlantNothing` checks the
stored annotations (Image Updater's only, the URL without userinfo), that
`hostile-set` has none, that the template keeps only git-repository, and
that the hostile `argocd_app_image_updater` fact has its write-back method
and target. `TestArgoCDPackFixtureApplications` checks the updater fact of
the normal fixtures without `hostile-app`, unchanged.

Checks: `make test`.
