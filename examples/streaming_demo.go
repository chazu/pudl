package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"pudl/internal/streaming"
)

func main() {
	fmt.Println("🚀 PUDL Streaming Parser Demo")
	fmt.Println("=============================")

	// Create streaming configuration
	config := streaming.DefaultStreamingConfig()
	config.MinChunkSize = 32    // Small for demo
	config.MaxChunkSize = 256   // Small for demo
	config.AvgChunkSize = 128   // Small for demo
	config.MaxMemoryMB = 50     // 50MB limit
	config.ChunkAlgorithm = "fastcdc"

	fmt.Printf("📋 Configuration:\n")
	fmt.Printf("   Algorithm: %s\n", config.ChunkAlgorithm)
	fmt.Printf("   Chunk sizes: %d - %d bytes (avg: %d)\n", 
		config.MinChunkSize, config.MaxChunkSize, config.AvgChunkSize)
	fmt.Printf("   Memory limit: %d MB\n", config.MaxMemoryMB)
	fmt.Println()

	// Create streaming parser
	parser, err := streaming.NewStreamingParser(config)
	if err != nil {
		log.Fatalf("Failed to create parser: %v", err)
	}
	defer parser.Close()

	// Add progress reporter
	reporter := streaming.NewCLIProgressReporter(true) // Verbose mode
	parser.SetProgressReporter(reporter)

	// Add schema detector
	err = parser.SetCUESchemaDetector(nil) // Pass nil for now, would be schema manager in real usage
	if err != nil {
		log.Printf("Warning: Failed to set schema detector: %v", err)
	}

	// Sample data - mix of JSON, CSV, YAML, and text with AWS/K8s patterns
	sampleData := `{"InstanceId": "i-1234567890abcdef0", "InstanceType": "t2.micro", "State": {"Name": "running", "Code": 16}, "ImageId": "ami-12345678"}
{"InstanceId": "i-0987654321fedcba0", "InstanceType": "t3.small", "State": {"Name": "stopped", "Code": 80}, "ImageId": "ami-87654321"}
{"Name": "my-test-bucket", "CreationDate": "2024-01-15T10:30:00Z", "Region": "us-east-1"}

name,age,city,occupation
Alice Brown,28,Houston,Designer
Charlie Wilson,42,Phoenix,Marketing Manager
Diana Davis,31,Philadelphia,Developer

---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx:latest
status:
  phase: Running
---
apiVersion: v1
kind: Service
metadata:
  name: test-service
spec:
  selector:
    app: test
  ports:
  - port: 80

This is some plain text content that might be found in log files or other unstructured data sources.
It contains multiple lines and various types of information that need to be processed.

{"event": "user_login", "timestamp": "2024-01-15T10:30:00Z", "user_id": "12345"}
{"event": "page_view", "timestamp": "2024-01-15T10:30:15Z", "user_id": "12345", "page": "/dashboard"}
{"event": "user_logout", "timestamp": "2024-01-15T11:45:30Z", "user_id": "12345"}

More text content here to demonstrate mixed format parsing.
The streaming parser should be able to handle this gracefully.`

	fmt.Printf("📄 Sample data size: %d bytes\n", len(sampleData))
	fmt.Println()

	// Parse the data
	reader := strings.NewReader(sampleData)
	ctx := context.Background()

	results, errors := parser.Parse(ctx, reader)

	// Process results
	chunkCount := 0
	totalObjects := 0
	formatCounts := make(map[string]int)

	fmt.Println("📦 Processing chunks:")
	fmt.Println()

	done := false
	for !done {
		select {
		case chunk, ok := <-results:
			if !ok {
				done = true
				break
			}

			chunkCount++
			totalObjects += len(chunk.Objects)
			formatCounts[chunk.Format]++

			fmt.Printf("Chunk %d:\n", chunkCount)
			fmt.Printf("  📏 Size: %d bytes\n", chunk.Size)
			fmt.Printf("  📋 Format: %s\n", chunk.Format)
			fmt.Printf("  📊 Objects: %d\n", len(chunk.Objects))
			fmt.Printf("  🔗 Hash: %s...\n", chunk.Hash[:8])
			
			if chunk.Schema != "" {
				fmt.Printf("  🏷️  Schema: %s (confidence: %.1f%%)\n", 
					chunk.Schema, chunk.Confidence*100)
			}

			if len(chunk.Errors) > 0 {
				fmt.Printf("  ⚠️  Errors: %d\n", len(chunk.Errors))
			}

			// Show first object if available
			if len(chunk.Objects) > 0 {
				fmt.Printf("  📝 First object: %+v\n", chunk.Objects[0])
			}

			fmt.Println()

		case err, ok := <-errors:
			if !ok {
				break
			}
			fmt.Printf("❌ Error: %v\n", err)

		case <-time.After(5 * time.Second):
			fmt.Println("⏰ Parsing timed out")
			done = true
		}
	}

	// Show final statistics
	stats := parser.Stats()
	
	fmt.Println("📊 Final Statistics:")
	fmt.Printf("   📦 Total chunks: %d\n", chunkCount)
	fmt.Printf("   📄 Total objects: %d\n", totalObjects)
	fmt.Printf("   📏 Bytes processed: %d\n", stats.BytesProcessed)
	fmt.Printf("   ⏱️  Duration: %v\n", stats.Duration)
	fmt.Printf("   ⚡ Throughput: %.2f MB/s\n", stats.Throughput)
	fmt.Printf("   💾 Memory usage: %d bytes\n", stats.MemoryUsage)
	fmt.Println()

	fmt.Println("📋 Format breakdown:")
	for format, count := range formatCounts {
		fmt.Printf("   %s: %d chunks\n", format, count)
	}

	fmt.Println()
	fmt.Println("✅ Demo completed successfully!")
}
