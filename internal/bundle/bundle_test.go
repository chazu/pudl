package bundle

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

type bundleFixtureState struct {
	root     string
	report   []byte
	facts    []database.Fact
	sequence int64
}

func bundleFixture(t *testing.T) bundleFixtureState {
	t.Helper()
	root := filepath.Join(t.TempDir(), "original.pudl")
	require.NoError(t, config.DefaultConfigFor(root).SaveTo(root))
	write := func(rel, body string) string {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0600))
		return path
	}
	write("schema/user/resource.cue", "package user\n#Resource: {name: string}\n")
	write("definitions/checks.cue", "package checks\n#Checks: {description: \"authored\"}\n")
	write("models/demo.cue", "package models\nname: \"demo\"\n")
	write("populators/demo.cue", "package populators\nname: \"populate-demo\"\n")
	write("schema/pudl/rules/demo.cue", "package rules\n#Large: {rule: \"large(x) :- observed(x).\"}\n")
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	resource := "resource-exact"
	clean := "clean"
	for i, serial := range []string{"9007199254740993", "9007199254740994"} {
		id := []string{"item-v1", "item-v2"}[i]
		version := i + 1
		rawSum := sha256.Sum256([]byte(`{"name":"exact","serial":` + serial + `}`))
		hash := hex.EncodeToString(rawSum[:])
		raw := write("data/raw/"+id+".json", `{"name":"exact","serial":`+serial+`}`)
		meta := write("data/metadata/"+id+".meta", `{"id":"`+id+`","source_info":{"original_path":"/historical/source.json"},"resource_tracking":{"resource_id":"resource-exact","version":`+[]string{"1", "2"}[i]+`,"identity_values":{"serial":`+serial+`}},"stored_path":"`+raw+`"}`)
		entry := database.CatalogEntry{ID: id, StoredPath: raw, MetadataPath: meta, ImportTimestamp: now.Add(time.Duration(i) * time.Second), Format: "json", Origin: "bundle-test", Schema: "user.#Resource", RecordCount: 1, ResourceID: &resource, ContentHash: &hash, Version: &version, Status: &clean}
		if i == 1 {
			kind, collection, index := "item", "collection-a", 0
			entry.CollectionType = &kind
			entry.CollectionID = &collection
			entry.ItemIndex = &index
		}
		require.NoError(t, db.AddEntry(entry))
	}
	for _, id := range []string{"collection-a", "collection-b"} {
		kind := "collection"
		raw := write("data/raw/"+id+".json", `[{"name":"exact","serial":9007199254740994}]`)
		require.NoError(t, db.AddEntry(database.CatalogEntry{ID: id, StoredPath: raw, ImportTimestamp: now, Format: "json", Origin: "bundle-test", Schema: "pudl/core.#Collection", CollectionType: &kind, RecordCount: 1}))
	}
	require.NoError(t, db.AddCollectionMembership("collection-b", "item-v2", 4))
	_, err = db.AddFact(database.Fact{Relation: "observed", Args: `{"serial":9007199254740993}`, ValidStart: 100, Source: "bundle-test"})
	require.NoError(t, err)
	first, err := db.QueryFacts(database.FactFilter{Relation: "observed", ValidAt: bundleInt64(101)})
	require.NoError(t, err)
	require.Len(t, first, 1)
	sequence := first[0].TxSeq
	require.NoError(t, db.RetractFact(first[0].ID))
	_, err = db.AddFact(database.Fact{Relation: "observed", Args: `{"serial":9007199254740994}`, ValidStart: 100, Source: "bundle-test"})
	require.NoError(t, err)
	facts, err := db.FactHistory("observed")
	require.NoError(t, err)
	require.NoError(t, db.StartRun("historical-run", "demo", "observe"))
	require.NoError(t, db.FinishRun("historical-run", database.RunConclusion{CompletionStatus: database.RunStatusSucceeded, Verdict: "clean"}))
	report := []byte(`{"report_version":1,"run_id":"historical-run","model":"demo","ok":true,"expected":{"serial":9007199254740993}}`)
	require.NoError(t, db.SaveRunReport("historical-run", "demo", report))
	require.NoError(t, db.SaveRunSetReport("historical-set", []byte(`{"run_set_id":"historical-set","status":"succeeded"}`)))
	require.NoError(t, db.SaveRunApproval("pending-run", "demo", []byte(`{"instruction":"requires fresh approval"}`)))
	require.NoError(t, db.SaveRunSetApproval("pending-set", "digest", []byte(`{"instruction":"requires fresh approval"}`), []byte(`{"plan":"history"}`)))
	// The approval-only snapshot has no manual owner and must not acquire one on restore.
	kind := "collection"
	snapshotRaw := write("data/raw/snapshot.json", `{"snapshot_id":"approval-snapshot"}`)
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: "approval-snapshot", StoredPath: snapshotRaw, ImportTimestamp: now, Format: "json", Origin: "bundle-test", Schema: "pudl/mu.#ObserveSnapshot", CollectionType: &kind}))
	require.NoError(t, db.RecordObserveSnapshot(database.ObserveSnapshot{SnapshotID: "approval-snapshot", RunID: "historical-run", Model: "demo", Workspace: "original", Source: database.SnapshotSourceMuObserve, CreatedAt: now}))
	require.NoError(t, db.SetSnapshotPin(database.SnapshotPin{SnapshotID: "approval-snapshot", OwnerKind: database.SnapshotPinApproval, OwnerID: "pending-set"}, true))
	return bundleFixtureState{root: root, report: report, facts: facts, sequence: sequence}
}
func bundleInt64(n int64) *int64 { return &n }

