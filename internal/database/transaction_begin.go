package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"modernc.org/sqlite"
	"time"
)

// beginImmediateContext avoids SQLite's uninterruptible busy-handler sleep.
// Retry lock acquisition ourselves, honoring both ctx and the configured wait.
// The connection's original busy timeout is restored before it is pooled.
func beginImmediateContext(ctx context.Context, conn *sql.Conn) (resultErr error) {
	var timeoutMS int
	if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeoutMS); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=0"); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanup, fmt.Sprintf("PRAGMA busy_timeout=%d", timeoutMS)); err != nil {
			if resultErr == nil {
				rollbackConn(cleanup, conn)
				resultErr = fmt.Errorf("restore transaction busy timeout: %w", err)
			}
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE")
		if err == nil {
			return nil
		}
		var sqlErr *sqlite.Error
		if !errors.As(err, &sqlErr) || (sqlErr.Code()&0xff != 5 && sqlErr.Code()&0xff != 6) {
			return err
		}
		if !time.Now().Before(deadline) {
			return err
		}
		delay := 20 * time.Millisecond
		if remaining := time.Until(deadline); remaining < delay {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
