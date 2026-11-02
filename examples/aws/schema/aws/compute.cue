package aws

import awsboot "pudl.schemas/pudl/aws@v0"

// `aws ec2 describe-instances --query 'Reservations[].Instances[]' --output
// json`: the query flattens reservations into one list of instances.
// Identity is InstanceId (describe-instances prints no ARN).
//
// This is not a specialization of the built-in pudl/aws.#Instance, which is a
// normalized snake_case shape (instance_id) rather than CLI output; the two
// cannot share identity fields.
//
// describe-instances does not print user data, but records enriched with
// `aws ec2 describe-instance-attribute --attribute userData` would carry
// bootstrap scripts and secrets, so UserData is redacted before storage.
#EC2Instance: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.ec2.instance"
		identity_fields: ["InstanceId"]
		review: status: "draft"
		owner: _#AWSOwner
		sensitive_fields: ["UserData"]
		facts: {
			aws_instance: args: {
				instance_id: "InstanceId"
				// Absent for terminated instances: the arg is then omitted.
				subnet_id: "SubnetId"
				vpc_id:    "VpcId"
				state:     "State.Name"
				public_ip: {exists: "PublicIpAddress"}
			}
			aws_instance_sg: {each: "SecurityGroups[*]", args: {group_id: "GroupId"}}
		}
	}
	AccountId:    string
	Region:       string
	InstanceId:   =~"^i-"
	InstanceType: string
	State: {Name: string, ...}
	SubnetId?:         =~"^subnet-"
	VpcId?:            =~"^vpc-"
	PrivateIpAddress?: string
	PublicIpAddress?:  string
	SecurityGroups?: [...{GroupId: =~"^sg-", GroupName?: string, ...}]
	MetadataOptions?: {HttpTokens?: string, HttpEndpoint?: string, ...}
	BlockDeviceMappings?: [...{DeviceName: string, Ebs?: {VolumeId: string, ...}, ...}]
	Tags?: [...awsboot.#Tag]
	...
}

// `aws ec2 describe-volumes --query Volumes --output json`. Identity is
// VolumeId. Attachments name the instances using the volume.
#EBSVolume: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.ec2.volume"
		identity_fields: ["VolumeId"]
		review: status: "draft"
		owner: _#AWSOwner
		facts: {
			aws_volume: args: {
				volume_id: "VolumeId"
				encrypted: "Encrypted"
				kms_key:   "KmsKeyId"
				state:     "State"
			}
			aws_volume_attachment: {each: "Attachments[*]", args: {instance_id: "InstanceId"}}
		}
	}
	AccountId:        string
	Region:           string
	VolumeId:         =~"^vol-"
	Encrypted:        bool
	KmsKeyId?:        =~"^arn:aws"
	State:            string
	Size:             int
	VolumeType:       string
	AvailabilityZone: string
	Attachments?: [...{InstanceId: =~"^i-", ...}]
	Tags?: [...awsboot.#Tag]
	...
}
