package models

import (
	"list"
	"regexp"
)

// Collectors of load balancing and edge security (schema/gcp/loadbalancing.cue).
// Each searches CAI and falls back to its gcloud list command, per project.
//
// Live parity (2026-11-05, field names only): CAI's copy equals gcloud's
// except what _gcpComputeShape drops (kind, fingerprint) and:
//   - backend services: usedBy lists the same references in another order;
//   - forwarding rules: gcloud adds attachedExtensions;
//   - health checks: CAI can spell a region URL with compute/v2;
//   - Cloud Armor policies: gcloud adds each rule's kind.
// The shapes below normalize these.

_gcpAssetTypes: {
	"forwarding-rules":        "compute.googleapis.com/ForwardingRule"
	"target-http-proxies":     "compute.googleapis.com/TargetHttpProxy"
	"target-https-proxies":    "compute.googleapis.com/TargetHttpsProxy"
	"url-maps":                "compute.googleapis.com/UrlMap"
	"backend-services":        "compute.googleapis.com/BackendService"
	"healthchecks":           "compute.googleapis.com/HealthCheck"
	"http-healthchecks":      "compute.googleapis.com/HttpHealthCheck"
	"network-endpoint-groups": "compute.googleapis.com/NetworkEndpointGroup"
	"ssl-certificates":        "compute.googleapis.com/SslCertificate"
	"ssl-policies":            "compute.googleapis.com/SslPolicy"
	"security-policies":       "compute.googleapis.com/SecurityPolicy"
}

_gcpSchemas: {
	"forwarding-rules":        "gcp.#ForwardingRule"
	"target-http-proxies":     "gcp.#TargetHTTPProxy"
	"target-https-proxies":    "gcp.#TargetHTTPSProxy"
	"url-maps":                "gcp.#URLMap"
	"backend-services":        "gcp.#BackendService"
	"healthchecks":           "gcp.#HealthCheck"
	"http-healthchecks":      "gcp.#HTTPHealthCheck"
	"network-endpoint-groups": "gcp.#NetworkEndpointGroup"
	"ssl-certificates":        "gcp.#SSLCertificate"
	"ssl-policies":            "gcp.#SSLPolicy"
	"security-policies":       "gcp.#SecurityPolicy"
}

_gcpCommands: {
	"forwarding-rules": ["compute", "forwarding-rules", "list"]
	"target-http-proxies": ["compute", "target-http-proxies", "list"]
	"target-https-proxies": ["compute", "target-https-proxies", "list"]
	"url-maps": ["compute", "url-maps", "list"]
	"backend-services": ["compute", "backend-services", "list"]
	"healthchecks": ["compute", "health-checks", "list"]
	"http-healthchecks": ["compute", "http-health-checks", "list"]
	"network-endpoint-groups": ["compute", "network-endpoint-groups", "list"]
	"ssl-certificates": ["compute", "ssl-certificates", "list"]
	"ssl-policies": ["compute", "ssl-policies", "list"]
	"security-policies": ["compute", "security-policies", "list"]
}

_gcpShapes: {
	"target-http-proxies":     _gcpComputeShape & {_kind: "compute#targetHttpProxy"}
	"target-https-proxies":    _gcpComputeShape & {_kind: "compute#targetHttpsProxy"}
	"url-maps":                _gcpComputeShape & {_kind: "compute#urlMap"}
	"http-healthchecks":      _gcpComputeShape & {_kind: "compute#httpHealthCheck"}
	"network-endpoint-groups": _gcpComputeShape & {_kind: "compute#networkEndpointGroup"}
	"ssl-certificates":        _gcpComputeShape & {_kind: "compute#sslCertificate"}
	"ssl-policies":            _gcpComputeShape & {_kind: "compute#sslPolicy"}

	"forwarding-rules": {
		in: {...}
		let r = in
		out: (_gcpComputeShape & {_kind: "compute#forwardingRule", in: {
			for k, v in r if k != "attachedExtensions" {(k): v}
		}}).out
	}
	"backend-services": {
		in: {...}
		let r = in
		out: (_gcpComputeShape & {_kind: "compute#backendService", in: {
			for k, v in r if k != "usedBy" {(k): v}
			if r.usedBy != _|_ {
				usedBy: list.Sort(r.usedBy, {x: _, y: _, less: x.reference < y.reference})
			}
		}}).out
	}
	"healthchecks": {
		in: {...}
		let r = in
		out: (_gcpComputeShape & {_kind: "compute#healthCheck", in: {
			for k, v in r if k != "region" {(k): v}
			if r.region != _|_ {
				region: regexp.ReplaceAll("/compute/v2/", r.region, "/compute/v1/")
			}
		}}).out
	}
	"security-policies": {
		in: {...}
		let r = in
		out: (_gcpComputeShape & {_kind: "compute#securityPolicy", in: {
			for k, v in r if k != "rules" {(k): v}
			if r.rules != _|_ {
				rules: [for rule in r.rules {{for k, v in rule if k != "kind" {(k): v}}}]
			}
		}}).out
	}
}
