package importer

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/stretchr/testify/require"
)

func assertPreparationPublishedNothing(t *testing.T, imp *EnhancedImporter, dataDir string) {
	t.Helper()
	result, err := imp.catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	require.Empty(t, result.Entries)
	for _, name := range []string{"raw", "metadata", "tmp"} {
		err := filepath.WalkDir(filepath.Join(dataDir, name), func(path string, d os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !d.IsDir() {
				t.Errorf("failed preparation retained file %s", path)
			}
			return nil
		})
		require.NoError(t, err)
	}
}

func TestPreparationMalformedCollectionDoesNotWaitForCatalogWriter(t *testing.T) {
	imp, source, dataDir := collectionFixture(t, "invalid.json", `[{"name":"valid"},{"name":]`)
	acquired := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- imp.catalogDB.WithCatalogTx(func(*database.CatalogTx) error { close(acquired); <-release; return nil })
	}()
	<-acquired
	released := false
	defer func() {
		if !released {
			close(release)
			<-done
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{Context: ctx, SourcePath: source})
	require.Error(t, err)
	require.False(t, errors.Is(err, context.DeadlineExceeded), "malformed preparation must finish before entering the held write transaction")
	close(release)
	released = true
	require.NoError(t, <-done)
	assertPreparationPublishedNothing(t, imp, dataDir)
}

func TestPreparationAggregateStagingIncludesSourceRawAndDescriptors(t *testing.T) {
	content := `[{"name":"` + strings.Repeat("a", 1024) + `"},{"name":"` + strings.Repeat("b", 1024) + `"}]`
	imp, source, dataDir := collectionFixture(t, "records.json", content)
	sum := sha256.Sum256([]byte(content))
	id := hex.EncodeToString(sum[:])
	limits, err := (ingestprep.Limits{}).Resolve()
	require.NoError(t, err)
	stream := collectionStream{importer: imp, opts: ImportOptions{Context: context.Background(), Limits: limits, SourcePath: source}, collectionID: id, timestamp: time.Now(), rawDir: filepath.Join(dataDir, "raw", time.Now().Format("2006/01/02")), metadataDir: filepath.Join(dataDir, "metadata")}
	require.NoError(t, stream.prepare(source, "json-array"))
	preparedBytes := stream.stagedBytes
	stream.close()
	// This budget comfortably fits the prepared raw+descriptors by themselves,
	// and fits the source by itself, but not the simultaneously owned aggregate.
	capBytes := preparedBytes + int64(len(content))/2
	require.Greater(t, capBytes, preparedBytes)
	require.Greater(t, capBytes, int64(len(content)))
	_, err = imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, Limits: ingestprep.Limits{StagingBytes: capBytes}})
	require.ErrorContains(t, err, "staging byte limit")
	assertPreparationPublishedNothing(t, imp, dataDir)
}

func TestPreparationCompressedDecodedLimitPublishesNothing(t *testing.T) {
	imp, source, dataDir := collectionFixture(t, "records.json.gz", "")
	file, err := os.Create(source)
	require.NoError(t, err)
	z := gzip.NewWriter(file)
	_, err = z.Write([]byte(`[{"name":"` + strings.Repeat("x", 8192) + `"}]`))
	require.NoError(t, err)
	require.NoError(t, z.Close())
	require.NoError(t, file.Close())
	_, err = imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, Limits: ingestprep.Limits{DecodedBytes: 256}})
	require.ErrorContains(t, err, "byte limit")
	assertPreparationPublishedNothing(t, imp, dataDir)
}

func TestPreparationCancellationWhileAwaitingPublicationPublishesNothing(t *testing.T) {
	imp, source, dataDir := collectionFixture(t, "records.json", `[{"name":"a"},{"name":"b"}]`)
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- artifacts.WithLock(context.Background(), imp.catalogDB.Root(), func() error { close(locked); <-release; return nil })
	}()
	<-locked
	released := false
	defer func() {
		if !released {
			close(release)
			<-done
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{Context: ctx, SourcePath: source})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	released = true
	require.NoError(t, <-done)
	assertPreparationPublishedNothing(t, imp, dataDir)
}

func TestPreparationRejectsMalformedFinalRecordAndTrailingPayload(t *testing.T) {
	for _, content := range []string{`[{"name":"first"},{"name":]`, `[{"name":"first"}] {"trailing":true}`, `[{"name":"first"},]`} {
		t.Run(content, func(t *testing.T) {
			imp, source, dataDir := collectionFixture(t, "records.json", content)
			_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
			require.Error(t, err)
			assertPreparationPublishedNothing(t, imp, dataDir)
		})
	}
}

func TestPreparationReimportReplacesOrphanMetadataPreservesLiveMetadata(t *testing.T) {
	content := `{"name":"resource","value":7}`
	imp, source, dataDir := collectionFixture(t, "record.json", content)
	sum := sha256.Sum256([]byte(content))
	id := hex.EncodeToString(sum[:])
	meta := filepath.Join(dataDir, "metadata", id+".meta")
	require.NoError(t, os.MkdirAll(filepath.Dir(meta), 0755))
	require.NoError(t, os.WriteFile(meta, []byte(`{"orphan":true,"old_timestamp":"old"}`), 0600))
	imported, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.False(t, imported.Skipped)
	before, err := os.ReadFile(imported.MetadataPath)
	require.NoError(t, err)
	require.NotContains(t, string(before), "old_timestamp")
	repeated, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, Explain: true})
	require.NoError(t, err)
	require.True(t, repeated.Skipped)
	after, err := os.ReadFile(imported.MetadataPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPreparationCollectionExplainDedupUsesOriginalAssignment(t *testing.T) {
	imp, source, _ := collectionFixture(t, "original.json", `[{"name":"shared"}]`)
	first, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, Explain: true})
	require.NoError(t, err)
	entries, err := imp.catalogDB.GetCollectionItems(first.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	bytes, err := os.ReadFile(entries[0].MetadataPath)
	require.NoError(t, err)
	var meta ImportMetadata
	require.NoError(t, json.Unmarshal(bytes, &meta))
	require.NotNil(t, meta.SchemaInfo.Explanation)
	// A recognizable persisted trace proves the second collection didn't expose
	// a fresh speculative classification for the already-cataloged shared item.
	meta.SchemaInfo.Explanation.Selected = "original-assignment-sentinel"
	bytes, err = json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(entries[0].MetadataPath, bytes, 0600))
	secondSource := filepath.Join(filepath.Dir(source), "other.json")
	require.NoError(t, os.WriteFile(secondSource, []byte(`[{"name":"shared"},{"name":"new"}]`), 0600))
	second, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: secondSource, Explain: true})
	require.NoError(t, err)
	var shared *inference.InferenceTrace
	for _, item := range second.ItemExplanations {
		if item.Index == 0 {
			shared = item.Trace
		}
	}
	require.NotNil(t, shared)
	require.Equal(t, "original-assignment-sentinel", shared.Selected)
	after, err := os.ReadFile(entries[0].MetadataPath)
	require.NoError(t, err)
	require.Equal(t, bytes, after)
}
