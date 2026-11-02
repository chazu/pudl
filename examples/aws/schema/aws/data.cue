package aws

import awsboot "pudl.schemas/pudl/aws@v0"

// Data stores and keys: S3, RDS and KMS.

// One record per bucket, assembled by the aws-s3-buckets collector from
// `aws s3api list-buckets` and, per bucket, get-public-access-block,
// get-bucket-encryption and get-bucket-versioning. A setting that is not
// configured is left out of the record (AWS answers with an error such as
// NoSuchPublicAccessBlockConfiguration). Bucket names are globally unique.
#S3Bucket: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.s3.bucket"
		identity_fields: ["Name"]
		review: status: "draft"
		owner: _#AWSOwner
		graph: {name: "Name", account: "AccountId"}
	}
	AccountId:     string
	Name:          string
	CreationDate?: string
	PublicAccessBlockConfiguration?: {
		BlockPublicAcls?:       bool
		IgnorePublicAcls?:      bool
		BlockPublicPolicy?:     bool
		RestrictPublicBuckets?: bool
		...
	}
	ServerSideEncryptionConfiguration?: {...}
	Versioning?: {Status?: "Enabled" | "Suspended", ...}
	...
}

// `aws rds describe-db-instances --query DBInstances --output json`.
// Identity is the DBInstanceArn. DBSubnetGroup names the VPC and subnets.
#RDSInstance: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.rds.db_instance"
		identity_fields: ["DBInstanceArn"]
		review: status: "draft"
		owner: _#AWSOwner
		graph: _awsGraph & {name: "DBInstanceIdentifier", refs: ["DBInstanceIdentifier"]}
		facts: {
			aws_db_instance: args: {
				arn:          "DBInstanceArn"
				id:           "DBInstanceIdentifier"
				subnet_group: "DBSubnetGroup.DBSubnetGroupName"
				vpc_id:       "DBSubnetGroup.VpcId"
				encrypted:    "StorageEncrypted"
				// A KMS key ARN; absent when storage is unencrypted.
				kms_key: "KmsKeyId"
			}
			aws_db_subnet: {each: "DBSubnetGroup.Subnets[*]", args: {subnet_id: "SubnetIdentifier"}}
			aws_db_sg: {each: "VpcSecurityGroups[*]", args: {group_id: "VpcSecurityGroupId"}}
		}
	}
	AccountId:             string
	Region:                string
	DBInstanceIdentifier:  string
	DBInstanceArn:         =~"^arn:aws"
	Engine:                string
	DBInstanceStatus:      string
	StorageEncrypted:      bool
	PubliclyAccessible:    bool
	BackupRetentionPeriod: int
	KmsKeyId?:             =~"^arn:aws"
	DBSubnetGroup?: {
		DBSubnetGroupName: string
		VpcId:             =~"^vpc-"
		Subnets?: [...{SubnetIdentifier: =~"^subnet-", ...}]
		...
	}
	VpcSecurityGroups?: [...{VpcSecurityGroupId: =~"^sg-", ...}]
	TagList?: [...awsboot.#Tag]
	...
}

// One record per key, assembled by the aws-kms-keys collector from `aws kms
// list-keys` and, per key, describe-key (KeyMetadata) and
// get-key-rotation-status (KeyRotationEnabled). KeyRotationEnabled is left out
// where AWS cannot report rotation: asymmetric and HMAC keys
// (UnsupportedOperationException) and keys pending deletion or import
// (KMSInvalidStateException). Identity is the key ARN, the form RDS and EBS
// use in KmsKeyId. list-keys includes AWS managed keys (KeyManager "AWS").
#KMSKey: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.kms.key"
		identity_fields: ["KeyMetadata.Arn"]
		review: status: "draft"
		owner: _#AWSOwner
		graph: _awsGraph & {name: "KeyMetadata.KeyId", refs: ["KeyMetadata.KeyId"]}
		facts: aws_kms_key: args: {
			arn:     "KeyMetadata.Arn"
			key_id:  "KeyMetadata.KeyId"
			state:   "KeyMetadata.KeyState"
			manager: "KeyMetadata.KeyManager"
		}
	}
	AccountId: string
	Region:    string
	KeyMetadata: {
		KeyId:      string
		Arn:        =~"^arn:aws"
		KeyState:   string
		KeyManager: "AWS" | "CUSTOMER"
		KeySpec?:   string
		KeyUsage?:  string
		Enabled?:   bool
		...
	}
	KeyRotationEnabled?: bool
	...
}
