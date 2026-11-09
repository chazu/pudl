package gcp

// Build and deploy: Cloud Build triggers, App Engine applications, services
// and versions, Cloud Functions and Cloud Run domain mappings: the API
// objects CAI exports and gcloud prints (schema/models/gcp_deploy.cue).
//
// Environment variables and build substitutions can hold credentials, so
// their maps are redacted whole; the shapes add the names they hold
// (envVariableNames and the like), sorted, which stay.

// `gcloud builds triggers list --region=L --format=json`. Both sources omit
// resourceName on some triggers, so identity is the project and the
// trigger's UUID; the collector adds the location (CAI's asset name holds it;
// a gcloud run sets the one it listed).
#BuildTrigger: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.cloudbuild.trigger"
		identity_fields: ["project", "id"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {region: "location"}
		graph: display: {code: "CBT", label: "Cloud Build trigger", class: "admin"}
		// Substitution values (webhook URLs, tokens) and an inline build's
		// step arguments, environment, scripts and encrypted secrets.
		sensitive_fields: [
			"substitutions",
			"build.substitutions",
			"build.steps[*].args[*]",
			"build.steps[*].env[*]",
			"build.steps[*].script",
			"build.options.env[*]",
			"build.secrets[*].secretEnv",
			"build.availableSecrets.inline[*].envMap",
		]
		facts: gcp_build_trigger: args: {
			project:  "project"
			id:       "id"
			name:     "name"
			disabled: {path: "disabled", default: false}
			// The e-mail of projects/P/serviceAccounts/<email>; "" when unset
			// (the trigger runs as the legacy Cloud Build service account).
			service_account: {path: "serviceAccount", default: "", rewrite: [
				{match: "^projects/[^/]+/serviceAccounts/(.+)$", replace: "$1"},
				{match: "^$", replace: ""},
			]}
		}
	}
	project:  _gcpProjectID
	id:       =~"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$" @pudl(match)
	location: string                                                             @pudl(match)
	name:     string
	...
}

// `gcloud app describe --format=json`: the project's application, named
// apps/P.
#AppEngineApplication: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.appengine.application"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "name", last_segment: true}, region: "locationId"}
		graph: display: {code: "GAE", label: "App Engine application", class: "compute"}
		sensitive_fields: ["iap.oauth2ClientSecret"]
		facts: gcp_appengine_app: args: {
			project:        "project"
			name:           "name"
			location:       {path: "locationId", default: ""}
			serving_status: {path: "servingStatus", default: ""}
		}
	}
	project: _gcpProjectID
	name:    =~"^apps/[^/]+$" @pudl(match)
	...
}

// `appengine.googleapis.com/Service`, named apps/P/services/S (CAI only;
// `gcloud app services describe S` prints the same object).
#AppEngineService: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.appengine.service"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "GAES", label: "App Engine service", class: "compute"}
		facts: gcp_appengine_service: args: {
			project: "project"
			name:    "name"
			ingress: {path: "networkSettings.ingressTrafficAllowed", default: ""}
		}
	}
	project: _gcpProjectID
	name:    =~"^apps/[^/]+/services/[^/]+$" @pudl(match)
	...
}

// The version field of `gcloud app versions list --format=json` (gcloud's
// wrapper is dropped), named apps/P/services/S/versions/V.
#AppEngineVersion: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.appengine.version"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner
		graph: _gcpGraph & {name: {path: "name", last_segment: true}}
		graph: display: {code: "GAEV", label: "App Engine version", class: "compute"}
		graph: inventory: collapsed: true
		sensitive_fields: ["envVariables", "buildEnvVariables"]
		facts: gcp_appengine_version: args: {
			project:        "project"
			name:           "name"
			service:        {path: "name", rewrite: [{match: "^(.*)/versions/[^/]+$", replace: "$1"}]}
			runtime:        {path: "runtime", default: ""}
			env:            {path: "env", default: "standard"}
			serving_status: {path: "servingStatus", default: ""}
		}
	}
	project: _gcpProjectID
	name:    =~"^apps/[^/]+/services/[^/]+/versions/[^/]+$" @pudl(match)
	...
}

// `gcloud functions list --format=json`, the v2 API, which lists 1st and 2nd
// gen functions (environment says which). The shape adds the location the
// name holds.
#CloudFunction: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.cloudfunctions.function"
		identity_fields: ["name"]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["labels.team"]}
		graph: _gcpGraph & {name: {path: "name", last_segment: true}, region: "location"}
		graph: display: {code: "FN", label: "Cloud Function", class: "compute"}
		// The upload URL is signed. Secret references
		// (secretEnvironmentVariables, secretVolumes) are kept.
		sensitive_fields: [
			"serviceConfig.environmentVariables",
			"buildConfig.environmentVariables",
			"buildConfig.source.storageSource.sourceUploadUrl",
		]
		facts: gcp_function: args: {
			project:         "project"
			name:            "name"
			environment:     {path: "environment", default: ""}
			state:           {path: "state", default: ""}
			runtime:         {path: "buildConfig.runtime", default: ""}
			ingress:         {path: "serviceConfig.ingressSettings", default: ""}
			service_account: {path: "serviceConfig.serviceAccountEmail", default: ""}
		}
		// The Cloud Run service a 2nd gen function runs as
		// (projects/P/locations/L/services/S).
		facts: gcp_function_run_service: args: service: {
			path:    "serviceConfig.service"
			rewrite: [{match: "^(projects/[^/]+/locations/[^/]+/services/[^/]+)$", replace: "$1"}]
		}
	}
	project:   _gcpProjectID
	name:      =~"^projects/[^/]+/locations/[^/]+/functions/[^/]+$" @pudl(match)
	location?: string
	...
}

// `gcloud beta run domain-mappings list --region=R --format=json`, a Knative
// object like #RunService:
// identity is the project number (metadata.namespace), the location label and
// the domain (metadata.name).
#RunDomainMapping: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.run.domain_mapping"
		identity_fields: [
			"metadata.namespace",
			"metadata.labels.\"cloud.googleapis.com/location\"",
			"metadata.name",
		]
		review: status: "draft"
		owner: _#GCPOwner & {paths: ["metadata.labels.team"]}
		graph: _gcpGraph & {name: "metadata.name", region: "metadata.labels.\"cloud.googleapis.com/location\""}
		graph: display: {code: "RUNDM", label: "Cloud Run domain mapping", class: "edge"}
		sensitive_fields: [
			"metadata.annotations.\"serving.knative.dev/creator\"",
			"metadata.annotations.\"serving.knative.dev/lastModifier\"",
		]
		facts: gcp_run_domain_mapping: args: {
			project:  "project"
			domain:   "metadata.name"
			location: "metadata.labels.\"cloud.googleapis.com/location\""
			route:    {path: "spec.routeName", default: ""}
		}
	}
	apiVersion: "domains.cloudrun.com/v1" @pudl(match)
	kind:       "DomainMapping"           @pudl(match)
	project:    _gcpProjectID
	metadata: {
		name:      string
		namespace: string
		labels: {
			"cloud.googleapis.com/location": string
			...
		}
		...
	}
	...
}
