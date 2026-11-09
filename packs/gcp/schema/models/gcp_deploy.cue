package models

import (
	"list"
	"regexp"
	"strings"
)

// Collectors of build and deploy types (schema/gcp/deploy.cue): Cloud Build
// triggers, App Engine applications, services and versions, Cloud Functions
// and Cloud Run domain mappings. Each searches CAI and falls back to gcloud
// per project, except App Engine services (CAI only, below).
//
// Live parity (2026-11-09, values compared): gcloud's copy equals CAI's
// except that gcloud adds a function's satisfiesPzi, prints App Engine
// versions inside its own wrapper (the API object is its version field), and
// lists triggers and domain mappings in one location per command.

_gcpAssetTypes: {
	"build-triggers":      "cloudbuild.googleapis.com/BuildTrigger"
	"appengine-apps":      "appengine.googleapis.com/Application"
	"appengine-services":  "appengine.googleapis.com/Service"
	"appengine-versions":  "appengine.googleapis.com/Version"
	functions:             "cloudfunctions.googleapis.com/Function"
	"run-domain-mappings": "run.googleapis.com/DomainMapping"
}

// `gcloud app services list` prints only each service's ID and versions, not
// its traffic split or ingress setting; `app services describe` prints the
// API object but needs each service's ID, a second level of enumeration a
// command source does not have.
_gcpCAIOnly: "appengine-services": true

_gcpSchemas: {
	"build-triggers":      "gcp.#BuildTrigger"
	"appengine-apps":      "gcp.#AppEngineApplication"
	"appengine-services":  "gcp.#AppEngineService"
	"appengine-versions":  "gcp.#AppEngineVersion"
	functions:             "gcp.#CloudFunction"
	"run-domain-mappings": "gcp.#RunDomainMapping"
}

// gcloud lists the application itself (describe prints one object), every
// version of every service, and functions of both generations in every
// region. A project without an App Engine application answers as one the
// caller may not read ("... or it may not exist"), so it is a failed input,
// never an empty one.
_gcpCommands: {
	"appengine-apps":     ["app", "describe"]
	"appengine-versions": ["app", "versions", "list"]
	functions:            ["functions", "list"]
}

// Triggers and domain mappings are listed one location per command
// (`--region=-` is refused), over the locations in the settings below. CAI
// finds every location, so their gcloud source is partial: it marks nothing
// gone that it did not look for.
_gcpRuns: {
	"build-triggers": [for l in _gcpBuildLocations {
		argv: ["gcloud", "builds", "triggers", "list", "--region=\(l)", "--project={{item}}", "--format=json"]
		set:  {project: "{{item}}", location: l}
	}]
	// The command group is beta for fully managed Cloud Run.
	"run-domain-mappings": [for l in _gcpRunDomainMappingRegions {
		argv: ["gcloud", "beta", "run", "domain-mappings", "list", "--region=\(l)", "--project={{item}}", "--format=json"]
		set: project: "{{item}}"
	}]
}

_gcpPartialGcloud: {
	"build-triggers":      true
	"run-domain-mappings": true
}

// The API object of a gcloud record whose output wraps it.
_gcpGcloudRecord: "appengine-versions": {
	in:  {...}
	out: in.version
}

// The sorted names of a map whose values are redacted when stored.
_gcpNames: {
	in:  {...}
	out: list.SortStrings([for k, _ in in {k}])
}

_gcpShapes: {
	// The location: from the asset name in CAI (CAI omits resourceName on
	// some triggers); a gcloud run sets the location it listed.
	"build-triggers": {
		_asset: *null | {...}
		in:     {...}
		let r = in
		out: {
			r
			if _asset != null {
				location: regexp.FindSubmatch("/locations/([^/]+)/", _asset.name)[1]
			}
			if r.substitutions != _|_ {
				substitutionNames: (_gcpNames & {in: r.substitutions}).out
			}
		}
	}
	"appengine-apps":     _gcpSameShape
	"appengine-services": _gcpSameShape
	"appengine-versions": {
		in: {...}
		let r = in
		out: {
			r
			if r.envVariables != _|_ {
				envVariableNames: (_gcpNames & {in: r.envVariables}).out
			}
			if r.buildEnvVariables != _|_ {
				buildEnvVariableNames: (_gcpNames & {in: r.buildEnvVariables}).out
			}
		}
	}
	// gcloud adds satisfiesPzi.
	functions: {
		in: {...}
		let r = in
		out: {
			(_gcpLocatedShape & {_drop: ["satisfiesPzi"], in: r}).out
			if r.serviceConfig.environmentVariables != _|_ {
				serviceConfig: environmentVariableNames: (_gcpNames & {in: r.serviceConfig.environmentVariables}).out
			}
			if r.buildConfig.environmentVariables != _|_ {
				buildConfig: environmentVariableNames: (_gcpNames & {in: r.buildConfig.environmentVariables}).out
			}
		}
	}
	"run-domain-mappings": _gcpSameShape
}

// CAI exports App Engine applications, services and versions twice: under
// //appengine.googleapis.com/projects/P/locations/L/applications/P/... and
// under the legacy //appengine.googleapis.com/apps/P/.... The legacy copy is
// dropped. gcloud records have no asset and are kept.
_gcpAppEngineLegacy: {
	_asset: *null | {...}
	in:     _
	skip:   [if _asset == null {false}, strings.HasPrefix(_asset.name, "//appengine.googleapis.com/apps/")][0]
}

_gcpSkip: {
	"appengine-apps":     _gcpAppEngineLegacy
	"appengine-services": _gcpAppEngineLegacy
	"appengine-versions": _gcpAppEngineLegacy
}

// The fixture's trigger list is the global location's.
_gcpFixtureGcloudRun: "build-triggers": set: location: "global"

#GCPBuildTriggers:            _#GCPLive & {_type: "build-triggers"}
#GCPAppEngineApps:            _#GCPLive & {_type: "appengine-apps"}
#GCPAppEngineServices:        _#GCPLive & {_type: "appengine-services"}
#GCPAppEngineVersions:        _#GCPLive & {_type: "appengine-versions"}
#GCPFunctions:                _#GCPLive & {_type: "functions"}
#GCPRunDomainMappings:        _#GCPLive & {_type: "run-domain-mappings"}
#GCPFixtureBuildTriggers:     _#GCPFixture & {_type: "build-triggers"}
#GCPFixtureAppEngineApps:     _#GCPFixture & {_type: "appengine-apps"}
#GCPFixtureAppEngineServices: _#GCPFixture & {_type: "appengine-services"}
#GCPFixtureAppEngineVersions: _#GCPFixture & {_type: "appengine-versions"}
#GCPFixtureFunctions:         _#GCPFixture & {_type: "functions"}
#GCPFixtureRunDomainMappings: _#GCPFixture & {_type: "run-domain-mappings"}
