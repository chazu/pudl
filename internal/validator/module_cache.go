package validator

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"sync/atomic"

	"cuelang.org/go/cue"
)

// Schema state is compiled once per invocation and shared.
//
// LoadAllModules is the expensive primitive under every consumer of schema
// state — the inferrer, the chain validator, model loading, the schema commands.
// It parses and CUE-compiles every module in a directory, and on a missing
// dependency it shells out to `cue mod tidy` and retries. A single `pudl run`
// used to pay that at least twice for the same unchanged files, and
// `listModelsIn` once per model search directory.
//
// Two levels of memoization, both needed: per loader, so a caller that loads
// twice compiles once even with a private loader; and per path via SharedLoader,
// so two *callers* naming the same directory share the compile.

var (
	sharedLoadersMu sync.Mutex
	sharedLoaders   = map[string]*CUEModuleLoader{}

	// moduleLoads counts real compiles — the work the memo exists to avoid.
	// Exposed so the saving is observable rather than asserted by faith.
	moduleLoads atomic.Int64
)

// ModuleLoadCount reports how many schema directories this process has actually
// compiled, as opposed to served from the memo.
func ModuleLoadCount() int64 { return moduleLoads.Load() }

// SharedLoader returns the process-wide loader for a schema path, creating it on
// first use.
//
// The loader is shared rather than just its modules because a loader owns a
// cue.Context, and CUE values from different contexts cannot safely be unified.
// Caching modules while callers kept private contexts would hand a caller values
// built in someone else's context — a hazard the uncached code does not have.
func SharedLoader(schemaPath string) *CUEModuleLoader {
	key, err := filepath.Abs(schemaPath)
	if err != nil {
		key = schemaPath
	}

	sharedLoadersMu.Lock()
	defer sharedLoadersMu.Unlock()

	if loader, ok := sharedLoaders[key]; ok {
		return loader
	}
	loader := NewCUEModuleLoader(schemaPath)
	sharedLoaders[key] = loader
	return loader
}

// ResetSharedLoaders drops the shared loaders. For tests that stage schema
// directories at the same temp path across cases.
func ResetSharedLoaders() {
	sharedLoadersMu.Lock()
	defer sharedLoadersMu.Unlock()
	sharedLoaders = map[string]*CUEModuleLoader{}
}

// Fingerprint is the loader's view of its schema directory's current state.
//
// Exposed so a caller memoizing something *derived* from a load — the schema
// inferrer's inheritance graph and identity metadata — can invalidate on the
// same signal rather than inventing a second one that could disagree.
func (loader *CUEModuleLoader) Fingerprint() (string, error) {
	return fingerprintDir(loader.schemaPath)
}

// cachedModules memoizes one loader's compile, invalidated by a file
// fingerprint.
//
// A path-keyed memo alone would be wrong within a single invocation rather than
// merely stale: `pudl schema new` writes a schema and then infers against it, so
// serving the pre-write compile would be a correctness bug introduced by an
// optimization.
type cachedModules struct {
	fingerprint string
	modules     map[string]*LoadedModule
}

// loadAllModulesCached is LoadAllModules with the memo in front of it.
func (loader *CUEModuleLoader) loadAllModulesCached() (map[string]*LoadedModule, error) {
	loader.cacheMu.Lock()
	defer loader.cacheMu.Unlock()

	fingerprint, err := fingerprintDir(loader.schemaPath)
	if err != nil {
		// An unreadable schema directory is the uncached path's problem to report,
		// not the cache's to hide.
		return loader.loadAllModulesUncached()
	}

	if loader.cache != nil && loader.cache.fingerprint == fingerprint {
		return copyModules(loader.cache.modules), nil
	}

	modules, err := loader.loadAllModulesUncached()
	if err != nil {
		// A failed load is not cached. It is exactly the case where the caller may
		// have fixed the problem — a dependency fetched, a syntax error corrected —
		// between attempts, and caching the failure would hide the fix until the
		// next process.
		return nil, err
	}

	loader.cache = &cachedModules{fingerprint: fingerprint, modules: modules}
	return copyModules(modules), nil
}

// copyModules shallow-copies the module map and each module's two maps, so a
// caller writing into what it was handed cannot poison later readers. cue.Value
// is immutable, so only the map spines need copying — a cost proportional to the
// schema count, against a CUE compile.
func copyModules(modules map[string]*LoadedModule) map[string]*LoadedModule {
	out := make(map[string]*LoadedModule, len(modules))
	for name, module := range modules {
		if module == nil {
			out[name] = nil
			continue
		}
		clone := *module
		clone.Schemas = make(map[string]cue.Value, len(module.Schemas))
		for k, v := range module.Schemas {
			clone.Schemas[k] = v
		}
		clone.Metadata = make(map[string]SchemaMetadata, len(module.Metadata))
		for k, v := range module.Metadata {
			clone.Metadata[k] = v
		}
		out[name] = &clone
	}
	return out
}

// fingerprintDir hashes every file under root by relative path, size and
// modification time.
//
// Modification time is not a perfect change signal: a same-nanosecond write of
// different content within one invocation would be missed. Size is in the hash
// too, and the write would have to come from pudl itself between two loads in
// the same process. Hashing contents instead would make the check cost
// proportional to the schema repo on every lookup, which is the cost the cache
// exists to avoid.
func fingerprintDir(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = path
		}
		fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", relative, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
