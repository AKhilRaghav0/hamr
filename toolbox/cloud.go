package toolbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/AKhilRaghav0/hamr"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultCloudTimeout = 30 * time.Second
)

// CloudConfig holds configuration for AWS cloud tools.
type CloudConfig struct {
	timeout  time.Duration
	region   string
	endpoint string
}

// CloudOption is a functional option for CloudTools.
type CloudOption func(*CloudConfig)

// WithCloudTimeout sets the AWS API timeout. Default is 30 seconds.
func WithCloudTimeout(d time.Duration) CloudOption {
	return func(c *CloudConfig) {
		c.timeout = d
	}
}

// WithCloudRegion sets the AWS region for the client.
func WithCloudRegion(region string) CloudOption {
	return func(c *CloudConfig) {
		c.region = region
	}
}

// WithCloudEndpoint sets a custom endpoint for testing or non-AWS S3-compatible services.
func WithCloudEndpoint(endpoint string) CloudOption {
	return func(c *CloudConfig) {
		c.endpoint = endpoint
	}
}

// CloudTools is a collection of AWS cloud service tools.
type CloudTools struct {
	s3Client *s3.Client
	ec2Client *ec2.Client
	cfg      CloudConfig
}

// Cloud returns a CloudTools collection with the given options applied.
// Uses the default AWS credential chain if no credentials are provided.
func Cloud(opts ...CloudOption) *CloudTools {
	cfg := CloudConfig{
		timeout: defaultCloudTimeout,
	}
	for _, o := range opts {
		o(&cfg)
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		func(o *config.LoadOptions) error {
			if cfg.region != "" {
				o.Region = cfg.region
}

// EC2DescribeInstancesInput is the input for the ec2_describe_instances tool.
type EC2DescribeInstancesInput struct {
	InstanceIds []string `json:"instance_ids" desc:"EC2 instance IDs to describe" optional:"true"`
	Filters     []EC2Filter `json:"filters" desc:"filters to apply" optional:"true"`
}

// EC2Filter represents a filter for EC2 describe instances.
type EC2Filter struct {
	Name   string   `json:"name" desc:"filter name"`
	Values []string `json:"values" desc:"filter values"`
}

// EC2StartInstancesInput is the input for the ec2_start_instances tool.
type EC2StartInstancesInput struct {
	InstanceIds []string `json:"instance_ids" desc:"EC2 instance IDs to start"`
}

// EC2StopInstancesInput is the input for the ec2_stop_instances tool.
type EC2StopInstancesInput struct {
	InstanceIds []string `json:"instance_ids" desc:"EC2 instance IDs to stop"`
}

// EC2TerminateInstancesInput is the input for the ec2_terminate_instances tool.
type EC2TerminateInstancesInput struct {
	InstanceIds []string `json:"instance_ids" desc:"EC2 instance IDs to terminate"`
}

// ec2DescribeInstances describes EC2 instances.
func (c *CloudTools) ec2DescribeInstances(ctx context.Context, in EC2DescribeInstancesInput) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	var instanceIds []*string
	for _, id := range in.InstanceIds {
		instanceIds = append(instanceIds, aws.String(id))
	}

	var filters []ec2types.Filter
	for _, f := range in.Filters {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String(f.Name),
			Values: aws.StringSlice(f.Values),
		})
	}

	out, err := c.ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: instanceIds,
		Filters:     filters,
	})
	if err != nil {
		return "", fmt.Errorf("ec2_describe_instances: %w", err)
	}

	var reservations []map[string]any
	for _, res := range out.Reservations {
		resInfo := map[string]any{
			"reservation_id": aws.ToString(res.ReservationId),
			"owner_id":       aws.ToString(res.OwnerId),
			"groups":         aws.ToStringSlice(res.Groups),
		}
		var instances []map[string]any
		for _, inst := range res.Instances {
			instInfo := map[string]any{
				"instance_id":    aws.ToString(inst.InstanceId),
				"instance_type":  aws.ToString(inst.InstanceType),
				"state":          aws.ToString(inst.State.Name),
				"public_ip":      aws.ToString(inst.PublicIpAddress),
				"private_ip":     aws.ToString(inst.PrivateIpAddress),
				"launch_time":    aws.ToTime(inst.LaunchTime).Format(time.RFC3339),
			}
			instances = append(instances, instInfo)
		}
		resInfo["instances"] = instances
		reservations = append(reservations, resInfo)
	}

	data, err := json.Marshal(reservations)
	if err != nil {
		return "", fmt.Errorf("ec2_describe_instances: marshal: %w", err)
	}
	return string(data), nil
}

