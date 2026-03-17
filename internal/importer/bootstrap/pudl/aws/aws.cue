package aws

// Account represents an AWS account with its canonical 12-digit ID.
// Extends the generic infra.#Account with AWS-specific identity and tracking.
// Data source: aws organizations list-accounts, aws sts get-caller-identity
#Account: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.account"
		identity_fields: ["Id"]
		tracked_fields:  ["Arn", "Name", "Status", "Email"]
	}

	Id:      string // 12-digit AWS account ID
	Arn:     string
	Name:    string
	Status?: "ACTIVE" | "SUSPENDED" | "PENDING_CLOSURE"
	Email?:  string
	...
}

// VPC represents an AWS Virtual Private Cloud.
// Data source: aws ec2 describe-vpcs
#VPC: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.vpc"
		identity_fields: ["VpcId"]
		tracked_fields:  ["CidrBlock", "State", "IsDefault", "EnableDnsSupport", "EnableDnsHostnames"]
	}

	VpcId:               string
	CidrBlock:           string
	State:               "available" | "pending"
	IsDefault:           bool
	OwnerId?:            string
	InstanceTenancy?:    "default" | "dedicated" | "host"
	EnableDnsSupport?:   bool
	EnableDnsHostnames?: bool
	Tags?: [...#Tag]
	...
}

// Subnet represents a subdivision of a VPC tied to an availability zone.
// Data source: aws ec2 describe-subnets
#Subnet: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.subnet"
		identity_fields: ["SubnetId"]
		tracked_fields:  ["VpcId", "CidrBlock", "AvailabilityZone", "MapPublicIpOnLaunch", "State"]
	}

	SubnetId:                string
	VpcId:                   string
	CidrBlock:               string
	AvailabilityZone:        string
	AvailabilityZoneId?:     string
	State:                   "available" | "pending"
	MapPublicIpOnLaunch:     bool
	AvailableIpAddressCount?: int
	DefaultForAz?:           bool
	Tags?: [...#Tag]
	...
}

// RouteTable contains routing rules for a VPC.
// Data source: aws ec2 describe-route-tables
#RouteTable: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.route_table"
		identity_fields: ["RouteTableId"]
		tracked_fields:  ["VpcId", "Routes", "Associations"]
	}

	RouteTableId: string
	VpcId:        string
	Routes: [...#Route]
	Associations: [...#RouteTableAssociation]
	Tags?: [...#Tag]
	...
}

#Route: {
	DestinationCidrBlock?:     string
	DestinationIpv6CidrBlock?: string
	DestinationPrefixListId?:  string
	GatewayId?:                string
	NatGatewayId?:             string
	InstanceId?:               string
	VpcPeeringConnectionId?:   string
	TransitGatewayId?:         string
	VpcEndpointId?:            string
	State:                     "active" | "blackhole"
	Origin:                    "CreateRouteTable" | "CreateRoute" | "EnableVgwRoutePropagation"
	...
}

#RouteTableAssociation: {
	RouteTableAssociationId: string
	RouteTableId:            string
	SubnetId?:               string
	GatewayId?:              string
	Main:                    bool
	AssociationState: {
		State: string
		...
	}
	...
}

// SecurityGroup represents a stateful firewall for EC2 resources.
// Data source: aws ec2 describe-security-groups
#SecurityGroup: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.security_group"
		identity_fields: ["GroupId"]
		tracked_fields:  ["VpcId", "IpPermissions", "IpPermissionsEgress"]
	}

	GroupId:              string
	GroupName:            string
	VpcId:               string
	Description?:        string
	OwnerId?:            string
	IpPermissions:       [...#IpPermission]
	IpPermissionsEgress: [...#IpPermission]
	Tags?: [...#Tag]
	...
}

#IpPermission: {
	IpProtocol: string          // "tcp", "udp", "icmp", "-1" (all)
	FromPort?:  int
	ToPort?:    int
	IpRanges?: [...{
		CidrIp:       string
		Description?: string
		...
	}]
	Ipv6Ranges?: [...{
		CidrIpv6:     string
		Description?: string
		...
	}]
	UserIdGroupPairs?: [...{
		GroupId:     string
		UserId?:    string
		Description?: string
		...
	}]
	PrefixListIds?: [...{
		PrefixListId: string
		Description?: string
		...
	}]
	...
}

// InternetGateway provides internet access for a VPC.
// Data source: aws ec2 describe-internet-gateways
#InternetGateway: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.internet_gateway"
		identity_fields: ["InternetGatewayId"]
		tracked_fields:  ["Attachments"]
	}

	InternetGatewayId: string
	OwnerId?:          string
	Attachments: [...{
		VpcId: string
		State: "available" | "attaching" | "attached" | "detaching" | "detached"
		...
	}]
	Tags?: [...#Tag]
	...
}

