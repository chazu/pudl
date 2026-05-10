# Idea: extract a `gitstore` library

Captured while scoping `lodge` (github.com/chazu/lodge — composable
AGENTS.md/CLAUDE.md snippet manager). Both pudl and lodge want the same
git-backed-directory pattern; worth considering a shared library later.

## Observation

`pudl/internal/git/git.go` is a clean shell-out wrapper around the `git` CLI:
`Repository{Path}` with `Status` / `Add` / `Commit` / `Log` / `HasChanges` /
`AddAndCommit`. ~260 lines. Nothing pudl-specific in it except the
`pudl/internal/errors` import — it's already a de facto generic layer that
got stuck in `internal/`.

## Not at the git-wrapper layer

Extracting just `git.go` isn't valuable. Everyone who needs git either shells
out or uses go-git; a thin Repository wrapper is table stakes. The right
layer is one up: the **pass-style git-backed store** pattern.

## The actual abstraction

Name TBD (`gitstore`, `archive`, `ledger`, ...). Core idea:

> A directory where path-addressed mutations auto-commit, and any past
> version is readable.

```go
type Store struct { ... }
func Open(path string) (*Store, error)
func Init(path string) (*Store, error)

// Path-addressed CRUD, each call auto-commits
func (s *Store) Put(path string, data []byte, msg string) error
func (s *Store) Get(path string) ([]byte, error)
func (s *Store) Delete(path, msg string) error
func (s *Store) List(prefix string) ([]string, error)

// Versioning
func (s *Store) GetAt(path, ref string) ([]byte, error)
func (s *Store) History(path string) ([]Commit, error)

// Batch multiple writes into one commit
func (s *Store) Tx(msg string, fn func(*Tx) error) error

// Escape hatch
func (s *Store) Repo() *Repository  // raw wrapper for git status/log/etc.
```

## Stays OUT (to keep it general)

- **Encryption** — `pass` encrypts, pudl/lodge don't. `Put` takes `[]byte`;
  callers encrypt if they want.
- **Remote sync (push/pull/clone)** — defer. Local-only covers most cases.
- **Content interpretation** — no schemas, no markdown parsing, no CUE.
  Bytes in, bytes out.
- **Workspace layout conventions** — no opinions on `~/.foo/`; caller
  passes a path.
- **Merge/conflict UX** — caller's problem if they add remotes.
- **Auth, signing policy** — rely on the user's git config.

## Who benefits

- **lodge** — snippets as `base/identity.md`, auto-versioned.
- **pudl** — schemas dir is already git-tracked; this formalizes the
  "mutate + commit" pattern pudl already uses ad-hoc.
- Pass clones, note apps, agent-memory stores, config managers, tiny CMSes.

## Recommendation

**Don't extract yet.** Build lodge first with an internal `store` package.
When two commands deep into pudl integration, or when the copy-paste itch
kicks in, extract to `github.com/chazu/gitstore` (or whatever the name
lands on). Premature extraction is a bigger risk than delayed extraction —
you don't yet know which methods pudl actually needs from a shared API.

Trigger to revisit: second consumer (lodge) has stabilized its
`store`-layer API, **and** pudl has a concrete reason to replace its
`internal/git` usage (e.g. wanting `GetAt` or `History` for schemas).
