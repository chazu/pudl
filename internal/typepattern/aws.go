// Package typepattern provides type detection patterns for common data formats.
package typepattern

import "strings"

// awsTrackedFields maps AWS resource types to fields that should be tracked for change detection.
var awsTrackedFields = map[string][]string{
	"ec2:Instance":    {"State.Name", "InstanceType", "PrivateIpAddress", "PublicIpAddress"},
	"s3:Bucket":       {"CreationDate"},
	"s3:Object":       {"Size", "LastModified", "ETag"},
	"lambda:Function": {"Runtime", "CodeSize", "LastModified", "State"},
	"iam:Role":        {"AssumeRolePolicyDocument", "MaxSessionDuration"},
}

// extractCloudFormationTypeID extracts a type identifier from CloudFormation resource data.
// Returns the Type field value, e.g., "AWS::EC2::Instance".
func extractCloudFormationTypeID(data map[string]interface{}) string {
	if typeVal, ok := data["Type"].(string); ok {
		return typeVal
	}
	return ""
}

// mapCloudFormationImport maps a CloudFormation type ID to its CUE import path.
// Currently returns empty string since AWS doesn't have official CUE schemas in cue.dev registry.
func mapCloudFormationImport(typeID string) string {
	// AWS does not have official CUE schemas in cue.dev registry
	// Return empty string to signal that a standalone schema should be generated
	return ""
}

// cloudFormationMetadataDefaults returns default PUDL metadata for a CloudFormation type.
func cloudFormationMetadataDefaults(typeID string) *PudlMetadata {
	// Parse typeID like "AWS::EC2::Instance" -> "aws.ec2.instance"
	resourceType := parseCloudFormationResourceType(typeID)

	return &PudlMetadata{
		SchemaType:      "aws",
		ResourceType:    resourceType,
		IdentityFields:  []string{"LogicalResourceId"},
		TrackedFields:   []string{},
	}
}

// parseCloudFormationResourceType converts a CloudFormation type to PUDL resource type.
// "AWS::EC2::Instance" -> "aws.ec2.instance"
func parseCloudFormationResourceType(typeID string) string {
	// Remove "AWS::" prefix and convert to lowercase with dots
	typeID = strings.TrimPrefix(typeID, "AWS::")
	parts := strings.Split(typeID, "::")
	result := "aws"
	for _, part := range parts {
		result += "." + strings.ToLower(part)
	}
	return result
}

// extractEC2TypeID extracts a type identifier for EC2 resources.
func extractEC2TypeID(data map[string]interface{}) string {
	return "ec2:Instance"
}

// ec2MetadataDefaults returns default PUDL metadata for EC2 instances.
func ec2MetadataDefaults(typeID string) *PudlMetadata {
	return &PudlMetadata{
		SchemaType:      "aws",
		ResourceType:    "aws.ec2.instance",
		IdentityFields:  []string{"InstanceId"},
		TrackedFields:   awsTrackedFields["ec2:Instance"],
	}
}

// extractS3BucketTypeID extracts a type identifier for S3 buckets.
func extractS3BucketTypeID(data map[string]interface{}) string {
	return "s3:Bucket"
}

// s3BucketMetadataDefaults returns default PUDL metadata for S3 buckets.
func s3BucketMetadataDefaults(typeID string) *PudlMetadata {
	return &PudlMetadata{
		SchemaType:      "aws",
		ResourceType:    "aws.s3.bucket",
		IdentityFields:  []string{"Name"},
		TrackedFields:   awsTrackedFields["s3:Bucket"],
	}
}

// extractS3ObjectTypeID extracts a type identifier for S3 objects.
func extractS3ObjectTypeID(data map[string]interface{}) string {
	return "s3:Object"
}

// s3ObjectMetadataDefaults returns default PUDL metadata for S3 objects.
func s3ObjectMetadataDefaults(typeID string) *PudlMetadata {
	return &PudlMetadata{
		SchemaType:      "aws",
		ResourceType:    "aws.s3.object",
		IdentityFields:  []string{"Key"},
		TrackedFields:   awsTrackedFields["s3:Object"],
	}
}

// extractLambdaTypeID extracts a type identifier for Lambda functions.
func extractLambdaTypeID(data map[string]interface{}) string {
	return "lambda:Function"
}

// lambdaMetadataDefaults returns default PUDL metadata for Lambda functions.
func lambdaMetadataDefaults(typeID string) *PudlMetadata {
	return &PudlMetadata{
		SchemaType:      "aws",
		ResourceType:    "aws.lambda.function",
		IdentityFields:  []string{"FunctionArn"},
		TrackedFields:   awsTrackedFields["lambda:Function"],
	}
}

