package aws

import awsboot "pudl.schemas/pudl/aws@v0"

// IAM read-only posture. IAM is global, so these records carry AccountId but
// no Region. The collectors read no secrets: login profiles say only that a
// console password exists, and access keys are not collected.

// One record per user, assembled by the aws-iam-users collector from `aws iam
// list-users` and, per user, get-user (User), get-login-profile
// (LoginProfile, left out when the user has no console password: NoSuchEntity)
// and list-mfa-devices (MFADevices). Identity is the user ARN.
#IAMUser: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.iam.user"
		identity_fields: ["User.Arn"]
		review: status: "draft"
		owner: _#AWSOwner
	}
	AccountId: string
	User: {
		UserName:          string
		UserId:            string
		Arn:               =~"^arn:aws"
		CreateDate:        string
		PasswordLastUsed?: string
		Tags?: [...awsboot.#Tag]
		...
	}
	LoginProfile?: {CreateDate?: string, PasswordResetRequired?: bool, ...}
	MFADevices: [...{SerialNumber: string, ...}]
	...
}

// `aws iam list-roles --query Roles --output json`. The CLI prints
// AssumeRolePolicyDocument (the trust policy) as a JSON object. Identity is
// the role ARN.
#IAMRole: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.iam.role"
		identity_fields: ["Arn"]
		review: status: "draft"
		owner: _#AWSOwner
	}
	AccountId:  string
	RoleName:   string
	RoleId:     string
	Arn:        =~"^arn:aws"
	Path:       string
	CreateDate: string
	AssumeRolePolicyDocument: {Statement: _, ...}
	MaxSessionDuration?: int
	...
}
