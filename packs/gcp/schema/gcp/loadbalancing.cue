package gcp

// Cloud Load Balancing and its edge security, as `gcloud compute ... list
// --format=json` prints them (global and regional resources together):
// forwarding rules, target HTTP(S) proxies, URL maps, backend services,
// health checks, network endpoint groups, SSL certificates and policies,
// and Cloud Armor security policies. Identity is the selfLink; references
// between them are compute URLs, which facts store in path form
// (projects/P/global/backendServices/B).
//
// Header values a load balancer adds to requests (backend custom headers,
// URL map and Cloud Armor header actions) can carry credentials meant for
// the backend, so they are redacted before storage, as is an IAP OAuth
// client secret (the API returns only its hash; declared regardless).

// A compute URL field in path form, and no fact when absent.
_gcpPath: {
	_p: string
	out: {path: _p, rewrite: _gcpComputePathRewrite}
}

// Where a URL map or route can add request headers.
_gcpHeaderValues: {
	_prefix: string
	out: "\(_prefix)headerAction.requestHeadersToAdd[*].headerValue"
}

_gcpLBGraph: _gcpComputeGraph & {region: {path: "region", last_segment: true}}

#ForwardingRule: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.forwarding_rule"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpLBGraph
		graph: display: {code: "FR", label: "Forwarding rule", class: "edge"}
		graph: network: region: true
		facts: gcp_forwarding_rule: args: {
			project:  "project"
			name:     "name"
			ip:       {path: "IPAddress", default: ""}
			protocol: {path: "IPProtocol", default: ""}
			scheme:   {path: "loadBalancingScheme", default: ""}
			ports:    {path: "portRange", default: "all"}
		}
		// What the rule sends traffic to: a target proxy or pool, or a
		// backend service (internal and network load balancers).
		facts: gcp_forwarding_rule_target: args: target: (_gcpPath & {_p: "target"}).out
		facts: gcp_forwarding_rule_backend: args: backend: (_gcpPath & {_p: "backendService"}).out
		facts: gcp_forwarding_rule_network: args: network: (_gcpPath & {_p: "network"}).out
	}
	kind:     "compute#forwardingRule" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

#TargetHTTPProxy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.target_http_proxy"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "THP", label: "Target HTTP proxy", class: "edge"}
		facts: gcp_target_proxy_url_map: args: url_map: (_gcpPath & {_p: "urlMap"}).out
	}
	kind:     "compute#targetHttpProxy" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

#TargetHTTPSProxy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.target_https_proxy"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "THPS", label: "Target HTTPS proxy", class: "edge"}
		facts: gcp_target_proxy_url_map: args: url_map: (_gcpPath & {_p: "urlMap"}).out
		facts: gcp_target_proxy_certificate: args: certificate: (_gcpPath & {_p: "sslCertificates[*]"}).out
		facts: gcp_target_proxy_ssl_policy: args: ssl_policy: (_gcpPath & {_p: "sslPolicy"}).out
	}
	kind:     "compute#targetHttpsProxy" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	sslCertificates?: [...string]
	...
}

#URLMap: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.url_map"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "URLM", label: "URL map", class: "edge"}
		sensitive_fields: [
			(_gcpHeaderValues & {_prefix: ""}).out,
			(_gcpHeaderValues & {_prefix: "defaultRouteAction.weightedBackendServices[*]."}).out,
			(_gcpHeaderValues & {_prefix: "pathMatchers[*]."}).out,
			(_gcpHeaderValues & {_prefix: "pathMatchers[*].defaultRouteAction.weightedBackendServices[*]."}).out,
			(_gcpHeaderValues & {_prefix: "pathMatchers[*].routeRules[*]."}).out,
			(_gcpHeaderValues & {_prefix: "pathMatchers[*].routeRules[*].routeAction.weightedBackendServices[*]."}).out,
			(_gcpHeaderValues & {_prefix: "pathMatchers[*].pathRules[*].routeAction.weightedBackendServices[*]."}).out,
		]
		// The backend service (or bucket) a request goes to when no rule
		// matches; path rules' services are not projected.
		facts: gcp_url_map_default_service: args: service: (_gcpPath & {_p: "defaultService"}).out
		facts: gcp_url_map_service: {
			each: "pathMatchers[*]"
			args: service: (_gcpPath & {_p: "defaultService"}).out
		}
	}
	kind:     "compute#urlMap" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

