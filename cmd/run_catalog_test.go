package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogFile is where NewCatalogDB puts the database under a pudl dir. Its
// absence is the observable proof that nothing opened the catalog.
func catalogFile(pudlDir string) string {
	return filepath.Join(pudlDir, "data", "sqlite", "catalog.db")
}

// The handle opens nothing until a phase borrows it. This is what keeps
// `--dry-run` honest: it promises no catalog writes, and a run that reaches no
// catalog-touching phase must not so much as create the database file.
func TestRunCatalog_LazyOpen(t *testing.T) {
	dir := t.TempDir()
	cat := newRunCatalog(dir)

	require.NoFileExists(t, catalogFile(dir), "constructing the handle must not open anything")
	require.NoError(t, cat.Close(), "closing an unborrowed handle is a no-op")
	assert.NoFileExists(t, catalogFile(dir), "closing must not open anything either")
}

// Every borrow returns the same handle, whichever accessor asks for it. This is
// the point of the consolidation: the run path used to open the catalog eleven
// times, re-running createTables and every migration on each one.
func TestRunCatalog_OneOpenServesEveryBorrow(t *testing.T) {
	dir := t.TempDir()
	cat := newRunCatalog(dir)
	defer cat.Close()

	first, err := cat.required()
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := cat.required()
	require.NoError(t, err)
	third, err := cat.optional()
	require.NoError(t, err)

	assert.Same(t, first, second, "a second required borrow reuses the open handle")
	assert.Same(t, first, third, "an optional borrow reuses the same handle as a required one")
	assert.FileExists(t, catalogFile(dir), "the first borrow is what opens the catalog")
}

// The two accessors are the whole reason this type exists: opening in place used
// to let each phase inherit its failure semantics from its call site. A borrow
// now has to say which it is, and the two disagree about what an unopenable
// catalog means — fatal for required(), a dropped write for optional().
func TestRunCatalog_RequiredFailsWhereOptionalCarriesOn(t *testing.T) {
	// A pudl dir that cannot hold data/sqlite/, because a regular file sits
	// where the directory would go.
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

	cat := newRunCatalog(blocked)
	defer cat.Close()

	db, err := cat.required()
	assert.Nil(t, db)
	require.Error(t, err, "a required borrow reports the failure for the caller to propagate")
	assert.Contains(t, err.Error(), "open catalog", "required wraps the error the way its callers return it")

	db, optErr := cat.optional()
	assert.Nil(t, db, "an optional borrow hands back no handle, so the caller returns instead of writing")
	assert.Error(t, optErr, "the error still comes back, so a dropped write can be reported rather than swallowed")

	assert.NoError(t, cat.Close(), "closing after a failed open is a no-op")
}

// A failed open is attempted once, not once per phase. Six best-effort phases
// borrowing a broken catalog must not mean six open attempts.
func TestRunCatalog_FailedOpenIsNotRetried(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))

	cat := newRunCatalog(blocked)
	defer cat.Close()

	_, first := cat.optional()
	require.Error(t, first)

	// Unblock the path. A retrying handle would now succeed; a memoized one
	// reports the same failure, so every phase of one run sees one catalog.
	require.NoError(t, os.Remove(blocked))
	_, second := cat.optional()
	require.Error(t, second, "the open is resolved once and the outcome stands for the run")
	assert.Equal(t, first, second)
}

// Dir is what phases writing raw files beside their catalog rows use, so the
// rows and the files they point at cannot land under different roots.
func TestRunCatalog_DirIsThePudlDirItWasGiven(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, dir, newRunCatalog(dir).Dir())
}
