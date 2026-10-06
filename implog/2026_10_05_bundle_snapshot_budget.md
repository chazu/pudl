# SQLite backup staging budget

Idea 3: `onlineSnapshot` receives the bundle byte budget. It rejects a source
whose page count and page size already exceed the budget before creating a copy,
then checks the destination after each 128-page backup step to detect growth.
The existing aggregate payload budget still applies to the complete capture.
The limit is checked during copying; private SQLite journal/WAL bytes and temporary
step overshoot are not a promise of an exact total filesystem quota.

Validation: full bundle tests passed, including a 1 MiB evidence table rejected
one byte below its SQLite file size without creating a destination, exact-budget
backup with data verification, rowid-preserving recovery, and existing byte-limit,
cancellation and corruption cases. No public API change.
