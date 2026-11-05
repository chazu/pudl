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
		graph: {name: "User.UserName", account: "AccountId"}
		graph: display: {code: "USER", label: "IAM user", class: "identity"}
		graph: security: control: true
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
		graph: {name: "RoleName", account: "AccountId"}
		graph: display: {code: "ROLE", label: "IAM role", class: "identity"}
		graph: security: control: true
		// Trust policy principals. IAM writes a lone statement, and a lone
		// principal, without the enclosing list: one_or_many reads both
		// shapes. aws_role_trust is one fact per AWS principal (an ARN, an
		// account, or "*"); aws_role_trust_any records statements whose whole
		// Principal is "*". Service principals are not projected.
		facts: {
			aws_role_trust: {
				each:        "AssumeRolePolicyDocument.Statement[*]"
				one_or_many: true
				args: {effect: "Effect", principal: "Principal.AWS[*]", conditional: {exists: "Condition"}}
			}
			aws_role_trust_any: {
				each:        "AssumeRolePolicyDocument.Statement[*]"
				one_or_many: true
				args: {effect: "Effect", principal: "Principal", conditional: {exists: "Condition"}}
			}
		}
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

// `aws iam list-instance-profiles`: what EC2 instances launch with. Each
// profile carries (at most) one role; an instance names the profile, not the
// role, so the graph goes instance -> profile -> role. Identity is the ARN.
#IAMInstanceProfile: {
	_pudl: {
		schema_type:   "base"
		resource_type: "aws.iam.instance_profile"
		identity_fields: ["Arn"]
		review: status: "draft"
		owner: _#AWSOwner
		graph: {name: "InstanceProfileName", account: "AccountId"}
		graph: display: {code: "PROF", label: "Instance profile", class: "identity"}
		facts: aws_instance_profile_role: args: role_arn: "Roles[*].Arn"
	}
	AccountId:           string
	InstanceProfileName: string
	InstanceProfileId:   string
	Arn:                 =~":instance-profile/"
	Roles: [...{Arn: string, RoleName?: string, ...}]
	...
}
