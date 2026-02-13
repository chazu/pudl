package typepattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCloudFormationTypeID(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected string
	}{
		{
			name: "EC2 Instance",
			data: map[string]interface{}{
				"Type":       "AWS::EC2::Instance",
				"Properties": map[string]interface{}{},
			},
			expected: "AWS::EC2::Instance",
		},
		{
			name: "S3 Bucket",
			data: map[string]interface{}{
				"Type":       "AWS::S3::Bucket",
				"Properties": map[string]interface{}{},
			},
			expected: "AWS::S3::Bucket",
		},
		{
			name:     "missing Type",
			data:     map[string]interface{}{"Properties": map[string]interface{}{}},
			expected: "",
		},
		{
			name:     "empty data",
			data:     map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCloudFormationTypeID(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCloudFormationResourceType(t *testing.T) {
	tests := []struct {
		typeID   string
		expected string
	}{
		{"AWS::EC2::Instance", "aws.ec2.instance"},
		{"AWS::S3::Bucket", "aws.s3.bucket"},
		{"AWS::Lambda::Function", "aws.lambda.function"},
		{"AWS::IAM::Role", "aws.iam.role"},
		{"AWS::DynamoDB::Table", "aws.dynamodb.table"},
		{"AWS::ECS::Cluster", "aws.ecs.cluster"},
	}

	for _, tt := range tests {
		t.Run(tt.typeID, func(t *testing.T) {
			result := parseCloudFormationResourceType(tt.typeID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCloudFormationMetadataDefaults(t *testing.T) {
	meta := cloudFormationMetadataDefaults("AWS::EC2::Instance")
	require.NotNil(t, meta)
	assert.Equal(t, "aws", meta.SchemaType)
	assert.Equal(t, "aws.ec2.instance", meta.ResourceType)
	assert.Equal(t, 85, meta.CascadePriority)
	assert.Equal(t, []string{"LogicalResourceId"}, meta.IdentityFields)
}

func TestEC2MetadataDefaults(t *testing.T) {
	meta := ec2MetadataDefaults("ec2:Instance")
	require.NotNil(t, meta)
	assert.Equal(t, "aws", meta.SchemaType)
	assert.Equal(t, "aws.ec2.instance", meta.ResourceType)
	assert.Equal(t, 85, meta.CascadePriority)
	assert.Equal(t, []string{"InstanceId"}, meta.IdentityFields)
	assert.Contains(t, meta.TrackedFields, "State.Name")
}

func TestS3BucketMetadataDefaults(t *testing.T) {
	meta := s3BucketMetadataDefaults("s3:Bucket")
	require.NotNil(t, meta)
	assert.Equal(t, "aws", meta.SchemaType)
	assert.Equal(t, "aws.s3.bucket", meta.ResourceType)
	assert.Equal(t, []string{"Name"}, meta.IdentityFields)
}

func TestS3ObjectMetadataDefaults(t *testing.T) {
	meta := s3ObjectMetadataDefaults("s3:Object")
	require.NotNil(t, meta)
	assert.Equal(t, "aws", meta.SchemaType)
	assert.Equal(t, "aws.s3.object", meta.ResourceType)
	assert.Equal(t, []string{"Key"}, meta.IdentityFields)
}

func TestLambdaMetadataDefaults(t *testing.T) {
	meta := lambdaMetadataDefaults("lambda:Function")
	require.NotNil(t, meta)
	assert.Equal(t, "aws", meta.SchemaType)
	assert.Equal(t, "aws.lambda.function", meta.ResourceType)
	assert.Equal(t, []string{"FunctionArn"}, meta.IdentityFields)
}

func TestIAMRoleMetadataDefaults(t *testing.T) {
	meta := iamRoleMetadataDefaults("iam:Role")
	require.NotNil(t, meta)
	assert.Equal(t, "aws", meta.SchemaType)
	assert.Equal(t, "aws.iam.role", meta.ResourceType)
	assert.Equal(t, []string{"Arn"}, meta.IdentityFields)
}

func TestAWSImportMapper(t *testing.T) {
	// All AWS types should return empty string (no official CUE schemas)
	tests := []string{
		"ec2:Instance",
		"s3:Bucket",
		"lambda:Function",
		"iam:Role",
		"AWS::EC2::Instance",
	}

	for _, typeID := range tests {
		t.Run(typeID, func(t *testing.T) {
			result := awsImportMapper(typeID)
			assert.Equal(t, "", result)
		})
	}
}

func TestMapCloudFormationImport(t *testing.T) {
	// All CloudFormation types should return empty string
	result := mapCloudFormationImport("AWS::EC2::Instance")
	assert.Equal(t, "", result)
}

func TestRegisterAWSPatterns(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	patterns := r.GetPatternsByEcosystem("aws")
	require.Len(t, patterns, 6) // CloudFormation, EC2, S3 Bucket, S3 Object, Lambda, IAM

	// Verify all patterns have Priority 80
	for _, p := range patterns {
		assert.Equal(t, 80, p.Priority)
		assert.Equal(t, "aws", p.Ecosystem)
	}
}

func TestAWSPattern_CloudFormationDetection(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	data := map[string]interface{}{
		"Type": "AWS::EC2::Instance",
		"Properties": map[string]interface{}{
			"ImageId":      "ami-12345",
			"InstanceType": "t2.micro",
		},
		"Metadata": map[string]interface{}{},
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "aws-cloudformation", result.Pattern.Name)
	assert.Equal(t, "AWS::EC2::Instance", result.TypeID)
	assert.Equal(t, "", result.ImportPath) // No CUE schemas for AWS
	assert.Greater(t, result.Confidence, 0.5)
}

func TestAWSPattern_EC2Detection(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	data := map[string]interface{}{
		"InstanceId":   "i-1234567890abcdef0",
		"InstanceType": "t2.micro",
		"State": map[string]interface{}{
			"Name": "running",
			"Code": 16,
		},
		"PrivateIpAddress": "10.0.0.1",
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "aws-ec2-instance", result.Pattern.Name)
	assert.Equal(t, "ec2:Instance", result.TypeID)
	assert.Greater(t, result.Confidence, 0.5)
}

func TestAWSPattern_S3BucketDetection(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	data := map[string]interface{}{
		"Name":         "my-bucket",
		"CreationDate": "2023-01-15T10:30:00Z",
		"Owner": map[string]interface{}{
			"ID":          "owner-id",
			"DisplayName": "owner-name",
		},
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "aws-s3-bucket", result.Pattern.Name)
	assert.Equal(t, "s3:Bucket", result.TypeID)
}

func TestAWSPattern_S3ObjectDetection(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	data := map[string]interface{}{
		"Key":          "path/to/file.txt",
		"Size":         1024,
		"LastModified": "2023-06-01T12:00:00Z",
		"ETag":         "abc123",
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "aws-s3-object", result.Pattern.Name)
	assert.Equal(t, "s3:Object", result.TypeID)
}

func TestAWSPattern_LambdaDetection(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	data := map[string]interface{}{
		"FunctionName": "my-function",
		"FunctionArn":  "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		"Runtime":      "python3.9",
		"Handler":      "index.handler",
		"CodeSize":     1024,
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "aws-lambda-function", result.Pattern.Name)
	assert.Equal(t, "lambda:Function", result.TypeID)
}

func TestAWSPattern_IAMRoleDetection(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	data := map[string]interface{}{
		"RoleName":                 "my-role",
		"Arn":                      "arn:aws:iam::123456789012:role/my-role",
		"AssumeRolePolicyDocument": `{"Version": "2012-10-17", "Statement": []}`,
		"CreateDate":               "2023-01-15T10:30:00Z",
	}

	result := r.Detect(data)
	require.NotNil(t, result)
	assert.Equal(t, "aws-iam-role", result.Pattern.Name)
	assert.Equal(t, "iam:Role", result.TypeID)
}

func TestAWSPattern_NoMatch(t *testing.T) {
	r := NewRegistry()
	RegisterAWSPatterns(r)

	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{
			name: "random data",
			data: map[string]interface{}{"foo": "bar", "baz": 123},
		},
		{
			name: "partial EC2 (missing State)",
			data: map[string]interface{}{"InstanceId": "i-123", "InstanceType": "t2.micro"},
		},
		{
			name: "kubernetes resource",
			data: map[string]interface{}{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]interface{}{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.Detect(tt.data)
			// Either nil or not an AWS pattern
			if result != nil {
				assert.NotEqual(t, "aws", result.Pattern.Ecosystem)
			}
		})
	}
}

