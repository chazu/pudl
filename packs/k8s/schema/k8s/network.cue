package k8s

// Services and Ingresses, as `kubectl get -o json` prints them, shaped by
// the collectors (schema/models/k8s_objects.cue): metadata.managedFields
// dropped; every annotation keeps its key, the value of one whose key names
// a credential (auth, secret, token, password, passwd, credential, key) is
// replaced by "[REDACTED]", and a value that is a URL with a scheme loses
// its userinfo. Objects in system namespaces are not collected.
//
// Identity is the cluster, namespace and name, as for workloads
// (workloads.cue); the UID links the record to the same object seen by
// another pack.

// The last-applied annotation repeats the whole object as applied,
// annotation values included.
_k8sLastApplied: #"metadata.annotations."kubectl.kubernetes.io/last-applied-configuration""#

// The facts every namespaced object projects, under its own relation.
_k8sNamespacedArgs: {
	cluster:   "cluster"
	namespace: "metadata.namespace"
	name:      "metadata.name"
}

#Service: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.service"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "SVC", label: "Kubernetes service", class: "edge"}
		sensitive_fields: [_k8sLastApplied]
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: {
			k8s_service: args: _k8sNamespacedArgs & {
				type:       {path: "spec.type", default: "ClusterIP"}
				cluster_ip: {path: "spec.clusterIP", default: ""}
			}
			// One tuple per port; join k8s_service on resource_id.
			k8s_service_port: {
				each: "spec.ports[*]"
				args: {
					name:        {path: "name", default: ""}
					port:        "port"
					protocol:    {path: "protocol", default: "TCP"}
					target_port: {path: "targetPort", default: ""}
				}
			}
		}
	}
	apiVersion: "v1" @pudl(match)
	kind:       "Service" @pudl(match)
	cluster:    string & !=""
	metadata:   {name: string, namespace: string, uid: string, ...}
	spec?:      {...}
	status?:    {...}
}

#Ingress: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "k8s.ingress"
		identity_fields: ["cluster", "metadata.namespace", "metadata.name"]
		review: status: "draft"
		graph: name: "metadata.name"
		graph: display: {code: "ING", label: "Kubernetes ingress", class: "edge"}
		sensitive_fields: [_k8sLastApplied]
		same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]
		facts: {
			k8s_ingress: args: _k8sNamespacedArgs & {
				class: {path: "spec.ingressClassName", default: ""}
			}
			// One tuple per rule host; a rule without a host adds none.
			k8s_ingress_host: args: _k8sNamespacedArgs & {
				host: "spec.rules[*].host"
			}
			// One tuple per path of a rule. host and port are set on every
			// path by the collectors: its rule's host ("" for none), and its
			// service backend's port number or name. The pack's rule
			// k8s_ingress_backend joins these with k8s_ingress.
			k8s_ingress_path: {
				each: "spec.rules[*].http.paths[*]"
				args: {
					host:    "host"
					path:    {path: "path", default: ""}
					service: {path: "backend.service.name", default: ""}
					port:    {path: "port", default: ""}
				}
			}
		}
	}
	apiVersion: "networking.k8s.io/v1" @pudl(match)
	kind:       "Ingress"              @pudl(match)
	cluster:    string & !=""
	metadata:   {name: string, namespace: string, uid: string, ...}
	spec?:      {...}
	status?:    {...}
}