// extractIAMRoleTypeID extracts a type identifier for IAM roles.
func extractIAMRoleTypeID(data map[string]interface{}) string {
	return "iam:Role"
}

// iamRoleMetadataDefaults returns default PUDL metadata for IAM roles.
func iamRoleMetadataDefaults(typeID string) *PudlMetadata {
	return &PudlMetadata{
		SchemaType:      "aws",
		ResourceType:    "aws.iam.role",
		IdentityFields:  []string{"Arn"},
		TrackedFields:   awsTrackedFields["iam:Role"],
	}
}

// awsImportMapper returns empty string for all AWS types since there are no official CUE schemas.
func awsImportMapper(typeID string) string {
	return ""
}

// RegisterAWSPatterns registers AWS type detection patterns with the registry.
func RegisterAWSPatterns(r *Registry) {
	// CloudFormation resource pattern
	r.Register(&TypePattern{
		Name:           "aws-cloudformation",
		Ecosystem:      "aws",
		RequiredFields: []string{"Type", "Properties"},
		OptionalFields: []string{"Metadata", "DependsOn", "Condition", "DeletionPolicy"},
		FieldValues: map[string][]string{
			"Type": {
				"AWS::EC2::Instance", "AWS::EC2::VPC", "AWS::EC2::Subnet",
				"AWS::S3::Bucket", "AWS::Lambda::Function", "AWS::IAM::Role",
				"AWS::IAM::Policy", "AWS::DynamoDB::Table", "AWS::SNS::Topic",
				"AWS::SQS::Queue", "AWS::ECS::Cluster", "AWS::EKS::Cluster",
			},
		},
		TypeExtractor:    extractCloudFormationTypeID,
		ImportMapper:     mapCloudFormationImport,
		MetadataDefaults: cloudFormationMetadataDefaults,
		Priority:         80,
	})

	// EC2 Instance pattern (API response)
	r.Register(&TypePattern{
		Name:           "aws-ec2-instance",
		Ecosystem:      "aws",
		RequiredFields: []string{"InstanceId", "InstanceType", "State"},
		OptionalFields: []string{"ImageId", "PrivateIpAddress", "PublicIpAddress", "VpcId", "SubnetId"},
		TypeExtractor:    extractEC2TypeID,
		ImportMapper:     awsImportMapper,
		MetadataDefaults: ec2MetadataDefaults,
		Priority:         80,
	})

	// S3 Bucket pattern (API response)
	r.Register(&TypePattern{
		Name:           "aws-s3-bucket",
		Ecosystem:      "aws",
		RequiredFields: []string{"Name", "CreationDate"},
		OptionalFields: []string{"Owner"},
		TypeExtractor:    extractS3BucketTypeID,
		ImportMapper:     awsImportMapper,
		MetadataDefaults: s3BucketMetadataDefaults,
		Priority:         80,
	})

	// S3 Object pattern (API response)
	r.Register(&TypePattern{
		Name:           "aws-s3-object",
		Ecosystem:      "aws",
		RequiredFields: []string{"Key", "Size"},
		OptionalFields: []string{"LastModified", "ETag", "StorageClass", "Owner"},
		TypeExtractor:    extractS3ObjectTypeID,
		ImportMapper:     awsImportMapper,
		MetadataDefaults: s3ObjectMetadataDefaults,
		Priority:         80,
	})

	// Lambda Function pattern (API response)
	r.Register(&TypePattern{
		Name:           "aws-lambda-function",
		Ecosystem:      "aws",
		RequiredFields: []string{"FunctionName", "FunctionArn", "Runtime"},
		OptionalFields: []string{"Handler", "CodeSize", "Description", "Timeout", "MemorySize", "LastModified"},
		TypeExtractor:    extractLambdaTypeID,
		ImportMapper:     awsImportMapper,
		MetadataDefaults: lambdaMetadataDefaults,
		Priority:         80,
	})

	// IAM Role pattern (API response)
	r.Register(&TypePattern{
		Name:           "aws-iam-role",
		Ecosystem:      "aws",
		RequiredFields: []string{"RoleName", "Arn", "AssumeRolePolicyDocument"},
		OptionalFields: []string{"RoleId", "Path", "CreateDate", "Description", "MaxSessionDuration"},
		TypeExtractor:    extractIAMRoleTypeID,
		ImportMapper:     awsImportMapper,
		MetadataDefaults: iamRoleMetadataDefaults,
		Priority:         80,
	})
}

