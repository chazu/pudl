package database

import (
	"context"
	"database/sql"
	"strings"

	"github.com/chazu/pudl/internal/errors"
)

// entryFilterClause renders the WHERE clause (empty when nothing filters) and
// its arguments for a catalog_entries read.
func entryFilterClause(filters FilterOptions) (string, []interface{}) {
	var whereConditions []string
	var args []interface{}

	if filters.Schema != "" {
		cond, condArgs := SchemaFilterCondition(filters.Schema)
		whereConditions = append(whereConditions, cond)
		args = append(args, condArgs...)
	}
	if filters.Origin != "" {
		whereConditions = append(whereConditions, `origin LIKE ? ESCAPE '\'`)
		args = append(args, "%"+EscapeLike(filters.Origin)+"%")
	}
	if filters.Format != "" {
		whereConditions = append(whereConditions, `format LIKE ? ESCAPE '\'`)
		args = append(args, "%"+EscapeLike(filters.Format)+"%")
	}
	if filters.CollectionID != "" {
		whereConditions = append(whereConditions, "id IN (SELECT item_id FROM collection_memberships WHERE collection_id = ?)")
		args = append(args, filters.CollectionID)
	}
	if filters.CollectionType != "" {
		whereConditions = append(whereConditions, "collection_type = ?")
		args = append(args, filters.CollectionType)
	}
	if filters.ItemID != "" {
		whereConditions = append(whereConditions, "item_id = ?")
		args = append(args, filters.ItemID)
	}
	if len(filters.EntryTypes) > 0 {
		placeholders := make([]string, len(filters.EntryTypes))
		for i, t := range filters.EntryTypes {
			placeholders[i] = "?"
			args = append(args, t)
		}
		whereConditions = append(whereConditions, "entry_type IN ("+strings.Join(placeholders, ",")+")")
	}

	if len(whereConditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(whereConditions, " AND "), args
}

// EachEntryContext streams the entries matching filters to fn, oldest import
// first, without holding the catalog in memory. The read keeps a connection
// open until it returns, so fn should not query c. An error from fn stops the
// scan and is returned.
func (c *CatalogDB) EachEntryContext(ctx context.Context, filters FilterOptions, fn func(CatalogEntry) error) error {
	where, args := entryFilterClause(filters)
	rows, err := c.db.QueryContext(ctx, "SELECT "+entrySelect("catalog_entries")+
		" FROM catalog_entries "+where+" ORDER BY import_timestamp ASC, id ASC", args...)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to query catalog entries", err)
	}
	defer rows.Close()
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "Failed to scan catalog entry", err)
		}
		if err := fn(*entry); err != nil {
			return err
		}
	}
	return rows.Err()
}

// EachCurrentFactContext streams every currently-valid fact to fn, by ID,
// with the same connection caveat as EachEntryContext.
func (c *CatalogDB) EachCurrentFactContext(ctx context.Context, fn func(Fact) error) error {
	rows, err := c.db.QueryContext(ctx, `SELECT id, relation, args, source, provenance FROM current_facts ORDER BY id`)
	if err != nil {
		return errors.WrapError(errors.ErrCodeDatabaseError, "failed to query current facts", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f Fact
		var source, provenance sql.NullString
		if err := rows.Scan(&f.ID, &f.Relation, &f.Args, &source, &provenance); err != nil {
			return errors.WrapError(errors.ErrCodeDatabaseError, "failed to scan current fact", err)
		}
		f.Source, f.Provenance = source.String, provenance.String
		if err := fn(f); err != nil {
			return err
		}
	}
	return rows.Err()
}
