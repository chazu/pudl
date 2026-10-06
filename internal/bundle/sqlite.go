package bundle

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"modernc.org/sqlite"
)

// onlineSnapshot preserves SQLite rowids as well as the logical rows. Snapshot
// chronology uses rowids for equal timestamps, including pruning tombstones;
// VACUUM INTO may renumber those ids and is unsuitable for that contract.
func onlineSnapshot(ctx context.Context, db *sql.DB, destination string, maxBytes int64) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var pages, pageSize int64
	if err := conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil {
		return err
	}
	if pageSize <= 0 || maxBytes <= 0 || pages > maxBytes/pageSize {
		return fmt.Errorf("catalog exceeds bundle byte limit (%d)", maxBytes)
	}
	return conn.Raw(func(raw any) (resultErr error) {
		source, ok := raw.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("SQLite driver does not support online backup")
		}
		backup, err := source.NewBackup(destination)
		if err != nil {
			return err
		}
		defer func() {
			if err := backup.Finish(); resultErr == nil {
				resultErr = err
			}
		}()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err := backup.Step(128)
			if err != nil {
				return err
			}
			// The live database may grow after preflight. Bound staging growth
			// before taking another step (at most 128 pages of transient excess).
			info, err := os.Stat(destination)
			if err != nil {
				return err
			}
			if info.Size() > maxBytes {
				return fmt.Errorf("catalog exceeds bundle byte limit (%d)", maxBytes)
			}
			if !more {
				return nil
			}
		}
	})
}
