# Directory-confined artifact cleanup

Idea 2: committed orphan cleanup now removes relative paths through `os.Root`
directory handles after checking catalog references. Workspace data paths are
anchored at the owning workspace, and explicit external prune DataDir values
remain supported as their own boundaries. raw/metadata parent symlinks cannot
redirect removal outside the selected subtree. No public API change.

Validation: all focused deletion, orphan and prune tests passed, including escaping
subtree/nested directory symlinks for both raw and metadata, shared references,
transaction rollback, and the existing external DataDir contract. A first attempt
incorrectly restricted that explicit contract; the regression caught it and the
implementation was corrected before commit.

Final verification: the linter requires explicit handling of directory-handle
close results. These are now intentionally discarded through deferred closures;
unlink and root-resolution failures still propagate. Focused cleanup/deletion/prune
tests and focused cleanup race tests passed after that final adjustment.