// NATGateway provides outbound internet for private subnets.
// Data source: aws ec2 describe-nat-gateways
#NATGateway: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.nat_gateway"
		identity_fields: ["NatGatewayId"]
		tracked_fields:  ["SubnetId", "VpcId", "State", "ConnectivityType"]
	}

	NatGatewayId:     string
	SubnetId:         string
	VpcId:            string
	State:            "pending" | "failed" | "available" | "deleting" | "deleted"
	ConnectivityType: "public" | "private"
	NatGatewayAddresses?: [...{
		AllocationId?:       string
		NetworkInterfaceId?: string
		PrivateIp?:          string
		PublicIp?:           string
		...
	}]
	Tags?: [...#Tag]
	...
}

// NetworkACL represents a stateless subnet-level firewall.
// Data source: aws ec2 describe-network-acls
#NetworkACL: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.network_acl"
		identity_fields: ["NetworkAclId"]
		tracked_fields:  ["VpcId", "Entries", "IsDefault"]
	}

	NetworkAclId: string
	VpcId:        string
	IsDefault:    bool
	Entries: [...#NetworkACLEntry]
	Associations: [...{
		NetworkAclAssociationId: string
		NetworkAclId:            string
		SubnetId:                string
		...
	}]
	Tags?: [...#Tag]
	...
}

#NetworkACLEntry: {
	RuleNumber: int
	Protocol:   string // "-1" (all), "6" (tcp), "17" (udp)
	RuleAction: "allow" | "deny"
	Egress:     bool
	CidrBlock?: string
	Ipv6CidrBlock?: string
	PortRange?: {
		From: int
		To:   int
	}
	...
}

// VPCPeering represents a connection between two VPCs.
// Data source: aws ec2 describe-vpc-peering-connections
#VPCPeering: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.vpc_peering"
		identity_fields: ["VpcPeeringConnectionId"]
		tracked_fields:  ["RequesterVpcInfo", "AccepterVpcInfo", "Status"]
	}

	VpcPeeringConnectionId: string
	RequesterVpcInfo: {
		OwnerId: string
		VpcId:   string
		CidrBlock?: string
		Region?:    string
		...
	}
	AccepterVpcInfo: {
		OwnerId: string
		VpcId:   string
		CidrBlock?: string
		Region?:    string
		...
	}
	Status: {
		Code:     "initiating-request" | "pending-acceptance" | "active" | "deleted" | "rejected" | "failed" | "expired" | "provisioning" | "deleting"
		Message?: string
		...
	}
	Tags?: [...#Tag]
	...
}

// VPCEndpoint represents private connectivity to an AWS service.
// Data source: aws ec2 describe-vpc-endpoints
#VPCEndpoint: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.vpc_endpoint"
		identity_fields: ["VpcEndpointId"]
		tracked_fields:  ["ServiceName", "VpcId", "VpcEndpointType", "State"]
	}

	VpcEndpointId:   string
	ServiceName:     string
	VpcId:           string
	VpcEndpointType: "Interface" | "Gateway" | "GatewayLoadBalancer"
	State:           "PendingAcceptance" | "Pending" | "Available" | "Deleting" | "Deleted" | "Rejected" | "Failed" | "Expired"
	PolicyDocument?: string
	RouteTableIds?: [...string]
	SubnetIds?: [...string]
	Groups?: [...{
		GroupId:   string
		GroupName: string
		...
	}]
	NetworkInterfaceIds?: [...string]
	PrivateDnsEnabled?:   bool
	Tags?: [...#Tag]
	...
}

// ElasticIP represents a static public IPv4 address.
// Data source: aws ec2 describe-addresses
#ElasticIP: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "aws.ec2.elastic_ip"
		identity_fields: ["AllocationId"]
		tracked_fields:  ["PublicIp", "AssociationId", "Domain", "InstanceId", "NetworkInterfaceId"]
	}

	AllocationId:        string
	PublicIp:            string
	Domain:              "vpc" | "standard"
	AssociationId?:      string
	InstanceId?:         string
	NetworkInterfaceId?: string
	PrivateIpAddress?:   string
	Tags?: [...#Tag]
	...
}

// Tag is the standard AWS key-value tag.
#Tag: {
	Key:   string
	Value: string
	...
}