#BackendService: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.backend_service"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "BES", label: "Backend service", class: "edge"}
		sensitive_fields: ["customRequestHeaders[*]", "iap.oauth2ClientSecret"]
		facts: gcp_backend_service: args: {
			project:  "project"
			name:     "name"
			scheme:   {path: "loadBalancingScheme", default: ""}
			protocol: {path: "protocol", default: ""}
			iap:      {path: "iap.enabled", default: false}
			logging:  {path: "logConfig.enable", default: false}
		}
		// One fact per backend: the instance group or endpoint group.
		facts: gcp_backend_service_backend: args: group: (_gcpPath & {_p: "backends[*].group"}).out
		facts: gcp_backend_service_health_check: args: health_check: (_gcpPath & {_p: "healthChecks[*]"}).out
		facts: gcp_backend_service_security_policy: args: policy: (_gcpPath & {_p: "securityPolicy"}).out
	}
	kind:     "compute#backendService" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

#HealthCheck: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.health_check"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "HC", label: "Health check", class: "edge"}
		graph: inventory: collapsed: true
		facts: gcp_health_check: args: {project: "project", name: "name", type: {path: "type", default: ""}}
	}
	kind:     "compute#healthCheck" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

// Legacy HTTP health checks, still used by target pools and some older
// backend services.
#HTTPHealthCheck: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.http_health_check"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph
		graph: display: {code: "HHC", label: "Legacy HTTP health check", class: "edge"}
		graph: inventory: collapsed: true
		facts: gcp_http_health_check: args: {project: "project", name: "name"}
	}
	kind:     "compute#httpHealthCheck" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

#NetworkEndpointGroup: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.network_endpoint_group"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpComputeGraph & {region: {path: "region", last_segment: true}, zone: {path: "zone", last_segment: true}}
		graph: display: {code: "NEG", label: "Network endpoint group", class: "edge"}
		facts: gcp_neg: args: {
			project: "project"
			name:    "name"
			type:    {path: "networkEndpointType", default: ""}
			// A serverless NEG names its Cloud Run service.
			run_service: {path: "cloudRun.service", default: ""}
		}
		facts: gcp_neg_network: args: network: (_gcpPath & {_p: "network"}).out
	}
	kind:     "compute#networkEndpointGroup" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

// The certificate is public (PEM); the private key is never returned.
#SSLCertificate: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.ssl_certificate"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "CERT", label: "SSL certificate", class: "security"}
		facts: gcp_ssl_certificate: args: {
			project: "project"
			name:    "name"
			type:    {path: "type", default: "SELF_MANAGED"}
			expires: {path: "expireTime", default: ""}
		}
		facts: gcp_ssl_certificate_domain: args: domain: "subjectAlternativeNames[*]"
	}
	kind:     "compute#sslCertificate" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

#SSLPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.ssl_policy"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "SSLP", label: "SSL policy", class: "security"}
		graph: security: control: true
		facts: gcp_ssl_policy: args: {
			project:     "project"
			name:        "name"
			min_tls:     {path: "minTlsVersion", default: "TLS_1_0"}
			profile:     {path: "profile", default: "COMPATIBLE"}
		}
	}
	kind:     "compute#sslPolicy" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	...
}

// A Cloud Armor security policy, attached to backend services.
#SecurityPolicy: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.compute.security_policy"
		identity_fields: ["selfLink"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpLBGraph
		graph: display: {code: "ARMR", label: "Cloud Armor policy", class: "security"}
		graph: security: control: true
		sensitive_fields: ["rules[*].headerAction.requestHeadersToAdds[*].headerValue"]
		facts: gcp_security_policy: args: {project: "project", name: "name", type: {path: "type", default: "CLOUD_ARMOR"}}
		// One fact per rule; the default rule has priority 2147483647.
		facts: gcp_security_policy_rule: {
			each: "rules[*]"
			args: {
				priority: "priority"
				action:   "action"
				preview:  {path: "preview", default: false}
			}
		}
	}
	kind:     "compute#securityPolicy" @pudl(match)
	project:  _gcpProjectID
	name:     string
	selfLink: string
	rules?: [...{priority: int, action: string, ...}]
	...
}
