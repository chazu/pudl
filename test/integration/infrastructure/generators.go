package infrastructure

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// TestFileGenerator creates realistic but completely synthetic test data
type TestFileGenerator struct {
	rand *rand.Rand
}

// NewTestFileGenerator creates a new test file generator
func NewTestFileGenerator() *TestFileGenerator {
	return &TestFileGenerator{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateAWSEC2Response creates synthetic AWS EC2 describe-instances response
func (g *TestFileGenerator) GenerateAWSEC2Response(instanceCount int) string {
	instances := make([]map[string]interface{}, instanceCount)
	
	for i := 0; i < instanceCount; i++ {
		instances[i] = map[string]interface{}{
			"InstanceId":   fmt.Sprintf("i-%016x", g.rand.Int63()),
			"ImageId":      fmt.Sprintf("ami-%08x", g.rand.Int31()),
			"State": map[string]interface{}{
				"Code": 16,
				"Name": "running",
			},
			"PrivateDnsName": fmt.Sprintf("ip-%d-%d-%d-%d.ec2.internal", 
				g.rand.Intn(256), g.rand.Intn(256), g.rand.Intn(256), g.rand.Intn(256)),
			"PublicDnsName": fmt.Sprintf("ec2-%d-%d-%d-%d.compute-1.amazonaws.com",
				g.rand.Intn(256), g.rand.Intn(256), g.rand.Intn(256), g.rand.Intn(256)),
			"StateTransitionReason": "",
			"InstanceType": []string{"t3.micro", "t3.small", "t3.medium", "m5.large", "c5.xlarge"}[g.rand.Intn(5)],
			"KeyName": fmt.Sprintf("test-key-%d", g.rand.Intn(10)),
			"LaunchTime": time.Now().Add(-time.Duration(g.rand.Intn(168)) * time.Hour).Format(time.RFC3339),
			"Placement": map[string]interface{}{
				"AvailabilityZone": []string{"us-east-1a", "us-east-1b", "us-east-1c"}[g.rand.Intn(3)],
				"GroupName":        "",
				"Tenancy":          "default",
			},
			"VpcId":    fmt.Sprintf("vpc-%08x", g.rand.Int31()),
			"SubnetId": fmt.Sprintf("subnet-%08x", g.rand.Int31()),
			"SecurityGroups": []map[string]interface{}{
				{
					"GroupName": "default",
					"GroupId":   fmt.Sprintf("sg-%08x", g.rand.Int31()),
				},
			},
			"Tags": []map[string]interface{}{
				{
					"Key":   "Name",
					"Value": fmt.Sprintf("test-instance-%d", i+1),
				},
				{
					"Key":   "Environment",
					"Value": []string{"production", "staging", "development"}[g.rand.Intn(3)],
				},
			},
		}
	}
	
	response := map[string]interface{}{
		"Reservations": []map[string]interface{}{
			{
				"Instances": instances,
				"OwnerId":   fmt.Sprintf("%012d", g.rand.Int63n(999999999999)),
				"RequesterId": "",
				"ReservationId": fmt.Sprintf("r-%08x", g.rand.Int31()),
			},
		},
		"ResponseMetadata": map[string]interface{}{
			"RequestId": fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
				g.rand.Int31(), g.rand.Int31n(65536), g.rand.Int31n(65536),
				g.rand.Int31n(65536), g.rand.Int63()),
			"HTTPStatusCode": 200,
			"HTTPHeaders": map[string]string{
				"content-type": "text/xml;charset=UTF-8",
				"date":         time.Now().Format(time.RFC1123),
			},
		},
	}
	
	jsonBytes, _ := json.MarshalIndent(response, "", "  ")
	return string(jsonBytes)
}

// GenerateAWSS3Response creates synthetic AWS S3 list-buckets response
func (g *TestFileGenerator) GenerateAWSS3Response(bucketCount int) string {
	buckets := make([]map[string]interface{}, bucketCount)
	
	for i := 0; i < bucketCount; i++ {
		buckets[i] = map[string]interface{}{
			"Name": fmt.Sprintf("test-bucket-%d-%08x", i+1, g.rand.Int31()),
			"CreationDate": time.Now().Add(-time.Duration(g.rand.Intn(365)) * 24 * time.Hour).Format(time.RFC3339),
		}
	}
	
	response := map[string]interface{}{
		"Buckets": buckets,
		"Owner": map[string]interface{}{
			"DisplayName": "test-user",
			"ID": fmt.Sprintf("%064x", g.rand.Int63()),
		},
		"ResponseMetadata": map[string]interface{}{
			"RequestId": fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
				g.rand.Int31(), g.rand.Int31n(65536), g.rand.Int31n(65536),
				g.rand.Int31n(65536), g.rand.Int63()),
			"HTTPStatusCode": 200,
		},
	}
	
	jsonBytes, _ := json.MarshalIndent(response, "", "  ")
	return string(jsonBytes)
}