// ec2StartInstances starts EC2 instances.
func (c *CloudTools) ec2StartInstances(ctx context.Context, in EC2StartInstancesInput) (string, error) {
	if len(in.InstanceIds) == 0 {
		return "", fmt.Errorf("ec2_start_instances: at least one instance ID must be provided")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	var instanceIds []*string
	for _, id := range in.InstanceIds {
		instanceIds = append(instanceIds, aws.String(id))
	}

	out, err := c.ec2Client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: instanceIds,
	})
	if err != nil {
		return "", fmt.Errorf("ec2_start_instances: %w", err)
	}

	var startingInstances []map[string]any
	for _, inst := range out.StartingInstances {
		startingInstances = append(startingInstances, map[string]any{
			"instance_id": aws.ToString(inst.InstanceId),
			"current_state": map[string]any{
				"code":   aws.ToInt64(inst.CurrentState.Code),
				"name":   aws.ToString(inst.CurrentState.Name),
			},
			"previous_state": map[string]any{
				"code":   aws.ToInt64(inst.PreviousState.Code),
				"name":   aws.ToString(inst.PreviousState.Name),
			},
		})
	}

	data, err := json.Marshal(startingInstances)
	if err != nil {
		return "", fmt.Errorf("ec2_start_instances: marshal: %w", err)
	}
	return string(data), nil
}

// ec2StopInstances stops EC2 instances.
func (c *CloudTools) ec2StopInstances(ctx context.Context, in EC2StopInstancesInput) (string, error) {
	if len(in.InstanceIds) == 0 {
		return "", fmt.Errorf("ec2_stop_instances: at least one instance ID must be provided")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	var instanceIds []*string
	for _, id := range in.InstanceIds {
		instanceIds = append(instanceIds, aws.String(id))
	}

	out, err := c.ec2Client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: instanceIds,
	})
	if err != nil {
		return "", fmt.Errorf("ec2_stop_instances: %w", err)
	}

	var stoppingInstances []map[string]any
	for _, inst := range out.StoppingInstances {
		stoppingInstances = append(stoppingInstances, map[string]any{
			"instance_id": aws.ToString(inst.InstanceId),
			"current_state": map[string]any{
				"code":   aws.ToInt64(inst.CurrentState.Code),
				"name":   aws.ToString(inst.CurrentState.Name),
			},
			"previous_state": map[string]any{
				"code":   aws.ToInt64(inst.PreviousState.Code),
				"name":   aws.ToString(inst.PreviousState.Name),
			},
		})
	}

	data, err := json.Marshal(stoppingInstances)
	if err != nil {
		return "", fmt.Errorf("ec2_stop_instances: marshal: %w", err)
	}
	return string(data), nil
}

