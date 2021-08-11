package emr

import (
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/emr"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/terraform-providers/terraform-provider-aws/internal/client"
)

func ResourceSecurityConfiguration() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsEmrSecurityConfigurationCreate,
		Read:   resourceAwsEmrSecurityConfigurationRead,
		Delete: resourceAwsEmrSecurityConfigurationDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:          schema.TypeString,
				Optional:      true,
				Computed:      true,
				ForceNew:      true,
				ConflictsWith: []string{"name_prefix"},
				ValidateFunc:  validation.StringLenBetween(0, 10280),
			},
			"name_prefix": {
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"name"},
				ValidateFunc:  validation.StringLenBetween(0, 10280-resource.UniqueIDSuffixLength),
			},

			"configuration": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringIsJSON,
			},

			"creation_date": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAwsEmrSecurityConfigurationCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).EMRConn

	var emrSCName string
	if v, ok := d.GetOk("name"); ok {
		emrSCName = v.(string)
	} else {
		if v, ok := d.GetOk("name_prefix"); ok {
			emrSCName = resource.PrefixedUniqueId(v.(string))
		} else {
			emrSCName = resource.PrefixedUniqueId("tf-emr-sc-")
		}
	}

	resp, err := conn.CreateSecurityConfiguration(&emr.CreateSecurityConfigurationInput{
		Name:                  aws.String(emrSCName),
		SecurityConfiguration: aws.String(d.Get("configuration").(string)),
	})

	if err != nil {
		return err
	}

	d.SetId(aws.StringValue(resp.Name))
	return resourceAwsEmrSecurityConfigurationRead(d, meta)
}

func resourceAwsEmrSecurityConfigurationRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).EMRConn

	resp, err := conn.DescribeSecurityConfiguration(&emr.DescribeSecurityConfigurationInput{
		Name: aws.String(d.Id()),
	})
	if err != nil {
		if tfawserr.ErrMessageContains(err, "InvalidRequestException", "does not exist") {
			log.Printf("[WARN] EMR Security Configuration (%s) not found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return err
	}

	d.Set("creation_date", aws.TimeValue(resp.CreationDateTime).Format(time.RFC3339))
	d.Set("name", resp.Name)
	d.Set("configuration", resp.SecurityConfiguration)

	return nil
}

func resourceAwsEmrSecurityConfigurationDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*client.AWSClient).EMRConn

	_, err := conn.DeleteSecurityConfiguration(&emr.DeleteSecurityConfigurationInput{
		Name: aws.String(d.Id()),
	})
	if err != nil {
		if tfawserr.ErrMessageContains(err, "InvalidRequestException", "does not exist") {
			return nil
		}
		return err
	}

	return nil
}