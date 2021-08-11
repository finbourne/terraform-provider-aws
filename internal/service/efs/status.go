package efs

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/terraform-providers/terraform-provider-aws/internal/tfresource"
)

// statusAccessPointLifeCycleState fetches the Access Point and its LifecycleState
func statusAccessPointLifeCycleState(conn *efs.EFS, accessPointId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		input := &efs.DescribeAccessPointsInput{
			AccessPointId: aws.String(accessPointId),
		}

		output, err := conn.DescribeAccessPoints(input)

		if err != nil {
			return nil, "", err
		}

		if output == nil || len(output.AccessPoints) == 0 || output.AccessPoints[0] == nil {
			return nil, "", nil
		}

		mt := output.AccessPoints[0]

		return mt, aws.StringValue(mt.LifeCycleState), nil
	}
}

func statusBackupPolicy(conn *efs.EFS, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := findBackupPolicyByID(conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.Status), nil
	}
}

// statusFileSystemLifeCycleState fetches the Access Point and its LifecycleState
func statusFileSystemLifeCycleState(conn *efs.EFS, fileSystemID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		input := &efs.DescribeFileSystemsInput{
			FileSystemId: aws.String(fileSystemID),
		}

		output, err := conn.DescribeFileSystems(input)

		if err != nil {
			return nil, "", err
		}

		if output == nil || len(output.FileSystems) == 0 || output.FileSystems[0] == nil {
			return nil, "", nil
		}

		mt := output.FileSystems[0]

		return mt, aws.StringValue(mt.LifeCycleState), nil
	}
}