// ec2TerminateInstances terminates EC2 instances.
func (c *CloudTools) ec2TerminateInstances(ctx context.Context, in EC2TerminateInstancesInput) (string, error) {
	if len(in.InstanceIds) == 0 {
		return "", fmt.Errorf("ec2_terminate_instances: at least one instance ID must be provided")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	var instanceIds []*string
	for _, id := range in.InstanceIds {
		instanceIds = append(instanceIds, aws.String(id))
	}

	out, err := c.ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: instanceIds,
	})
	if err != nil {
		return "", fmt.Errorf("ec2_terminate_instances: %w", err)
	}

	var terminatingInstances []map[string]any
	for _, inst := range out.TerminatingInstances {
		terminatingInstances = append(terminatingInstances, map[string]any{
			"instance_id": aws.ToString(inst.InstanceId),
			"current_state": map[string]any{
				"code":   aws.ToInt64(inst.CurrentState.Code),
				"name":   aws.ToString(inst.CurrentState.Name),
			},
			"previous_state": map[string]any{
				"code":   aws.ToInt64(inst.PreviousState.Code),
				"name":   aws.ToString(inst.PreviousState.Name),
			},
		})
	}

	data, err := json.Marshal(terminatingInstances)
	if err != nil {
		return "", fmt.Errorf("ec2_terminate_instances: marshal: %w", err)
	}
	return string(data), nil
}
			return nil
		},
	)
	if err != nil {
		panic(fmt.Sprintf("toolbox/cloud: cannot load AWS config: %v", err))
	}

	var s3Client *s3.Client
	var ec2Client *ec2.Client
	if cfg.endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: cfg.endpoint}, nil
		})
		awsCfg.EndpointResolverWithOptions = customResolver
	}
	s3Client = s3.NewFromConfig(awsCfg)
	ec2Client = ec2.NewFromConfig(awsCfg)

	return &CloudTools{
		s3Client: s3Client,
		ec2Client: ec2Client,
		cfg:      cfg,
	}
}

// Tools implements hamr.ToolCollection.
func (c *CloudTools) Tools() []hamr.ToolInfo {
	return []hamr.ToolInfo{
		{
			Name:        "s3_list_buckets",
			Description: "List all S3 buckets in the AWS account.",
			Handler:     c.s3ListBuckets,
		},
		{
			Name:        "s3_list_objects",
			Description: "List objects in an S3 bucket with optional prefix filtering.",
			Handler:     c.s3ListObjects,
		},
		{
			Name:        "s3_get_object",
			Description: "Get an object from an S3 bucket by key.",
			Handler:     c.s3GetObject,
		},
		{
			Name:        "s3_put_object",
			Description: "Put an object into an S3 bucket with the specified key.",
			Handler:     c.s3PutObject,
		},
		{
			Name:        "s3_delete_object",
			Description: "Delete an object from an S3 bucket by key.",
			Handler:     c.s3DeleteObject,
		},
		{
			Name:        "ec2_describe_instances",
			Description: "Describe EC2 instances with optional filtering.",
			Handler:     c.ec2DescribeInstances,
		},
		{
			Name:        "ec2_start_instances",
			Description: "Start EC2 instances by instance IDs.",
			Handler:     c.ec2StartInstances,
		},
		{
			Name:        "ec2_stop_instances",
			Description: "Stop EC2 instances by instance IDs.",
			Handler:     c.ec2StopInstances,
		},
		{
			Name:        "ec2_terminate_instances",
			Description: "Terminate EC2 instances by instance IDs.",
			Handler:     c.ec2TerminateInstances,
		},
	}
}

// S3ListBucketsInput is the input for the s3_list_buckets tool.
type S3ListBucketsInput struct{}

// S3ListObjectsInput is the input for the s3_list_objects tool.
type S3ListObjectsInput struct {
	Bucket string `json:"bucket" desc:"name of the S3 bucket"`
	Prefix string `json:"prefix" desc:"optional prefix to filter objects" optional:"true"`
}

// S3GetObjectInput is the input for the s3_get_object tool.
type S3GetObjectInput struct {
	Bucket string `json:"bucket" desc:"name of the S3 bucket"`
	Key    string `json:"key" desc:"object key to retrieve"`
}

// S3PutObjectInput is the input for the s3_put_object tool.
type S3PutObjectInput struct {
	Bucket string `json:"bucket" desc:"name of the S3 bucket"`
	Key    string `json:"key" desc:"object key to write"`
	Body   string `json:"body" desc:"content to store in the object"`
}

