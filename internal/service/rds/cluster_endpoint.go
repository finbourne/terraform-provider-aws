package rds

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/terraform-providers/terraform-provider-aws/internal/client"
	"github.com/terraform-providers/terraform-provider-aws/internal/flex"
	"github.com/terraform-providers/terraform-provider-aws/internal/keyvaluetags"
	"github.com/terraform-providers/terraform-provider-aws/internal/tags"
)

const (
	AWSRDSClusterEndpointCreateTimeout   = 30 * time.Minute
	AWSRDSClusterEndpointRetryDelay      = 5 * time.Second
	AWSRDSClusterEndpointRetryMinTimeout = 3 * time.Second
)

func ResourceClusterEndpoint() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsRDSClusterEndpointCreate,
		Read:   resourceAwsRDSClusterEndpointRead,
		Update: resourceAwsRDSClusterEndpointUpdate,
		Delete: resourceAwsRDSClusterEndpointDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"cluster_endpoint_identifier": {
				Type:         schema.TypeString,
				ForceNew:     true,
				Required:     true,
				ValidateFunc: validIdentifier,
			},
			"cluster_identifier": {
				Type:         schema.TypeString,
				ForceNew:     true,
				Required:     true,
				ValidateFunc: validIdentifier,
			},
			"custom_endpoint_type": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					"READER",
					"ANY",
				}, false),
			},
			"excluded_members": {
				Type:          schema.TypeSet,
				Optional:      true,
				ConflictsWith: []string{"static_members"},
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
			},
			"static_members": {
				Type:          schema.TypeSet,
				Optional:      true,
				ConflictsWith: []string{"excluded_members"},
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
			},
			"endpoint": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tags":     tags.TagsSchema(),
			"tags_all": tags.TagsSchemaComputed(),
		},

		CustomizeDiff: tags.SetTagsDiff,
	}
}

func resourceAwsRDSClusterEndpointCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).RDSConn
	defaultTagsConfig := meta.(*client.AWSClient).DefaultTagsConfig
	tags := defaultTagsConfig.MergeTags(keyvaluetags.New(d.Get("tags").(map[string]interface{})))

	clusterId := d.Get("cluster_identifier").(string)
	endpointId := d.Get("cluster_endpoint_identifier").(string)
	endpointType := d.Get("custom_endpoint_type").(string)

	createClusterEndpointInput := &rds.CreateDBClusterEndpointInput{
		DBClusterIdentifier:         aws.String(clusterId),
		DBClusterEndpointIdentifier: aws.String(endpointId),
		EndpointType:                aws.String(endpointType),
		Tags:                        tags.IgnoreAws().RdsTags(),
	}

	if v := d.Get("static_members"); v != nil {
		createClusterEndpointInput.StaticMembers = flex.ExpandStringSet(v.(*schema.Set))
	}
	if v := d.Get("excluded_members"); v != nil {
		createClusterEndpointInput.ExcludedMembers = flex.ExpandStringSet(v.(*schema.Set))
	}

	_, err := conn.CreateDBClusterEndpoint(createClusterEndpointInput)
	if err != nil {
		return fmt.Errorf("Error creating RDS Cluster Endpoint: %s", err)
	}

	d.SetId(endpointId)

	err = resourceAwsRDSClusterEndpointWaitForAvailable(AWSRDSClusterEndpointCreateTimeout, d.Id(), conn)
	if err != nil {
		return err
	}

	return resourceAwsRDSClusterEndpointRead(d, meta)
}

func resourceAwsRDSClusterEndpointRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).RDSConn
	defaultTagsConfig := meta.(*client.AWSClient).DefaultTagsConfig
	ignoreTagsConfig := meta.(*client.AWSClient).IgnoreTagsConfig

	input := &rds.DescribeDBClusterEndpointsInput{
		DBClusterEndpointIdentifier: aws.String(d.Id()),
	}
	log.Printf("[DEBUG] Describing RDS Cluster: %s", input)
	resp, err := conn.DescribeDBClusterEndpoints(input)

	if err != nil {
		return fmt.Errorf("error describing RDS Cluster Endpoints (%s): %s", d.Id(), err)
	}

	if resp == nil {
		return fmt.Errorf("Error retrieving RDS Cluster Endpoints: empty response for: %s", input)
	}

	var clusterEp *rds.DBClusterEndpoint
	for _, e := range resp.DBClusterEndpoints {
		if aws.StringValue(e.DBClusterEndpointIdentifier) == d.Id() {
			clusterEp = e
			break
		}
	}

	if clusterEp == nil {
		log.Printf("[WARN] RDS Cluster Endpoint (%s) not found", d.Id())
		d.SetId("")
		return nil
	}

	arn := clusterEp.DBClusterEndpointArn
	d.Set("cluster_endpoint_identifier", clusterEp.DBClusterEndpointIdentifier)
	d.Set("cluster_identifier", clusterEp.DBClusterIdentifier)
	d.Set("arn", arn)
	d.Set("endpoint", clusterEp.Endpoint)
	d.Set("custom_endpoint_type", clusterEp.CustomEndpointType)

	if err := d.Set("excluded_members", flex.FlattenStringList(clusterEp.ExcludedMembers)); err != nil {
		return fmt.Errorf("error setting excluded_members: %s", err)
	}

	if err := d.Set("static_members", flex.FlattenStringList(clusterEp.StaticMembers)); err != nil {
		return fmt.Errorf("error setting static_members: %s", err)
	}

	tags, err := keyvaluetags.RdsListTags(conn, *arn)

	if err != nil {
		return fmt.Errorf("error listing tags for RDS Cluster Endpoint (%s): %s", *arn, err)
	}

	tags = tags.IgnoreAws().IgnoreConfig(ignoreTagsConfig)

	//lintignore:AWSR002
	if err := d.Set("tags", tags.RemoveDefaultConfig(defaultTagsConfig).Map()); err != nil {
		return fmt.Errorf("error setting tags: %w", err)
	}

	if err := d.Set("tags_all", tags.Map()); err != nil {
		return fmt.Errorf("error setting tags_all: %w", err)
	}

	return nil
}

func resourceAwsRDSClusterEndpointUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).RDSConn
	input := &rds.ModifyDBClusterEndpointInput{
		DBClusterEndpointIdentifier: aws.String(d.Id()),
	}

	if d.HasChange("tags_all") {
		o, n := d.GetChange("tags_all")

		if err := keyvaluetags.RdsUpdateTags(conn, d.Get("arn").(string), o, n); err != nil {
			return fmt.Errorf("error updating RDS Cluster Endpoint (%s) tags: %s", d.Get("arn").(string), err)
		}
	}

	if v, ok := d.GetOk("custom_endpoint_type"); ok {
		input.EndpointType = aws.String(v.(string))
	}

	if attr := d.Get("excluded_members").(*schema.Set); attr.Len() > 0 {
		input.ExcludedMembers = flex.ExpandStringSet(attr)
	} else {
		input.ExcludedMembers = make([]*string, 0)
	}

	if attr := d.Get("static_members").(*schema.Set); attr.Len() > 0 {
		input.StaticMembers = flex.ExpandStringSet(attr)
	} else {
		input.StaticMembers = make([]*string, 0)
	}

	_, err := conn.ModifyDBClusterEndpoint(input)
	if err != nil {
		return fmt.Errorf("Error modifying RDS Cluster Endpoint: %s", err)
	}

	return resourceAwsRDSClusterEndpointRead(d, meta)
}

func resourceAwsRDSClusterEndpointDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).RDSConn
	input := &rds.DeleteDBClusterEndpointInput{
		DBClusterEndpointIdentifier: aws.String(d.Id()),
	}
	_, err := conn.DeleteDBClusterEndpoint(input)
	if err != nil {
		return fmt.Errorf("Error deleting RDS Cluster Endpoint: %s", err)
	}

	if err := resourceAwsRDSClusterEndpointWaitForDestroy(d.Timeout(schema.TimeoutDelete), d.Id(), conn); err != nil {
		return err
	}

	return nil
}

func resourceAwsRDSClusterEndpointWaitForDestroy(timeout time.Duration, id string, conn *rds.RDS) error {
	log.Printf("Waiting for RDS Cluster Endpoint %s to be deleted...", id)
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"available", "deleting"},
		Target:     []string{"destroyed"},
		Refresh:    DBClusterEndpointStateRefreshFunc(conn, id),
		Timeout:    timeout,
		Delay:      AWSRDSClusterEndpointRetryDelay,
		MinTimeout: AWSRDSClusterEndpointRetryMinTimeout,
	}
	_, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for RDS Cluster Endpoint (%s) to be deleted: %v", id, err)
	}
	return nil
}

func resourceAwsRDSClusterEndpointWaitForAvailable(timeout time.Duration, id string, conn *rds.RDS) error {
	log.Printf("Waiting for RDS Cluster Endpoint %s to become available...", id)

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"creating"},
		Target:     []string{"available"},
		Refresh:    DBClusterEndpointStateRefreshFunc(conn, id),
		Timeout:    timeout,
		Delay:      AWSRDSClusterEndpointRetryDelay,
		MinTimeout: AWSRDSClusterEndpointRetryMinTimeout,
	}

	_, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for RDS Cluster Endpoint (%s) to be ready: %v", id, err)
	}
	return nil
}

func DBClusterEndpointStateRefreshFunc(conn *rds.RDS, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		emptyResp := &rds.DescribeDBClusterEndpointsOutput{}

		resp, err := conn.DescribeDBClusterEndpoints(
			&rds.DescribeDBClusterEndpointsInput{
				DBClusterEndpointIdentifier: aws.String(id),
			})
		if err != nil {
			if tfawserr.ErrMessageContains(err, rds.ErrCodeDBClusterNotFoundFault, "") {
				return emptyResp, "destroyed", nil
			} else if resp != nil && len(resp.DBClusterEndpoints) == 0 {
				return emptyResp, "destroyed", nil
			} else {
				return emptyResp, "", fmt.Errorf("Error on refresh: %+v", err)
			}
		}

		if resp == nil || resp.DBClusterEndpoints == nil || len(resp.DBClusterEndpoints) == 0 {
			return emptyResp, "destroyed", nil
		}

		return resp.DBClusterEndpoints[0], *resp.DBClusterEndpoints[0].Status, nil
	}
}