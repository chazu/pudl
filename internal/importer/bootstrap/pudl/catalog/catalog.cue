package catalog

// CatalogEntry describes a registered schema type in the pudl catalog.
// Users can extend the catalog by adding their own entries alongside
// the built-in ones.
#CatalogEntry: {
	schema:        string // canonical schema name e.g. "pudl/core.#Item"
	schema_type:   string // "catchall", "base", "collection", "policy", "custom"
	resource_type: string // e.g. "unknown", "generic.collection"
	description:   string
}

entries: [string]: #CatalogEntry

// Core types
entries: {
	"pudl/core.#Item": {
		schema:        "pudl/core.#Item"
		schema_type:   "catchall"
		resource_type: "unknown"
		description:   "Universal fallback schema for any data"
	}
	"pudl/core.#Collection": {
		schema:        "pudl/core.#Collection"
		schema_type:   "collection"
		resource_type: "generic.collection"
		description:   "Collection of related data items"
	}
}

// Filesystem types
entries: {
	"pudl/fs.#File": {
		schema:        "pudl/fs.#File"
		schema_type:   "base"
		resource_type: "fs.file"
		description:   "Filesystem entry with type and permissions"
	}
	"pudl/fs.#Dir": {
		schema:        "pudl/fs.#Dir"
		schema_type:   "base"
		resource_type: "fs.dir"
		description:   "Directory with typed file and subdirectory maps"
	}
	"pudl/fs.#Layout": {
		schema:        "pudl/fs.#Layout"
		schema_type:   "base"
		resource_type: "fs.layout"
		description:   "Expected directory structure for validation"
	}
}

// Version types
entries: {
	"pudl/version.#Version": {
		schema:        "pudl/version.#Version"
		schema_type:   "base"
		resource_type: "version"
		description:   "Pinned version string with optional constraint"
	}
	"pudl/version.#ToolVersion": {
		schema:        "pudl/version.#ToolVersion"
		schema_type:   "base"
		resource_type: "version.tool"
		description:   "Tool version with download and verification metadata"
	}
	"pudl/version.#TrackedVersion": {
		schema:        "pudl/version.#TrackedVersion"
		schema_type:   "base"
		resource_type: "version.tracked"
		description:   "Version with sync chain for multi-file tracking"
	}
}

// Infrastructure types
entries: {
	"pudl/infra.#Organization": {
		schema:        "pudl/infra.#Organization"
		schema_type:   "base"
		resource_type: "infra.organization"
		description:   "Organizational unit (company, team, AWS org)"
	}
	"pudl/infra.#Account": {
		schema:        "pudl/infra.#Account"
		schema_type:   "base"
		resource_type: "infra.account"
		description:   "Named account within an organization"
	}
	"pudl/infra.#Platform": {
		schema:        "pudl/infra.#Platform"
		schema_type:   "base"
		resource_type: "infra.platform"
		description:   "Execution platform composed of services"
	}
	"pudl/infra.#Environment": {
		schema:        "pudl/infra.#Environment"
		schema_type:   "base"
		resource_type: "infra.environment"
		description:   "Deployment target composed of platforms"
	}
	"pudl/infra.#Service": {
		schema:        "pudl/infra.#Service"
		schema_type:   "base"
		resource_type: "infra.service"
		description:   "Deployable unit with kind discriminator"
	}
}

// Component classification types
entries: {
	"pudl/component.#Component": {
		schema:        "pudl/component.#Component"
		schema_type:   "base"
		resource_type: "component"
		description:   "Classifiable unit in a system hierarchy (contract/instance/package/rule)"
	}
}

// Artifact types
entries: {
	"pudl/artifact.#ImageRef": {
		schema:        "pudl/artifact.#ImageRef"
		schema_type:   "base"
		resource_type: "artifact.image"
		description:   "Container image reference with digest pinning"
	}
	"pudl/artifact.#ArtifactRef": {
		schema:        "pudl/artifact.#ArtifactRef"
		schema_type:   "base"
		resource_type: "artifact"
		description:   "Generic content-addressed artifact reference"
	}
}

// AWS types
entries: {
	"pudl/aws.#Account": {
		schema:        "pudl/aws.#Account"
		schema_type:   "base"
		resource_type: "aws.account"
		description:   "AWS account with 12-digit ID"
	}
	"pudl/aws.#VPC": {
		schema:        "pudl/aws.#VPC"
		schema_type:   "base"
		resource_type: "aws.ec2.vpc"
		description:   "AWS Virtual Private Cloud"
	}
	"pudl/aws.#Subnet": {
		schema:        "pudl/aws.#Subnet"
		schema_type:   "base"
		resource_type: "aws.ec2.subnet"
		description:   "Subdivision of a VPC tied to an availability zone"
	}
	"pudl/aws.#RouteTable": {
		schema:        "pudl/aws.#RouteTable"
		schema_type:   "base"
		resource_type: "aws.ec2.route_table"
		description:   "VPC routing rules"
	}
	"pudl/aws.#SecurityGroup": {
		schema:        "pudl/aws.#SecurityGroup"
		schema_type:   "base"
		resource_type: "aws.ec2.security_group"
		description:   "Stateful firewall for EC2 resources"
	}
	"pudl/aws.#InternetGateway": {
		schema:        "pudl/aws.#InternetGateway"
		schema_type:   "base"
		resource_type: "aws.ec2.internet_gateway"
		description:   "Internet access gateway for a VPC"
	}
	"pudl/aws.#NATGateway": {
		schema:        "pudl/aws.#NATGateway"
		schema_type:   "base"
		resource_type: "aws.ec2.nat_gateway"
		description:   "Outbound internet for private subnets"
	}
	"pudl/aws.#NetworkACL": {
		schema:        "pudl/aws.#NetworkACL"
		schema_type:   "base"
		resource_type: "aws.ec2.network_acl"
		description:   "Stateless subnet-level firewall"
	}
	"pudl/aws.#VPCPeering": {
		schema:        "pudl/aws.#VPCPeering"
		schema_type:   "base"
		resource_type: "aws.ec2.vpc_peering"
		description:   "Connection between two VPCs"
	}
	"pudl/aws.#VPCEndpoint": {
		schema:        "pudl/aws.#VPCEndpoint"
		schema_type:   "base"
		resource_type: "aws.ec2.vpc_endpoint"
		description:   "Private connectivity to an AWS service"
	}
	"pudl/aws.#ElasticIP": {
		schema:        "pudl/aws.#ElasticIP"
		schema_type:   "base"
		resource_type: "aws.ec2.elastic_ip"
		description:   "Static public IPv4 address"
	}
}

// Registry types
entries: {
	"pudl/registry.#Entry": {
		schema:        "pudl/registry.#Entry"
		schema_type:   "base"
		resource_type: "registry.entry"
		description:   "Base type for any inventory item"
	}
	"pudl/registry.#Domain": {
		schema:        "pudl/registry.#Domain"
		schema_type:   "base"
		resource_type: "registry.domain"
		description:   "Registered domain name"
	}
	"pudl/registry.#Formatter": {
		schema:        "pudl/registry.#Formatter"
		schema_type:   "base"
		resource_type: "registry.formatter"
		description:   "Code formatting tool configuration"
	}
}
