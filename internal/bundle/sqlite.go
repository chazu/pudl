package bundle

import (
	"context"
	"database/sql"
	"fmt"

	"modernc.org/sqlite"
)

// onlineSnapshot preserves SQLite rowids as well as the logical rows. Snapshot
// chronology uses rowids for equal timestamps, including pruning tombstones;
// VACUUM INTO may renumber those ids and is unsuitable for that contract.
func onlineSnapshot(ctx context.Context, db *sql.DB, destination string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
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
			if !more {
				return nil
			}
		}
	})
}