// S3DeleteObjectInput is the input for the s3_delete_object tool.
type S3DeleteObjectInput struct {
	Bucket string `json:"bucket" desc:"name of the S3 bucket"`
	Key    string `json:"key" desc:"object key to delete"`
}

// s3ListBuckets lists all S3 buckets.
func (c *CloudTools) s3ListBuckets(ctx context.Context, _ S3ListBucketsInput) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	out, err := c.s3Client.ListBuckets(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("s3_list_buckets: %w", err)
	}

	if len(out.Buckets) == 0 {
		return "no S3 buckets found", nil
	}

	buckets := make([]string, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		if b.Name != nil {
			buckets = append(buckets, *b.Name)
		}
	}

	data, err := json.Marshal(buckets)
	if err != nil {
		return "", fmt.Errorf("s3_list_buckets: marshal: %w", err)
	}
	return string(data), nil
}

// s3ListObjects lists objects in an S3 bucket.
func (c *CloudTools) s3ListObjects(ctx context.Context, in S3ListObjectsInput) (string, error) {
	if in.Bucket == "" {
		return "", fmt.Errorf("s3_list_objects: bucket name must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	params := &s3.ListObjectsV2Input{
		Bucket: aws.String(in.Bucket),
	}
	if in.Prefix != "" {
		params.Prefix = aws.String(in.Prefix)
	}

	out, err := c.s3Client.ListObjectsV2(ctx, params)
	if err != nil {
		return "", fmt.Errorf("s3_list_objects: %w", err)
	}

	if len(out.Contents) == 0 {
		return fmt.Sprintf("no objects found in bucket %s", in.Bucket), nil
	}

	objects := make([]map[string]any, 0, len(out.Contents))
	for _, obj := range out.Contents {
		objInfo := map[string]any{
			"key":       aws.ToString(obj.Key),
			"size":      aws.ToInt64(obj.Size),
			"last_modified": aws.ToTime(obj.LastModified).Format(time.RFC3339),
		}
		objects = append(objects, objInfo)
	}

	data, err := json.Marshal(objects)
	if err != nil {
		return "", fmt.Errorf("s3_list_objects: marshal: %w", err)
	}
	return string(data), nil
}

// s3GetObject retrieves an object from S3.
func (c *CloudTools) s3GetObject(ctx context.Context, in S3GetObjectInput) (string, error) {
	if in.Bucket == "" || in.Key == "" {
		return "", fmt.Errorf("s3_get_object: bucket and key must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	out, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
	})
	if err != nil {
		return "", fmt.Errorf("s3_get_object: %w", err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(io.LimitReader(out.Body, 1<<20)) // 1 MiB max
	if err != nil {
		return "", fmt.Errorf("s3_get_object: read: %w", err)
	}

	return string(data), nil
}

// s3PutObject stores an object in S3.
func (c *CloudTools) s3PutObject(ctx context.Context, in S3PutObjectInput) (string, error) {
	if in.Bucket == "" || in.Key == "" {
		return "", fmt.Errorf("s3_put_object: bucket and key must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
		Body:   io.NopCloser(strings.NewReader(in.Body)),
	})
	if err != nil {
		return "", fmt.Errorf("s3_put_object: %w", err)
	}

	return fmt.Sprintf("wrote %d bytes to s3://%s/%s", len(in.Body), in.Bucket, in.Key), nil
}

// s3DeleteObject deletes an object from S3.
func (c *CloudTools) s3DeleteObject(ctx context.Context, in S3DeleteObjectInput) (string, error) {
	if in.Bucket == "" || in.Key == "" {
		return "", fmt.Errorf("s3_delete_object: bucket and key must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(in.Bucket),
		Key:    aws.String(in.Key),
	})
	if err != nil {
		return "", fmt.Errorf("s3_delete_object: %w", err)
	}

	return fmt.Sprintf("deleted s3://%s/%s", in.Bucket, in.Key), nil
}