func TestBundleMovedRootRoundTripPreservesEvidenceAndHistory(t *testing.T) {
	fixture := bundleFixture(t)
	archive := filepath.Join(t.TempDir(), "workspace.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	// Remove the original to prove every operational artifact is restored locally.
	require.NoError(t, os.RemoveAll(fixture.root))
	root := filepath.Join(t.TempDir(), "moved.pudl")
	require.NoError(t, Restore(context.Background(), archive, root, 0))
	cfg, err := config.LoadFrom(root)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "data"), cfg.DataPath)
	require.Equal(t, filepath.Join(root, "schema"), cfg.SchemaPath)
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	for i, id := range []string{"item-v1", "item-v2"} {
		entry, err := db.GetEntry(id)
		require.NoError(t, err)
		require.Equal(t, i+1, *entry.Version)
		require.Equal(t, "resource-exact", *entry.ResourceID)
		require.Equal(t, "unknown", *entry.Status)
		require.True(t, strings.HasPrefix(entry.StoredPath, root+string(filepath.Separator)))
		require.True(t, strings.HasPrefix(entry.MetadataPath, root+string(filepath.Separator)))
		raw, err := os.ReadFile(entry.StoredPath)
		require.NoError(t, err)
		require.Contains(t, string(raw), []string{"9007199254740993", "9007199254740994"}[i])
		meta, err := os.ReadFile(entry.MetadataPath)
		require.NoError(t, err)
		require.Contains(t, string(meta), "/historical/source.json")
		require.Contains(t, string(meta), []string{"9007199254740993", "9007199254740994"}[i])
		require.Contains(t, string(meta), entry.StoredPath)
	}
	for _, tc := range []struct {
		id    string
		index int
	}{{"collection-a", 0}, {"collection-b", 4}} {
		items, err := db.GetCollectionItems(tc.id)
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Equal(t, "item-v2", items[0].ID)
		require.Equal(t, tc.index, *items[0].ItemIndex)
		require.Equal(t, tc.id, *items[0].CollectionID)
	}
	facts, err := db.FactHistory("observed")
	require.NoError(t, err)
	require.Equal(t, fixture.facts, facts)
	historical, err := db.QueryFacts(database.FactFilter{Relation: "observed", ValidAt: bundleInt64(101), TxSeqAt: &fixture.sequence})
	require.NoError(t, err)
	require.Len(t, historical, 1)
	require.Contains(t, historical[0].Args, "9007199254740993")
	report, err := db.GetRunReport("historical-run")
	require.NoError(t, err)
	require.Equal(t, fixture.report, report.Report)
	set, err := db.GetRunSetReport("historical-set")
	require.NoError(t, err)
	require.JSONEq(t, `{"run_set_id":"historical-set","status":"succeeded"}`, string(set.Report))
	approval, err := db.GetRunApproval("pending-run")
	require.NoError(t, err)
	require.Equal(t, "rejected", approval.Status)
	require.NotNil(t, approval.ResolvedAt)
	require.Error(t, db.ResolveRunApproval("pending-run", "approved"))
	exact, err := db.GetRunSetApproval("pending-set")
	require.NoError(t, err)
	require.Equal(t, "rejected", exact.Status)
	require.Error(t, db.ResolveRunSetApproval("pending-set", "digest", "approved"))
	pins, err := db.SnapshotPins("approval-snapshot")
	require.NoError(t, err)
	require.Empty(t, pins, "restore must release only stale approval ownership without manufacturing manual pins")
	snapshot, err := db.GetObserveSnapshot("approval-snapshot")
	require.NoError(t, err)
	require.False(t, snapshot.Retained)
	live, err := db.LatestSuccessfulObserveSnapshot("demo", "original")
	require.NoError(t, err)
	require.Nil(t, live, "restored successful history must not authorize producer snapshot reuse")
	historicalRun, err := db.GetRun("historical-run")
	require.NoError(t, err)
	require.Equal(t, database.RunStatusSucceeded, historicalRun.CompletionStatus)
	pinned, err := db.ObserveSnapshotByIDForRun("approval-snapshot", "demo", "original", "historical-run")
	require.NoError(t, err)
	require.Nil(t, pinned, "pinned historical snapshots must require replanning after restore")
	require.NoError(t, db.StartRun("live-after-restore", "demo", "observe"))
	kind := "collection"
	freshPath := filepath.Join(root, "data/raw/fresh-snapshot.json")
	require.NoError(t, os.WriteFile(freshPath, []byte(`{"snapshot_id":"fresh-snapshot"}`), 0600))
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: "fresh-snapshot", StoredPath: freshPath, Schema: "pudl/mu.#ObserveSnapshot", Format: "json", Origin: "new-observation", ImportTimestamp: time.Now(), CollectionType: &kind}))
	require.NoError(t, db.RecordObserveSnapshot(database.ObserveSnapshot{SnapshotID: "fresh-snapshot", RunID: "live-after-restore", Model: "demo", Workspace: "original", Source: database.SnapshotSourceMuObserve, CreatedAt: time.Now()}))
	require.NoError(t, db.FinishRun("live-after-restore", database.RunConclusion{CompletionStatus: database.RunStatusSucceeded}))
	live, err = db.LatestSuccessfulObserveSnapshot("demo", "original")
	require.NoError(t, err)
	require.NotNil(t, live)
	require.Equal(t, "fresh-snapshot", live.SnapshotID)

	for _, rel := range []string{"schema/user/resource.cue", "definitions/checks.cue", "models/demo.cue", "populators/demo.cue", "schema/pudl/rules/demo.cue"} {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		require.NoError(t, err, rel)
	}
}

