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
		// The VPC of a private IP, in path form; absent without one.
		facts: gcp_sql_network: args: {
			self_link: "selfLink"
			network:   "settings.ipConfiguration.privateNetwork"
		}
	}
	kind:            "sql#instance"
	project:         string
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
	}
	project:                      string
	name:                         string
	storage_url:                  =~"^gs://"
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
	}
	project: string
	name:    =~"^projects/[^/]+/locations/[^/]+/keyRings/[^/]+$"
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
		facts: gcp_kms_key: args: {
			project:       "project"
			name:          "name"
			primary_state: "primary.state"
		}
	}
	project:         string
	name:            =~"^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$"
	purpose:         string
	rotationPeriod?: string
	primary?: {state?: string, ...}
	...
}
