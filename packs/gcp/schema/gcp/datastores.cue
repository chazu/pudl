package gcp

// Managed data stores beyond Cloud SQL (data.cue): AlloyDB clusters and
// instances, Memorystore for Redis instances and Firestore databases, plus
// Cloud SQL backups. The first four are named by full resource name
// (projects/P/locations/L/...), which is their identity; the shapes
// (schema/models/gcp_datastores.cue) add the location the name holds.
// Networks are in path form (projects/P/global/networks/N).

// A record's location (region), as its shape adds it.
_gcpLocationGraph: _gcpGraph & {name: {path: "name", last_segment: true}, region: "location"}

// `gcloud alloydb clusters list --format=json`.
#AlloyDBCluster: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.alloydb.cluster"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpLocationGraph
		graph: display: {code: "ADB", label: "AlloyDB cluster", class: "data"}
		// Set only when a cluster is created; the API does not return it.
		sensitive_fields: ["initialUser.password"]
		facts: gcp_alloydb_cluster: args: {
			project:           "project"
			name:              "name"
			state:             {path: "state", default: ""}
			database_version:  {path: "databaseVersion", default: ""}
			automated_backup:  {path: "automatedBackupPolicy.enabled", default: false}
			continuous_backup: {path: "continuousBackupConfig.enabled", default: false}
		}
		// The VPC of a private-services-access cluster; absent with Private
		// Service Connect.
		facts: gcp_alloydb_cluster_network: args: network: {
			path:    "networkConfig.network"
			rewrite: [{match: "^(projects/[^/]+/global/networks/[^/]+)$", replace: "$1"}]
		}
		// The customer-managed key, if any.
		facts: gcp_alloydb_cluster_key: args: key: {
			path:    "encryptionConfig.kmsKeyName"
			rewrite: [{match: "^(projects/.+/cryptoKeys/[^/]+)", replace: "$1"}]
		}
	}
	project:   _gcpProjectID
	name:      =~"^projects/[^/]+/locations/[^/]+/clusters/[^/]+$" @pudl(match)
	location?: string
	...
}

// `gcloud alloydb instances list --format=json` (every cluster and region).
#AlloyDBInstance: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.alloydb.instance"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpLocationGraph
		graph: display: {code: "ADBI", label: "AlloyDB instance", class: "data"}
		facts: gcp_alloydb_instance: args: {
			project: "project"
			name:    "name"
			cluster: {path: "name", rewrite: [{match: "^(.*)/instances/[^/]+$", replace: "$1"}]}
			type:    {path: "instanceType", default: ""}
			// A missing enablePublicIp means no public IP.
			public_ip: {path: "networkConfig.enablePublicIp", default: false}
			ssl_mode:  {path: "clientConnectionConfig.sslConfig.sslMode", default: ""}
		}
	}
	project:   _gcpProjectID
	name:      =~"^projects/[^/]+/locations/[^/]+/clusters/[^/]+/instances/[^/]+$" @pudl(match)
	location?: string
	...
}

// `gcloud redis instances list --region=- --format=json`. The AUTH string
// is not part of the listing (get-auth-string prints it).
#RedisInstance: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.redis.instance"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpLocationGraph
		graph: display: {code: "RDS", label: "Redis instance", class: "data"}
		facts: gcp_redis_instance: args: {
			project: "project"
			name:    "name"
			tier:    "tier"
			version: {path: "redisVersion", default: ""}
			// AUTH and in-transit encryption are off unless set.
			auth:               {path: "authEnabled", default: false}
			transit_encryption: {path: "transitEncryptionMode", default: "DISABLED"}
		}
		facts: gcp_redis_instance_network: args: network: {
			path:    "authorizedNetwork"
			rewrite: [{match: "^(projects/[^/]+/global/networks/[^/]+)$", replace: "$1"}]
		}
	}
	project:      _gcpProjectID
	name:         =~"^projects/[^/]+/locations/[^/]+/instances/[^/]+$" @pudl(match)
	location?:    string
	redisVersion: string                                               @pudl(match)
	tier:         string
	...
}

// `gcloud firestore databases list --format=json`.
#FirestoreDatabase: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.firestore.database"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "name", last_segment: true}, region: "locationId"}
		graph: display: {code: "FS", label: "Firestore database", class: "data"}
		facts: gcp_firestore_database: args: {
			project:           "project"
			name:              "name"
			type:              {path: "type", default: ""}
			location:          {path: "locationId", default: ""}
			pitr:              {path: "pointInTimeRecoveryEnablement", default: "POINT_IN_TIME_RECOVERY_DISABLED"}
			delete_protection: {path: "deleteProtectionState", default: "DELETE_PROTECTION_DISABLED"}
		}
	}
	project:    _gcpProjectID
	name:       =~"^projects/[^/]+/databases/[^/]+$" @pudl(match)
	locationId: string
	...
}

// Cloud SQL backups, the newest _gcpBackupHistory per instance
// (schema/models/gcp_sql_backups.cue). A backup (`gcloud sql backups list`,
// the Backup API) records the instance's settings when it was taken and
// outlives the instance; a backup run (`gcloud sql backups list
// --instance=I`) is the same backup as the instance's backup history shows
// it, and goes with the instance.
#SQLBackup: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.sql.backup"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["instanceSettings.settings.userLabels.team"]}
		graph: _gcpGraph & {name: {path: "name", last_segment: true}, region: "location"}
		graph: display: {code: "SQLB", label: "Cloud SQL backup", class: "data"}
		graph: inventory: collapsed: true
		facts: gcp_sql_backup: args: {
			project:  "project"
			instance: "instance"
			name:     "name"
			type:     {path: "type", default: ""}
			state:    {path: "state", default: ""}
			end_time: {path: "backupInterval.endTime", default: ""}
		}
	}
	kind:     "sql#backup" @pudl(match)
	project:  _gcpProjectID
	name:     =~"^projects/[^/]+/backups/[^/]+$"
	instance: string
	...
}

#SQLBackupRun: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.sql.backup_run"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: "id", region: "location"}
		graph: display: {code: "SQLR", label: "Cloud SQL backup run", class: "data"}
		graph: inventory: collapsed: true
		facts: gcp_sql_backup_run: args: {
			project:  "project"
			instance: "instance"
			id:       "id"
			type:     {path: "type", default: ""}
			status:   {path: "status", default: ""}
			end_time: {path: "endTime", default: ""}
		}
	}
	kind:     "sql#backupRun" @pudl(match)
	project:  _gcpProjectID
	selfLink: string
	instance: string
	id:       string
	...
}
