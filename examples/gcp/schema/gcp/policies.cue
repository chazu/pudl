package gcp

import "list"

// Single-resource best practices, evaluated against every resource of the
// named type (docs/design/2026-11-01-compliance-policies.md). Policies are
// standalone definitions linked to their type by base_schema. Each repeats its
// type's identity_fields: pudl doctor and fact projection treat the policy as a
// member of the type's family and warn when they differ.
//
// Absence: a constrained field the record lacks makes the result unknown,
// unless the field carries absent="violation". That is used only where gcloud
// omits a field to mean the setting is off.
//
// Conditional rules use hidden fields with defaults (_internal, _rotates, ...)
// so the definition evaluates on its own: a comprehension over an optional
// field that is not set loses every rule annotation of the policy. A rule that
// must hold "unless X or Y" is written as a conflict on a field every record
// carries (the anchor), because a conditional requirement cannot detect an
// absent field. Nested structs end in "..." so a record's extra fields do not
// break the references these conditions make.

// Firewalls: no enabled INGRESS ALLOW rule open to the internet.
#FirewallBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "gcp.#Firewall"
		identity_fields: ["selfLink"]
		description: "No enabled ingress ALLOW rule from 0.0.0.0/0 or ::/0"
	}
	direction?:    _
	disabled?:     _
	allowed?:      _
	sourceRanges?: _ @pudl(rule="gcp-firewall-world-ingress", severity="high")
	_direction:    *"" | string
	_disabled:     *false | bool
	if direction != _|_ {_direction: direction}
	if disabled != _|_ {_disabled: disabled}
	if _direction == "INGRESS" && allowed != _|_ && !_disabled {
		sourceRanges?: [...(!="0.0.0.0/0" & !="::/0")]
	}
}

// External addresses: a RESERVED external address is billed and unused.
// Internal reservations are free and are not flagged.
#AddressBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "gcp.#Address"
		identity_fields: ["selfLink"]
		description: "External addresses are in use"
	}
	addressType?: _
	status?:      _ @pudl(rule="gcp-address-unused", severity="low")
	_internal:    *false | bool
	if addressType != _|_ {_internal: addressType == "INTERNAL"}
	if !_internal {status?: "IN_USE"}
}

// Buckets. gcloud prints public_access_prevention as "inherited" when it is
// not enforced; an absent value is treated as not enforced. An absent
// uniform_bucket_level_access means fine-grained ACLs are in use.
#BucketBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "gcp.#Bucket"
		identity_fields: ["name"]
		description: "Public access prevention enforced, uniform bucket-level access"
	}
	public_access_prevention:    "enforced" @pudl(rule="gcp-bucket-public-access-prevention", severity="high", absent="violation")
	uniform_bucket_level_access: true       @pudl(rule="gcp-bucket-uniform-access", severity="medium", absent="violation")
}

// Cloud SQL. A missing backupConfiguration or enabled flag means no automated
// backups. Point-in-time recovery is pointInTimeRecoveryEnabled (PostgreSQL,
// SQL Server) or binaryLogEnabled (MySQL); when neither is true the violation
// is reported on databaseVersion, the field that decides which flag applies.
// A missing ipv4Enabled is treated as a public IP (the Cloud SQL default).
#SQLInstanceBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "gcp.#SQLInstance"
		identity_fields: ["selfLink"]
		description: "Backups and point-in-time recovery on, no public or world-open access"
	}
	databaseVersion?: _ @pudl(rule="gcp-sql-pitr", severity="medium")
	settings: {
		backupConfiguration: {
			enabled:                     true @pudl(rule="gcp-sql-backups", severity="high")
			pointInTimeRecoveryEnabled?: _
			binaryLogEnabled?:           _
			...
		} @pudl(absent="violation")
		ipConfiguration: {
			ipv4Enabled: false @pudl(rule="gcp-sql-public-ip", severity="medium", absent="violation")
			authorizedNetworks?: [...{
				value?: (!="0.0.0.0/0" & !="::/0") @pudl(rule="gcp-sql-world-authorized-network", severity="critical")
				...
			}]
			...
		}
		...
	}
	_backups: settings.backupConfiguration
	_pitrFlags: [
		if _backups.pointInTimeRecoveryEnabled != _|_ {_backups.pointInTimeRecoveryEnabled},
		if _backups.binaryLogEnabled != _|_ {_backups.binaryLogEnabled},
	]
	if !list.Contains(_pitrFlags, true) {databaseVersion?: "point-in-time recovery must be enabled"}
}

// KMS keys. Automatic rotation exists only for symmetric ENCRYPT_DECRYPT keys,
// so a key without rotationPeriod is a violation, reported on purpose, only
// when its purpose is ENCRYPT_DECRYPT.
#KMSKeyBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "gcp.#KMSKey"
		identity_fields: ["name"]
		description: "Symmetric keys rotate automatically"
	}
	purpose?:        _ @pudl(rule="gcp-kms-rotation", severity="medium")
	rotationPeriod?: _
	_rotates:        *false | bool
	if rotationPeriod != _|_ {_rotates: true}
	if !_rotates {purpose?: !="ENCRYPT_DECRYPT"}
}

// GKE. A missing privateClusterConfig or workloadIdentityConfig means the
// feature is off. The control plane must have a private endpoint or authorized
// networks; when it has neither, the violation is reported on endpoint, the
// address that is then reachable from anywhere.
#GKEClusterBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "gcp.#GKECluster"
		identity_fields: ["selfLink"]
		description: "Private nodes, restricted control plane, Workload Identity"
	}
	privateClusterConfig: {
		enablePrivateNodes:     true @pudl(rule="gke-private-nodes", severity="high")
		enablePrivateEndpoint?: _
		...
	} @pudl(absent="violation")
	workloadIdentityConfig: {
		workloadPool: =~"\\.svc\\.id\\.goog$" @pudl(rule="gke-workload-identity", severity="medium")
		...
	} @pudl(absent="violation")
	masterAuthorizedNetworksConfig?: _
	endpoint?:                       _ @pudl(rule="gke-control-plane-access", severity="high")
	_privateEndpoint:                *false | bool
	_authorizedNetworks:             *false | bool
	if privateClusterConfig.enablePrivateEndpoint != _|_ {
		_privateEndpoint: privateClusterConfig.enablePrivateEndpoint
	}
	if masterAuthorizedNetworksConfig.enabled != _|_ {
		_authorizedNetworks: masterAuthorizedNetworksConfig.enabled
	}
	if !_privateEndpoint && !_authorizedNetworks {
		endpoint?: "control plane needs a private endpoint or authorized networks"
	}
}
