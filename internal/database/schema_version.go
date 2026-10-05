package database

// LatestMigrationVersion is the newest catalog schema this binary understands.
func LatestMigrationVersion() int { return migrations[len(migrations)-1].version }
