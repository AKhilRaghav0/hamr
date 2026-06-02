package toolbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AKhilRaghav0/hamr"
)

// CloudProviderTools provides tools for interacting with cloud providers (AWS S3, EC2).
type CloudProviderTools struct {
	// Region is the AWS region to use.
	Region string
	// AccessKey and SecretKey are for authentication (stub).
	AccessKey string
	SecretKey string
}

// CloudProviderOptions are functional options for configuring the CloudProviderTools.
type CloudProviderOption func(*CloudProviderTools)

// WithRegion sets the AWS region.
func WithRegion(region string) CloudProviderOption {
	return func(c *CloudProviderTools) {
		c.Region = region
	}
}

// WithCredentials sets the access key and secret key.
func WithCredentials(accessKey, secretKey string) CloudProviderOption {
	return func(c *CloudProviderTools) {
		c.AccessKey = accessKey
		c.SecretKey = secretKey
	}
}

// CloudProvider returns a new CloudProviderTools instance with the given options.
func CloudProvider(opts ...CloudProviderOption) *CloudProviderTools {
	c := &CloudProviderTools{
		Region: "us-east-1",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// S3GetObject returns a tool handler that simulates getting an object from S3.
func (c *CloudProviderTools) S3GetObject() interface{} {
	return func(ctx context.Context, args struct {
		Bucket string `json:"bucket" desc:"S3 bucket name" required:"true"`
		Key    string `json:"key" desc:"Object key" required:"true"`
	}) (interface{}, error) {
		if args.Bucket == "" || args.Key == "" {
			return nil, errors.New("bucket and key are required")
		}
		// Simulate retrieving an object.
		return fmt.Sprintf("content of %s/%s (from region %s)", args.Bucket, args.Key, c.Region), nil
	}
}

// S3PutObject returns a tool handler that simulates putting an object to S3.
func (c *CloudProviderTools) S3PutObject() interface{} {
	return func(ctx context.Context, args struct {
		Bucket string `json:"bucket" desc:"S3 bucket name" required:"true"`
		Key    string `json:"key" desc:"Object key" required:"true"`
		Body   string `json:"body" desc:"Object body" required:"true"`
	}) (interface{}, error) {
		if args.Bucket == "" || args.Key == "" || args.Body == "" {
			return nil, errors.New("bucket, key, and body are required")
		}
		// Simulate storing the object.
		return fmt.Sprintf("stored %s/%s with size %d bytes (region %s)", args.Bucket, args.Key, len(args.Body), c.Region), nil
	}
}

// EC2DescribeInstances returns a tool handler that simulates describing EC2 instances.
func (c *CloudProviderTools) EC2DescribeInstances() interface{} {
	return func(ctx context.Context, args struct {
		InstanceIds []string `json:"instance_ids" desc:"List of instance IDs to describe"`
	}) (interface{}, error) {
		// Simulate returning instance information.
		instances := []struct {
			ID       string `json:"id"`
			State    string `json:"state"`
			InstanceType string `json:"instance_type"`
			Region   string `json:"region"`
		}{}
		if len(args.InstanceIds) == 0 {
			// Return some dummy instances.
			instances = append(instances, struct {
				ID       string `json:"id"`
				State    string `json:"state"`
				InstanceType string `json:"instance_type"`
				Region   string `json:"region"`
			}{ID: "i-1234567890abcdef0", State: "running", InstanceType: "t2.micro", Region: c.Region})
			instances = append(instances, struct {
				ID       string `json:"id"`
				State    string `json:"state"`
				InstanceType string `json:"instance_type"`
				Region   string `json:"region"`
			}{ID: "i-0987654321fedcba0", State: "stopped", InstanceType: "t3.small", Region: c.Region})
		} else {
			for _, id := range args.InstanceIds {
				instances = append(instances, struct {
					ID       string `json:"id"`
					State    string `json:"state"`
					InstanceType string `json:"instance_type"`
					Region   string `json:"region"`
				}{ID: id, State: "running", InstanceType: "t2.micro", Region: c.Region})
			}
		}
		return instances, nil
	}
}

// EC2StartInstances returns a tool handler that simulates starting EC2 instances.
func (c *CloudProviderTools) EC2StartInstances() interface{} {
	return func(ctx context.Context, args struct {
		InstanceIds []string `json:"instance_ids" desc:"Instance IDs to start" required:"true"`
	}) (interface{}, error) {
		if len(args.InstanceIds) == 0 {
			return nil, errors.New("at least one instance ID is required")
		}
		// Simulate starting instances.
		started := []string{}
		for _, id := range args.InstanceIds {
			started = append(started, id)
		}
		return map[string]interface{}{
			"started": started,
			"region":  c.Region,
		}, nil
	}
}