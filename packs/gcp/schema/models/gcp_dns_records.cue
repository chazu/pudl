package models

// DNS record sets (gcp-dns-record-sets): every record set of every Cloud DNS
// zone in each project, through gcloud only. Cloud Asset Inventory exports no
// record set of a project's managed zones; what it lists under
// dns.googleapis.com/ResourceRecordSet are the records of GKE's
// cluster-scoped DNS zones (pods and services, which come and go), which the
// pack does not catalog. The helper (populators/gcp/dns-record-sets.sh)
// lists the project's zones, then each zone's record sets, adding the zone.

_gcpSchemas: "dns-record-sets": "gcp.#DNSRecordSet"

_gcpRuns: "dns-record-sets": [{
	argv: ["sh", "../../populators/gcp/dns-record-sets.sh", "{{item}}"]
	set: project: "{{item}}"
}]
