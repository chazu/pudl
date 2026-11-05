package models

// Cloud SQL backups (gcp-sql-backups, gcp-sql-backup-runs): the newest
// _gcpBackupHistory backups of each instance (gcp_pack.cue), through gcloud
// only. Cloud Asset Inventory lists every retained backup and a per-record
// transform cannot keep only the newest per instance, so the helpers
// (populators/gcp/sql-backups.sh, sql-backup-runs.sh) ask gcloud for them
// instance by instance.
//
// The two views of a backup: the Backup API (sql#backup) records the
// instance's settings at the time and keeps an instance's final backups
// after it is deleted; a backup run (sql#backupRun) is the instance's backup
// history and goes with it.

_gcpSchemas: {
	"sql-backups":     "gcp.#SQLBackup"
	"sql-backup-runs": "gcp.#SQLBackupRun"
}

_gcpRuns: {
	"sql-backups": [{
		argv: ["sh", "../../populators/gcp/sql-backups.sh", "{{item}}", "\(_gcpBackupHistory)"]
		set: project: "{{item}}"
	}]
	"sql-backup-runs": [{
		argv: ["sh", "../../populators/gcp/sql-backup-runs.sh", "{{item}}", "\(_gcpBackupHistory)"]
		set: project: "{{item}}"
	}]
}

#GCPSQLBackups:           _#GCPLive & {_type: "sql-backups"}
#GCPSQLBackupRuns:        _#GCPLive & {_type: "sql-backup-runs"}
#GCPFixtureSQLBackups:    _#GCPFixture & {_type: "sql-backups"}
#GCPFixtureSQLBackupRuns: _#GCPFixture & {_type: "sql-backup-runs"}
