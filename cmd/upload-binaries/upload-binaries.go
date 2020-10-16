package main

import "os"

// Upload all of the per-platform evergreen binaries to S3.
func main() {
	awsKey := os.Getenv("AWS_KEY")
}
