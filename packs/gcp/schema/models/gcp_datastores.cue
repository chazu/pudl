package models

import (
	"list"
	"regexp"
)

// Collectors of managed data stores (schema/gcp/datastores.cue): AlloyDB
// clusters and instances, Redis instances and Firestore databases. Each
// searches CAI and falls back to its gcloud list command, per project.
//
// Live parity (2026-11-05, values compared): CAI's copy equals gcloud's
// except that
//   - AlloyDB: gcloud adds updateTime (which moves without a configuration
//     change) and reconciling;
//   - Redis: gcloud adds availableMaintenanceVersions and each server CA
//     certificate's PEM (CAI keeps its serial, fingerprint and dates);
//   - Firestore: CAI adds deleteTime (the zero time) and
//     enhancedTextSearchQueryMode; gcloud adds earliestVersionTime (which
//     moves every minute), etag, freeTier, keyPrefix, realtimeUpdatesMode
//     and versionRetentionPeriod.
// The shapes drop these, so each record holds what both sources print.

_gcpAssetTypes: {
	"alloydb-clusters":    "alloydb.googleapis.com/Cluster"
	"alloydb-instances":   "alloydb.googleapis.com/Instance"
	"redis-instances":     "redis.googleapis.com/Instance"
	"firestore-databases": "firestore.googleapis.com/Database"
}

_gcpSchemas: {
	"alloydb-clusters":    "gcp.#AlloyDBCluster"
	"alloydb-instances":   "gcp.#AlloyDBInstance"
	"redis-instances":     "gcp.#RedisInstance"
	"firestore-databases": "gcp.#FirestoreDatabase"
}

// AlloyDB lists every region (and every cluster's instances) by default;
// Redis needs --region=- for that.
_gcpCommands: {
	"alloydb-clusters":    ["alloydb", "clusters", "list"]
	"alloydb-instances":   ["alloydb", "instances", "list"]
	"redis-instances":     ["redis", "instances", "list", "--region=-"]
	"firestore-databases": ["firestore", "databases", "list"]
}

// A record named projects/P/locations/L/..., without the fields in _drop,
// with its location L.
_gcpLocatedShape: {
	_drop: [...string]
	in:    {...}
	let r = in
	out: {
		for k, v in r if !list.Contains(_drop, k) {(k): v}
		location: regexp.FindSubmatch("/locations/([^/]+)/", r.name)[1]
	}
}

_gcpShapes: {
	"alloydb-clusters":  _gcpLocatedShape & {_drop: ["updateTime", "reconciling"]}
	"alloydb-instances": _gcpLocatedShape & {_drop: ["updateTime", "reconciling"]}
	"redis-instances": {
		in: {...}
		let r = in
		out: (_gcpLocatedShape & {_drop: ["availableMaintenanceVersions"], in: {
			for k, v in r if k != "serverCaCerts" {(k): v}
			if r.serverCaCerts != _|_ {
				serverCaCerts: [for c in r.serverCaCerts {{for k, v in c if k != "cert" {(k): v}}}]
			}
		}}).out
	}
	"firestore-databases": {
		in: {...}
		let r = in
		out: {for k, v in r if !list.Contains(_gcpFirestoreOneSource, k) {(k): v}}
	}
}

_gcpFirestoreOneSource: [
	"deleteTime", "enhancedTextSearchQueryMode", "earliestVersionTime", "etag",
	"freeTier", "keyPrefix", "realtimeUpdatesMode", "versionRetentionPeriod",
]

#GCPAlloyDBClusters:           _#GCPLive & {_type: "alloydb-clusters"}
#GCPAlloyDBInstances:          _#GCPLive & {_type: "alloydb-instances"}
#GCPRedisInstances:            _#GCPLive & {_type: "redis-instances"}
#GCPFirestoreDatabases:        _#GCPLive & {_type: "firestore-databases"}
#GCPFixtureAlloyDBClusters:    _#GCPFixture & {_type: "alloydb-clusters"}
#GCPFixtureAlloyDBInstances:   _#GCPFixture & {_type: "alloydb-instances"}
#GCPFixtureRedisInstances:     _#GCPFixture & {_type: "redis-instances"}
#GCPFixtureFirestoreDatabases: _#GCPFixture & {_type: "firestore-databases"}
