package aws

import awsboot "pudl.schemas/pudl/aws@v0"

// EC2 networking as printed by `aws ec2 describe-* --query <List> --output
// json`. The --query unwraps the top-level list ({"Vpcs": [...]}) so each
// element is one record. AWS prints neither the region nor, for most types,
// the account, so every collector adds AccountId and Region (`set`).
//
// VPC, subnet and security group specialize the built-in pudl/aws types
// (base_schema) and keep their identity fields (VpcId, SubnetId, GroupId).
// A record imported against either schema therefore has one resource
// identity, and `pudl import` of plain describe-* output (without AccountId
// and Region) still falls back to the built-in type. AWS resource IDs are
// unique across accounts and regions.
//
// Every type in this pack is a draft: compare it with your own CLI output,
// then set review: {status: "validated", by: "...", at: "YYYY-MM-DD"}. Until
// then policy results against it are marked provisional.

#VPC: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.ec2.vpc"
		base_schema:   "pudl/aws.#VPC"
		identity_fields: ["VpcId"]
		review: status: "draft"
		owner: _#AWSOwner
		facts: aws_vpc: args: {
			vpc_id:     "VpcId"
			account:    "AccountId"
			region:     "Region"
			cidr:       "CidrBlock"
			is_default: "IsDefault"
		}
	}
	AccountId: string
	Region:    string
	VpcId:     =~"^vpc-"
	OwnerId:   string
	CidrBlock: string
	State:     string
	IsDefault: bool
	Tags?: [...awsboot.#Tag]
	...
}

#Subnet: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.ec2.subnet"
		base_schema:   "pudl/aws.#Subnet"
		identity_fields: ["SubnetId"]
		review: status: "draft"
		owner: _#AWSOwner
		facts: aws_subnet: args: {
			subnet_id:         "SubnetId"
			vpc_id:            "VpcId"
			availability_zone: "AvailabilityZone"
			public_ip_on_launch: {path: "MapPublicIpOnLaunch", default: false}
		}
	}
	AccountId:           string
	Region:              string
	SubnetId:            =~"^subnet-"
	SubnetArn?:          =~"^arn:aws"
	VpcId:               =~"^vpc-"
	CidrBlock:           string
	AvailabilityZone:    string
	MapPublicIpOnLaunch: bool
	DefaultForAz?:       bool
	Tags?: [...awsboot.#Tag]
	...
}

// Rules are in IpPermissions (ingress) and IpPermissionsEgress.
#SecurityGroup: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.ec2.security_group"
		base_schema:   "pudl/aws.#SecurityGroup"
		identity_fields: ["GroupId"]
		review: status: "draft"
		owner: _#AWSOwner
		facts: aws_security_group: args: {
			group_id: "GroupId"
			name:     "GroupName"
			vpc_id:   "VpcId"
		}
	}
	AccountId:         string
	Region:            string
	GroupId:           =~"^sg-"
	GroupName:         string
	SecurityGroupArn?: =~"^arn:aws"
	VpcId:             =~"^vpc-"
	OwnerId:           string
	IpPermissions: [...awsboot.#IpPermission]
	IpPermissionsEgress: [...awsboot.#IpPermission]
	Tags?: [...awsboot.#Tag]
	...
}
