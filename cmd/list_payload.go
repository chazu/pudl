package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
)

type payloadFields struct {
	Values  map[string]any
	Missing []string
}

func makePayloadMatcher(ctx context.Context, where, selectFields []string) (func(database.CatalogEntry) (bool, error), map[string]payloadFields, error) {
	type condition struct {
		path  fieldpath.Path
		value any
	}
	conditions := make([]condition, 0, len(where))
	for _, raw := range where {
		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, nil, fmt.Errorf("--where requires path=value")
		}
		path, err := fieldpath.Parse(key)
		if err != nil {
			return nil, nil, err
		}
		conditions = append(conditions, condition{path, parseCLIValue(value)})
	}
	paths := make([]fieldpath.Path, len(selectFields))
	for i, name := range selectFields {
		path, err := fieldpath.Parse(name)
		if err != nil {
			return nil, nil, err
		}
		paths[i] = path
	}
	fields := map[string]payloadFields{}
	budget := int64(256 << 20)
	match := func(entry database.CatalogEntry) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if entry.CollectionType != nil && *entry.CollectionType == "collection" {
			return false, nil
		}
		info, err := os.Stat(entry.StoredPath)
		if err != nil {
			return false, err
		}
		if info.Size() > 64<<20 || info.Size() > budget {
			return false, fmt.Errorf("payload inspection exceeds its 64 MiB record or 256 MiB scan budget; narrow --schema or --collection-id")
		}
		budget -= info.Size()
		data, err := loadReinferData(entry.StoredPath, entry.Format)
		if err != nil {
			return false, fmt.Errorf("read entry %s: %w", entry.ID, err)
		}
		for _, c := range conditions {
			matches := false
			for _, value := range c.path.LookupAll(data) {
				if acute.ValuesEqual(value, c.value) {
					matches = true
					break
				}
			}
			if !matches {
				return false, nil
			}
		}
		if len(paths) > 0 {
			selected := payloadFields{Values: map[string]any{}}
			for i, path := range paths {
				if path.HasWildcard() {
					selected.Values[selectFields[i]] = path.LookupAll(data)
					continue
				}
				value, ok := path.Lookup(data)
				if !ok {
					selected.Missing = append(selected.Missing, selectFields[i])
				} else {
					selected.Values[selectFields[i]] = value
				}
			}
			fields[entry.ID] = selected
		}
		return true, nil
	}
	return match, fields, nil
}