type archiveFixtureMember struct {
	header tar.Header
	body   []byte
}

func readBundleArchive(t *testing.T, path string) (Manifest, []archiveFixtureMember) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	z, err := zstd.NewReader(f)
	require.NoError(t, err)
	defer z.Close()
	tr := tar.NewReader(z)
	header, err := tr.Next()
	require.NoError(t, err)
	require.Equal(t, "manifest.json", header.Name)
	var manifest Manifest
	require.NoError(t, json.NewDecoder(tr).Decode(&manifest))
	var members []archiveFixtureMember
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		members = append(members, archiveFixtureMember{header: *header, body: body})
	}
	return manifest, members
}
func writeBundleArchive(t *testing.T, path string, m Manifest, members []archiveFixtureMember) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	z, err := zstd.NewWriter(f)
	require.NoError(t, err)
	tw := tar.NewWriter(z)
	body, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(body)), Typeflag: tar.TypeReg}))
	_, err = tw.Write(body)
	require.NoError(t, err)
	for _, member := range members {
		require.NoError(t, tw.WriteHeader(&member.header))
		_, err = tw.Write(member.body)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, z.Close())
	require.NoError(t, f.Close())
}

func TestBundleRestoreRejectsMalformedArchivesWithoutPublishing(t *testing.T) {
	fixture := bundleFixture(t)
	original := filepath.Join(t.TempDir(), "valid.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, original, 0))
	for _, name := range []string{"checksum", "future-version", "traversal", "absolute-path", "symlink", "extra-member", "missing-member", "duplicate-member"} {
		t.Run(name, func(t *testing.T) {
			m, members := readBundleArchive(t, original)
			switch name {
			case "checksum":
				m.Files[0].SHA256 = strings.Repeat("0", 64)
			case "future-version":
				m.Version = Version + 1
			case "traversal":
				m.Files[0].Path = "../escape"
				members[0].header.Name = "../escape"
			case "absolute-path":
				m.Files[0].Path = "/tmp/escape"
				members[0].header.Name = "/tmp/escape"
			case "symlink":
				members[0].header.Typeflag = tar.TypeSymlink
				members[0].header.Linkname = "../escape"
				members[0].header.Size = 0
				members[0].body = nil
			case "extra-member":
				members = append(members, archiveFixtureMember{header: tar.Header{Name: "unlisted.txt", Mode: 0600, Typeflag: tar.TypeReg, Size: 5}, body: []byte("extra")})
			case "missing-member":
				members = members[1:]
			case "duplicate-member":
				members = append(members, members[0])
			}
			parent := t.TempDir()
			bad := filepath.Join(parent, "malformed.tar.zst")
			writeBundleArchive(t, bad, m, members)
			destination := filepath.Join(parent, "restored")
			require.Error(t, Restore(context.Background(), bad, destination, 0))
			_, err := os.Lstat(destination)
			require.True(t, os.IsNotExist(err))
			_, err = os.Stat(filepath.Join(parent, "escape"))
			require.True(t, os.IsNotExist(err))
			staged, err := filepath.Glob(filepath.Join(parent, ".pudl-restore-*"))
			require.NoError(t, err)
			require.Empty(t, staged)
		})
	}
	bytes, err := os.ReadFile(original)
	require.NoError(t, err)
	for _, cut := range []int{len(bytes) / 2, len(bytes) - 1} {
		t.Run("truncated", func(t *testing.T) {
			parent := t.TempDir()
			bad := filepath.Join(parent, "truncated.tar.zst")
			require.NoError(t, os.WriteFile(bad, bytes[:cut], 0600))
			destination := filepath.Join(parent, "restored")
			require.Error(t, Restore(context.Background(), bad, destination, 0))
			_, err := os.Stat(destination)
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestBundleDestinationAndByteLimitFailuresAreNonDestructive(t *testing.T) {
	fixture := bundleFixture(t)
	archive := filepath.Join(t.TempDir(), "backup.tar.zst")
	require.NoError(t, os.WriteFile(archive, []byte("original destination"), 0600))
	require.Error(t, Export(context.Background(), fixture.root, archive, 1))
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	require.Equal(t, "original destination", string(data))
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	destination := filepath.Join(t.TempDir(), "existing")
	require.NoError(t, os.MkdirAll(destination, 0700))
	sentinel := filepath.Join(destination, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0600))
	require.Error(t, Restore(context.Background(), archive, destination, 0))
	data, err = os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(data))
	limited := filepath.Join(t.TempDir(), "limited")
	require.Error(t, Restore(context.Background(), archive, limited, 1))
	_, err = os.Stat(limited)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, os.WriteFile(archive, []byte("original destination"), 0600))
	require.NoError(t, os.Remove(filepath.Join(fixture.root, "data/raw/item-v1.json")))
	require.Error(t, Export(context.Background(), fixture.root, archive, 0))
	data, err = os.ReadFile(archive)
	require.NoError(t, err)
	require.Equal(t, "original destination", string(data))
}
func TestBundleExportRejectsSymlinkEvidenceIncludingAncestor(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "ancestor"}[ancestor], func(t *testing.T) {
			fixture := bundleFixture(t)
			outside := t.TempDir()
			if ancestor {
				original := filepath.Join(fixture.root, "data/raw")
				require.NoError(t, os.Rename(original, filepath.Join(outside, "raw")))
				require.NoError(t, os.Symlink(filepath.Join(outside, "raw"), original))
			} else {
				path := filepath.Join(fixture.root, "data/raw/item-v1.json")
				require.NoError(t, os.Rename(path, filepath.Join(outside, "record.json")))
				require.NoError(t, os.Symlink(filepath.Join(outside, "record.json"), path))
			}
			archive := filepath.Join(t.TempDir(), "backup.tar.zst")
			require.Error(t, Export(context.Background(), fixture.root, archive, 0))
			_, err := os.Stat(archive)
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestBundleRejectsSelfConsistentMissingCatalogReference(t *testing.T) {
	fixture := bundleFixture(t)
	archive := filepath.Join(t.TempDir(), "valid.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	m, members := readBundleArchive(t, archive)
	// Change a SQLite reference while retaining valid archive checksums.
	rewriteBundleCatalog(t, &m, members, func(db *database.CatalogDB) {
		_, err := db.DB().Exec("UPDATE catalog_entries SET stored_path=? WHERE id='item-v1'", filepath.Join(fixture.root, "data/raw/not-in-bundle.json"))
		require.NoError(t, err)
	})
	invalid := filepath.Join(t.TempDir(), "missing-reference.tar.zst")
	writeBundleArchive(t, invalid, m, members)
	root := filepath.Join(t.TempDir(), "restored")
	require.ErrorContains(t, Restore(context.Background(), invalid, root, 0), "missing evidence")
	_, err := os.Stat(root)
	require.True(t, os.IsNotExist(err))
}

func rewriteBundleCatalog(t *testing.T, m *Manifest, members []archiveFixtureMember, mutate func(*database.CatalogDB)) {
	t.Helper()
	for i := range members {
		if members[i].header.Name != catalogPath {
			continue
		}
		temp := t.TempDir()
		path := filepath.Join(temp, filepath.FromSlash(catalogPath))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, members[i].body, 0600))
		db, err := database.NewCatalogDB(temp)
		require.NoError(t, err)
		mutate(db)
		require.NoError(t, db.Close())
		members[i].body, err = os.ReadFile(path)
		require.NoError(t, err)
		members[i].header.Size = int64(len(members[i].body))
		sum := sha256.Sum256(members[i].body)
		for j := range m.Files {
			if m.Files[j].Path == catalogPath {
				m.Files[j].Size = int64(len(members[i].body))
				m.Files[j].SHA256 = hex.EncodeToString(sum[:])
			}
		}
		return
	}
	t.Fatal("archive has no catalog")
}
func TestBundleRejectsFutureCatalogMigrationBeforePublishing(t *testing.T) {
	fixture := bundleFixture(t)
	archive := filepath.Join(t.TempDir(), "valid.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	m, members := readBundleArchive(t, archive)
	rewriteBundleCatalog(t, &m, members, func(db *database.CatalogDB) {
		_, err := db.DB().Exec(`INSERT INTO schema_migrations(version,name,applied_at) VALUES(99999,'future',CURRENT_TIMESTAMP)`)
		require.NoError(t, err)
	})
	invalid := filepath.Join(t.TempDir(), "future-schema.tar.zst")
	writeBundleArchive(t, invalid, m, members)
	root := filepath.Join(t.TempDir(), "restored")
	require.ErrorContains(t, Restore(context.Background(), invalid, root, 0), "newer than supported")
	_, err := os.Stat(root)
	require.True(t, os.IsNotExist(err))
}
func TestBundleRejectsTrailingUnlistedCompressedData(t *testing.T) {
	fixture := bundleFixture(t)
	archive := filepath.Join(t.TempDir(), "valid.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	original, err := os.ReadFile(archive)
	require.NoError(t, err)
	var appended bytes.Buffer
	encoder, err := zstd.NewWriter(&appended)
	require.NoError(t, err)
	_, err = encoder.Write([]byte("unlisted bytes after the tar terminator"))
	require.NoError(t, err)
	require.NoError(t, encoder.Close())
	corrupt := filepath.Join(t.TempDir(), "extra-frame.tar.zst")
	require.NoError(t, os.WriteFile(corrupt, append(original, appended.Bytes()...), 0600))
	root := filepath.Join(t.TempDir(), "restored")
	require.Error(t, Restore(context.Background(), corrupt, root, 0))
	_, err = os.Stat(root)
	require.True(t, os.IsNotExist(err))
}
func TestBundleCancellationLeavesNoPublishedDestination(t *testing.T) {
	fixture := bundleFixture(t)
	archive := filepath.Join(t.TempDir(), "valid.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	existing := filepath.Join(t.TempDir(), "existing.tar.zst")
	require.NoError(t, os.WriteFile(existing, []byte("preserve"), 0600))
	require.ErrorIs(t, Export(ctx, fixture.root, existing, 0), context.Canceled)
	raw, err := os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(raw))
	root := filepath.Join(t.TempDir(), "restored")
	require.ErrorIs(t, Restore(ctx, archive, root, 0), context.Canceled)
	_, err = os.Stat(root)
	require.True(t, os.IsNotExist(err))
}

func TestBundleSourceReopenPreservesApprovalPinOwnership(t *testing.T) {
	fixture := bundleFixture(t)
	db, err := database.NewCatalogDB(fixture.root)
	require.NoError(t, err)
	defer db.Close()
	pins, err := db.SnapshotPins("approval-snapshot")
	require.NoError(t, err)
	require.Len(t, pins, 1)
	require.Equal(t, database.SnapshotPinApproval, pins[0].OwnerKind)
	require.Equal(t, "pending-set", pins[0].OwnerID)
}

func TestBundlePreservesGappedSnapshotChronology(t *testing.T) {
	fixture := bundleFixture(t)
	db, err := database.NewCatalogDB(fixture.root)
	require.NoError(t, err)
	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"z-first", "gap", "a-last"} {
		if id != "gap" {
			kind := "collection"
			path := filepath.Join(fixture.root, "data/raw/"+id+".json")
			require.NoError(t, os.WriteFile(path, []byte(`{"snapshot_id":"`+id+`"}`), 0600))
			require.NoError(t, db.AddEntry(database.CatalogEntry{ID: id, StoredPath: path, Schema: "pudl/mu.#ObserveSnapshot", Format: "json", Origin: "chronology", ImportTimestamp: created, CollectionType: &kind}))
		}
		require.NoError(t, db.RecordObserveSnapshot(database.ObserveSnapshot{SnapshotID: id, Model: "chronology", Workspace: "original", Source: database.SnapshotSourceMuObserve, CreatedAt: created}))
	}
	_, err = db.DB().Exec("DELETE FROM observe_snapshots WHERE snapshot_id='gap'")
	require.NoError(t, err)
	rowids := func(catalog *database.CatalogDB) map[string]int64 {
		rows, err := catalog.DB().Query("SELECT snapshot_id,rowid FROM observe_snapshots ORDER BY rowid")
		require.NoError(t, err)
		defer rows.Close()
		result := map[string]int64{}
		for rows.Next() {
			var id string
			var rowid int64
			require.NoError(t, rows.Scan(&id, &rowid))
			result[id] = rowid
		}
		require.NoError(t, rows.Err())
		return result
	}
	before := rowids(db)
	require.NoError(t, db.Close())
	archive := filepath.Join(t.TempDir(), "chronology.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	root := filepath.Join(t.TempDir(), "restored")
	require.NoError(t, Restore(context.Background(), archive, root, 0))
	restored, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer restored.Close()
	require.Equal(t, before, rowids(restored), "snapshot rowids are chronology and must survive backup gaps")
	snapshots, err := restored.ListObserveSnapshots("chronology", 0)
	require.NoError(t, err)
	require.Len(t, snapshots, 2)
	require.Equal(t, "a-last", snapshots[0].SnapshotID)
	require.Equal(t, "z-first", snapshots[1].SnapshotID)
}

func TestBundleRejectsDamagedValidSHAEvidenceWithoutReplacingArchive(t *testing.T) {
	fixture := bundleFixture(t)
	path := filepath.Join(fixture.root, "data/raw/item-v1.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"exact","serial":9007199254740999}`), 0600))
	archive := filepath.Join(t.TempDir(), "existing.tar.zst")
	require.NoError(t, os.WriteFile(archive, []byte("preserve existing archive"), 0600))
	require.ErrorContains(t, Export(context.Background(), fixture.root, archive, 0), "hash mismatch")
	raw, err := os.ReadFile(archive)
	require.NoError(t, err)
	require.Equal(t, "preserve existing archive", string(raw))
	raw, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "9007199254740999", "verification must not repair evidence")
}
func TestBundleNormalizesOnlyLegacySyntheticMetadataInPrivateCatalog(t *testing.T) {
	fixture := bundleFixture(t)
	db, err := database.NewCatalogDB(fixture.root)
	require.NoError(t, err)
	path := filepath.Join(fixture.root, "data/raw/legacy-manifest.json")
	body := []byte(`{"timestamp":"2026-10-05T12:00:00Z","actions":[]}`)
	require.NoError(t, os.WriteFile(path, body, 0600))
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	kind := "manifest"
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: "legacy-manifest", StoredPath: path, MetadataPath: path + ".meta", Format: "json", Schema: "pudl/mu.#Manifest", Origin: "legacy", ImportTimestamp: time.Now(), ContentHash: &hash, EntryType: &kind}))
	require.NoError(t, db.Close())
	archive := filepath.Join(t.TempDir(), "legacy.tar.zst")
	require.NoError(t, Export(context.Background(), fixture.root, archive, 0))
	manifest, _ := readBundleArchive(t, archive)
	require.NotEmpty(t, manifest.Notes)
	require.Contains(t, strings.Join(manifest.Notes, " "), "legacy manifest")
	unchanged, err := database.OpenCatalogDBReadOnly(fixture.root)
	require.NoError(t, err)
	entry, err := unchanged.GetEntry("legacy-manifest")
	require.NoError(t, err)
	require.Equal(t, path+".meta", entry.MetadataPath)
	require.NoError(t, unchanged.Close())
	_, err = os.Stat(path + ".meta")
	require.True(t, os.IsNotExist(err))
	root := filepath.Join(t.TempDir(), "restored")
	require.NoError(t, Restore(context.Background(), archive, root, 0))
	restored, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer restored.Close()
	entry, err = restored.GetEntry("legacy-manifest")
	require.NoError(t, err)
	require.Empty(t, entry.MetadataPath)
	raw, err := os.ReadFile(entry.StoredPath)
	require.NoError(t, err)
	require.Equal(t, body, raw)
	// An ordinary missing metadata reference must continue to fail closed.
	source, err := database.NewCatalogDB(fixture.root)
	require.NoError(t, err)
	_, err = source.DB().Exec("UPDATE catalog_entries SET entry_type=NULL WHERE id='legacy-manifest'")
	require.NoError(t, err)
	require.NoError(t, source.Close())
	existing := filepath.Join(t.TempDir(), "existing.tar.zst")
	require.NoError(t, os.WriteFile(existing, []byte("preserve"), 0600))
	require.Error(t, Export(context.Background(), fixture.root, existing, 0))
	raw, err = os.ReadFile(existing)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(raw))
}
