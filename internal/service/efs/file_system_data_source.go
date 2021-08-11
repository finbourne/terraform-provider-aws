package efs

import (
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/terraform-providers/terraform-provider-aws/internal/client"
	"github.com/terraform-providers/terraform-provider-aws/internal/keyvaluetags"
	"github.com/terraform-providers/terraform-provider-aws/internal/tags"
)

func DataSourceFileSystem() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceAwsEfsFileSystemRead,

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"availability_zone_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"availability_zone_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"creation_token": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ValidateFunc: validation.StringLenBetween(0, 64),
			},
			"encrypted": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"file_system_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"kms_key_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"performance_mode": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"dns_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tags": tags.TagsSchemaComputed(),
			"throughput_mode": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"provisioned_throughput_in_mibps": {
				Type:     schema.TypeFloat,
				Computed: true,
			},
			"size_in_bytes": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"lifecycle_policy": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"transition_to_ia": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func dataSourceAwsEfsFileSystemRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).EFSConn
	ignoreTagsConfig := meta.(*client.AWSClient).IgnoreTagsConfig

	describeEfsOpts := &efs.DescribeFileSystemsInput{}

	if v, ok := d.GetOk("creation_token"); ok {
		describeEfsOpts.CreationToken = aws.String(v.(string))
	}

	if v, ok := d.GetOk("file_system_id"); ok {
		describeEfsOpts.FileSystemId = aws.String(v.(string))
	}

	log.Printf("[DEBUG] Reading EFS File System: %s", describeEfsOpts)
	describeResp, err := conn.DescribeFileSystems(describeEfsOpts)
	if err != nil {
		return fmt.Errorf("error reading EFS FileSystem: %w", err)
	}

	if describeResp == nil || len(describeResp.FileSystems) == 0 {
		return errors.New("error reading EFS FileSystem: empty output")
	}

	if len(describeResp.FileSystems) > 1 {
		return fmt.Errorf("Search returned %d results, please revise so only one is returned", len(describeResp.FileSystems))
	}

	fs := describeResp.FileSystems[0]

	d.SetId(aws.StringValue(fs.FileSystemId))
	d.Set("availability_zone_id", fs.AvailabilityZoneId)
	d.Set("availability_zone_name", fs.AvailabilityZoneName)
	d.Set("creation_token", fs.CreationToken)
	d.Set("performance_mode", fs.PerformanceMode)

	fsARN := arn.ARN{
		AccountID: meta.(*client.AWSClient).AccountID,
		Partition: meta.(*client.AWSClient).Partition,
		Region:    meta.(*client.AWSClient).Region,
		Resource:  fmt.Sprintf("file-system/%s", aws.StringValue(fs.FileSystemId)),
		Service:   "elasticfilesystem",
	}.String()

	d.Set("arn", fsARN)
	d.Set("file_system_id", fs.FileSystemId)
	d.Set("encrypted", fs.Encrypted)
	d.Set("kms_key_id", fs.KmsKeyId)
	d.Set("provisioned_throughput_in_mibps", fs.ProvisionedThroughputInMibps)
	d.Set("throughput_mode", fs.ThroughputMode)
	if fs.SizeInBytes != nil {
		d.Set("size_in_bytes", fs.SizeInBytes.Value)
	}

	if err := d.Set("tags", keyvaluetags.EfsKeyValueTags(fs.Tags).IgnoreAws().IgnoreConfig(ignoreTagsConfig).Map()); err != nil {
		return fmt.Errorf("error setting tags: %w", err)
	}

	res, err := conn.DescribeLifecycleConfiguration(&efs.DescribeLifecycleConfigurationInput{
		FileSystemId: fs.FileSystemId,
	})
	if err != nil {
		return fmt.Errorf("Error describing lifecycle configuration for EFS file system (%s): %w",
			aws.StringValue(fs.FileSystemId), err)
	}

	if err := d.Set("lifecycle_policy", flattenEfsFileSystemLifecyclePolicies(res.LifecyclePolicies)); err != nil {
		return fmt.Errorf("error setting lifecycle_policy: %w", err)
	}

	d.Set("dns_name", meta.(*client.AWSClient).RegionalHostname(fmt.Sprintf("%s.efs", aws.StringValue(fs.FileSystemId))))

	return nil
}