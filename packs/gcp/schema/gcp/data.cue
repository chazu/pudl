package gcp

// Data stores and keys: Cloud SQL, Cloud Storage and Cloud KMS.

// `gcloud sql instances list --format=json`. Identity is the selfLink.
#SQLInstance: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.sql.instance"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["settings.userLabels.team"]}
		graph: _gcpGraph & {region: "region"}
		graph: display: {code: "SQL", label: "Cloud SQL", class: "data"}
		graph: network: region: true
		// The VPC of a private IP, in path form; absent without one.
		facts: gcp_sql_network: args: {
			self_link: "selfLink"
			network:   "settings.ipConfiguration.privateNetwork"
		}
		// One fact per authorized network, and whether the instance has a
		// public IP for it to reach.
		facts: gcp_sql_authorized_network: args: {
			range: "settings.ipConfiguration.authorizedNetworks[*].value"
			public: {path: "settings.ipConfiguration.ipv4Enabled", default: false}
		}
		// The customer-managed disk key, if any, as a key name (a key
		// version's /cryptoKeyVersions/N is dropped).
		facts: gcp_sql_key: args: key: {
			path: "diskEncryptionConfiguration.kmsKeyName"
			rewrite: [{match: "^(projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+)", replace: "$1"}]
		}
	}
	kind:            "sql#instance" @pudl(match)
	project:         _gcpProjectID
	name:            string
	selfLink:        string
	databaseVersion: string
	region?:         string
	settings: {
		backupConfiguration?: {...}
		ipConfiguration?: {
			ipv4Enabled?: bool
			authorizedNetworks?: [...{value: string, ...}]
			...
		}
		...
	}
	...
}

// `gcloud storage buckets list --format=json` (gcloud storage prints
// snake_case keys). Bucket names are globally unique.
#Bucket: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.storage.bucket"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpGraph & {region: "location"}
		graph: display: {code: "GCS", label: "Storage bucket", class: "data"}
		// The bucket's default customer-managed key, if any (a rewrite, so a
		// bucket without one yields no fact).
		facts: gcp_bucket_key: args: key: {path: "default_kms_key", rewrite: [{match: "^(projects/.+/cryptoKeys/[^/]+)$", replace: "$1"}]}
	}
	project:                      _gcpProjectID
	name:                         string
	storage_url:                  =~"^gs://" @pudl(match)
	location?:                    string
	public_access_prevention?:    string
	uniform_bucket_level_access?: bool
	...
}

// `gcloud kms keyrings list --location=L --format=json`. Identity is the
// fully qualified name projects/P/locations/L/keyRings/R.
#KMSKeyRing: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.kms.keyring"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "KRING", label: "KMS key ring", class: "security"}
	}
	project: _gcpProjectID
	name:    =~"^projects/[^/]+/locations/[^/]+/keyRings/[^/]+$" @pudl(match)
	...
}

// `gcloud kms keys list --keyring=projects/P/locations/L/keyRings/R
// --format=json`. Identity is the fully qualified key name, the form GKE uses
// in databaseEncryption.keyName.
#KMSKey: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.kms.key"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "KMS", label: "KMS key", class: "security"}
		facts: gcp_kms_key: args: {
			project:       "project"
			name:          "name"
			primary_state: "primary.state"
		}
		// The key ring a key belongs to: its name without /cryptoKeys/K.
		facts: gcp_kms_key_ring: args: ring: {path: "name", rewrite: [{match: "^(.*)/cryptoKeys/[^/]+$", replace: "$1"}]}
	}
	project:         _gcpProjectID
	name:            =~"^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$" @pudl(match)
	purpose:         string
	rotationPeriod?: string
	primary?: {state?: string, ...}
	...
}
