# Evidence boundary audit

Runtime inventory matching now resolves routing tags and canonical schema names
to the same identity family. Typed records without a routing tag use their stored
schema metadata, so matching cannot collapse same-named resources in different
projects. Human finding labels retain their authored routing names.

Projection now persists an incomplete status when numeric values are omitted
from the query domain. Dependent latest-known checks become unknown. Projection
fingerprints include contract version 2 so old projection state is refreshed on
next use. Snapshot checks already detect these omissions during materialization.

Handled assignment-spool flush/close errors and cleared lint findings. Real-Mu
smoke assertions now decode structured preflight errors; their no-mutation and
no-provider-traffic assertions remain intact.

Focused cmd/projection/importer/mubridge suites passed, including composite
identity without routing tags, original Git finding labels/history, and omitted
numeric evidence recovering after a projection change. Full final qualification
is recorded separately.

The vulnerability scanner reports advisories in the existing Go 1.26.6 and
network dependency pins. Follow-up pudl-ew9 tracks the toolchain/dependency
upgrade; this scope-reduction change does not claim a clean vulnerability scan.