// GenerateKubernetesPods creates synthetic Kubernetes Pod list
func (g *TestFileGenerator) GenerateKubernetesPods(podCount int) string {
	pods := make([]map[string]interface{}, podCount)
	
	namespaces := []string{"default", "kube-system", "production", "staging", "monitoring"}
	
	for i := 0; i < podCount; i++ {
		namespace := namespaces[g.rand.Intn(len(namespaces))]
		podName := fmt.Sprintf("test-pod-%d-%08x", i+1, g.rand.Int31())
		
		pods[i] = map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      podName,
				"namespace": namespace,
				"uid":       fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
					g.rand.Int31(), g.rand.Int31n(65536), g.rand.Int31n(65536),
					g.rand.Int31n(65536), g.rand.Int63()),
				"creationTimestamp": time.Now().Add(-time.Duration(g.rand.Intn(168)) * time.Hour).Format(time.RFC3339),
				"labels": map[string]interface{}{
					"app":     fmt.Sprintf("test-app-%d", (i%5)+1),
					"version": []string{"v1.0", "v1.1", "v2.0"}[g.rand.Intn(3)],
					"tier":    []string{"frontend", "backend", "database"}[g.rand.Intn(3)],
				},
			},
			"spec": map[string]interface{}{
				"containers": []map[string]interface{}{
					{
						"name":  fmt.Sprintf("container-%d", i+1),
						"image": fmt.Sprintf("test-registry/test-app:%s", []string{"latest", "v1.0", "v2.0"}[g.rand.Intn(3)]),
						"ports": []map[string]interface{}{
							{
								"containerPort": []int{8080, 8443, 9090, 3000}[g.rand.Intn(4)],
								"protocol":      "TCP",
							},
						},
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"cpu":    fmt.Sprintf("%dm", 100+g.rand.Intn(900)),
								"memory": fmt.Sprintf("%dMi", 128+g.rand.Intn(896)),
							},
							"limits": map[string]interface{}{
								"cpu":    fmt.Sprintf("%dm", 500+g.rand.Intn(1500)),
								"memory": fmt.Sprintf("%dMi", 256+g.rand.Intn(1792)),
							},
						},
					},
				},
				"restartPolicy": "Always",
				"nodeName":      fmt.Sprintf("node-%d", g.rand.Intn(10)+1),
			},
			"status": map[string]interface{}{
				"phase": []string{"Running", "Pending", "Succeeded"}[g.rand.Intn(3)],
				"podIP": fmt.Sprintf("10.%d.%d.%d", 
					g.rand.Intn(256), g.rand.Intn(256), g.rand.Intn(256)),
				"startTime": time.Now().Add(-time.Duration(g.rand.Intn(168)) * time.Hour).Format(time.RFC3339),
			},
		}
	}
	
	// Convert to proper YAML format
	var yamlLines []string
	yamlLines = append(yamlLines, "apiVersion: v1")
	yamlLines = append(yamlLines, "kind: List")
	yamlLines = append(yamlLines, "items:")

	for _, pod := range pods {
		yamlLines = append(yamlLines, "- apiVersion: v1")
		yamlLines = append(yamlLines, "  kind: Pod")
		yamlLines = append(yamlLines, "  metadata:")
		yamlLines = append(yamlLines, fmt.Sprintf("    name: %s", pod["metadata"].(map[string]interface{})["name"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    namespace: %s", pod["metadata"].(map[string]interface{})["namespace"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    uid: %s", pod["metadata"].(map[string]interface{})["uid"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    creationTimestamp: %s", pod["metadata"].(map[string]interface{})["creationTimestamp"]))
		yamlLines = append(yamlLines, "    labels:")
		labels := pod["metadata"].(map[string]interface{})["labels"].(map[string]interface{})
		for k, v := range labels {
			yamlLines = append(yamlLines, fmt.Sprintf("      %s: %s", k, v))
		}
		yamlLines = append(yamlLines, "  spec:")
		yamlLines = append(yamlLines, "    containers:")
		containers := pod["spec"].(map[string]interface{})["containers"].([]map[string]interface{})
		for _, container := range containers {
			yamlLines = append(yamlLines, fmt.Sprintf("    - name: %s", container["name"]))
			yamlLines = append(yamlLines, fmt.Sprintf("      image: %s", container["image"]))
		}
		yamlLines = append(yamlLines, fmt.Sprintf("    restartPolicy: %s", pod["spec"].(map[string]interface{})["restartPolicy"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    nodeName: %s", pod["spec"].(map[string]interface{})["nodeName"]))
		yamlLines = append(yamlLines, "  status:")
		yamlLines = append(yamlLines, fmt.Sprintf("    phase: %s", pod["status"].(map[string]interface{})["phase"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    podIP: %s", pod["status"].(map[string]interface{})["podIP"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    startTime: %s", pod["status"].(map[string]interface{})["startTime"]))
	}

	return strings.Join(yamlLines, "\n")
}

// GenerateKubernetesServices creates synthetic Kubernetes Service list
func (g *TestFileGenerator) GenerateKubernetesServices(serviceCount int) string {
	services := make([]map[string]interface{}, serviceCount)
	
	namespaces := []string{"default", "kube-system", "production", "staging"}
	
	for i := 0; i < serviceCount; i++ {
		namespace := namespaces[g.rand.Intn(len(namespaces))]
		serviceName := fmt.Sprintf("test-service-%d", i+1)
		
		services[i] = map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      serviceName,
				"namespace": namespace,
				"uid":       fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
					g.rand.Int31(), g.rand.Int31n(65536), g.rand.Int31n(65536),
					g.rand.Int31n(65536), g.rand.Int63()),
				"creationTimestamp": time.Now().Add(-time.Duration(g.rand.Intn(168)) * time.Hour).Format(time.RFC3339),
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"app": fmt.Sprintf("test-app-%d", (i%5)+1),
				},
				"ports": []map[string]interface{}{
					{
						"port":       []int{80, 443, 8080, 9090}[g.rand.Intn(4)],
						"targetPort": []int{8080, 8443, 9090, 3000}[g.rand.Intn(4)],
						"protocol":   "TCP",
					},
				},
				"type": []string{"ClusterIP", "NodePort", "LoadBalancer"}[g.rand.Intn(3)],
			},
			"status": map[string]interface{}{
				"loadBalancer": map[string]interface{}{},
			},
		}
	}
	
	// Convert to proper YAML format
	var yamlLines []string
	yamlLines = append(yamlLines, "apiVersion: v1")
	yamlLines = append(yamlLines, "kind: List")
	yamlLines = append(yamlLines, "items:")

	for _, service := range services {
		yamlLines = append(yamlLines, "- apiVersion: v1")
		yamlLines = append(yamlLines, "  kind: Service")
		yamlLines = append(yamlLines, "  metadata:")
		yamlLines = append(yamlLines, fmt.Sprintf("    name: %s", service["metadata"].(map[string]interface{})["name"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    namespace: %s", service["metadata"].(map[string]interface{})["namespace"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    uid: %s", service["metadata"].(map[string]interface{})["uid"]))
		yamlLines = append(yamlLines, fmt.Sprintf("    creationTimestamp: %s", service["metadata"].(map[string]interface{})["creationTimestamp"]))
		yamlLines = append(yamlLines, "  spec:")
		yamlLines = append(yamlLines, "    selector:")
		selector := service["spec"].(map[string]interface{})["selector"].(map[string]interface{})
		for k, v := range selector {
			yamlLines = append(yamlLines, fmt.Sprintf("      %s: %s", k, v))
		}
		yamlLines = append(yamlLines, "    ports:")
		ports := service["spec"].(map[string]interface{})["ports"].([]map[string]interface{})
		for _, port := range ports {
			yamlLines = append(yamlLines, fmt.Sprintf("    - port: %v", port["port"]))
			yamlLines = append(yamlLines, fmt.Sprintf("      targetPort: %v", port["targetPort"]))
			yamlLines = append(yamlLines, fmt.Sprintf("      protocol: %s", port["protocol"]))
		}
		yamlLines = append(yamlLines, fmt.Sprintf("    type: %s", service["spec"].(map[string]interface{})["type"]))
	}

	return strings.Join(yamlLines, "\n")
}

// GenerateLargeNDJSON creates a large NDJSON file for performance testing
func (g *TestFileGenerator) GenerateLargeNDJSON(recordCount int) string {
	var lines []string
	
	for i := 0; i < recordCount; i++ {
		record := map[string]interface{}{
			"id":        fmt.Sprintf("record-%08d", i+1),
			"timestamp": time.Now().Add(-time.Duration(g.rand.Intn(86400)) * time.Second).Format(time.RFC3339),
			"level":     []string{"INFO", "WARN", "ERROR", "DEBUG"}[g.rand.Intn(4)],
			"message":   fmt.Sprintf("Test log message %d with some random data %08x", i+1, g.rand.Int31()),
			"source":    fmt.Sprintf("service-%d", g.rand.Intn(10)+1),
			"host":      fmt.Sprintf("host-%d.example.com", g.rand.Intn(50)+1),
			"metrics": map[string]interface{}{
				"cpu":    g.rand.Float64() * 100,
				"memory": g.rand.Float64() * 8192,
				"disk":   g.rand.Float64() * 1024,
			},
		}
		
		jsonBytes, _ := json.Marshal(record)
		lines = append(lines, string(jsonBytes))
	}
	
	return strings.Join(lines, "\n")
}

// GenerateCorruptedJSON creates JSON with specific types of corruption
func (g *TestFileGenerator) GenerateCorruptedJSON(corruptionType string) string {
	baseJSON := `{
  "id": "test-item-001",
  "name": "Test Item",
  "value": 42,
  "active": true,
  "metadata": {
    "created": "2023-01-01T00:00:00Z",
    "tags": ["test", "example"]
  }
}`
	
	switch corruptionType {
	case "missing_brace":
		return strings.TrimSuffix(baseJSON, "}")
	case "invalid_json":
		return strings.ReplaceAll(baseJSON, `"value": 42`, `"value": 42,`)
	case "truncated":
		return baseJSON[:len(baseJSON)/2]
	case "invalid_utf8":
		return baseJSON + "\xff\xfe"
	default:
		return baseJSON
	}
}
