package ec2

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/model"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"math/rand"
	"os"
	"os/user"
	"time"
)

const (
	OnDemandProviderName = "ec2"
	SpotProviderName     = "ec2-spot"
	NameTimeFormat       = "20060102150405"
)

type MountInfo struct {
	VirtualName string `mapstructure:"virtualname" bson:"virtualname,omitempty"`
	DeviceName  string `mapstructure:"devicename" bson:"devicename"`
	Size        int    `mapstructure:"size" bson:"size,omitempty"`
}

//Utility func to create a create a temporary instance name for a host
func generateName(distroName string) string {
	return "mci_" + distroName + "_" + time.Now().Format(NameTimeFormat) +
		fmt.Sprintf("_%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

//makeBlockDeviceMapping takes the mountinfo settings defined in the distro,
//and converts them to ec2.BlockDeviceMapping structs which are usable by goamz.
//It returns a non-nil error if any of the fields appear invalid.
func makeBlockDeviceMappings(mounts []MountInfo) ([]ec2.BlockDeviceMapping, error) {
	mappings := []ec2.BlockDeviceMapping{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, fmt.Errorf("Missing devicename")
		}
		if mount.VirtualName == "" {
			if mount.Size <= 0 {
				return nil, fmt.Errorf("Missing 'size' for EBS mount info in distro")
			}
			//EBS Storage - device name but no virtual name
			mappings = append(mappings, ec2.BlockDeviceMapping{
				DeviceName:          mount.DeviceName,
				VolumeSize:          int64(mount.Size),
				DeleteOnTermination: true,
			})
		} else {
			//Instance Storage - virtual name but no size
			mappings = append(mappings, ec2.BlockDeviceMapping{
				DeviceName:  mount.DeviceName,
				VirtualName: mount.VirtualName,
			})
		}
	}
	return mappings, nil
}

//helper function for getting an EC2 handle at US east
func getUSEast(creds aws.Auth) *ec2.EC2 {
	return ec2.New(creds, aws.USEast)
}

func getEC2KeyOptions(keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, fmt.Errorf("No key specified for EC2 host")
	}
	return []string{
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
		"-i", keyPath,
	}, nil
}

//getInstanceInfo returns the full ec2 instance info for the given instance ID.
//Note that this is the *instance* id, not the spot request ID, which is different.
func getInstanceInfo(ec2Handle *ec2.EC2, instanceId string) (*ec2.Instance, error) {
	amis := []string{instanceId}
	resp, err := ec2Handle.Instances(amis, nil)

	if err != nil {
		return nil, err
	}

	reservation := resp.Reservations
	if len(reservation) < 1 {
		return nil, mci.Logger.Errorf(slogger.ERROR, "No reservation found for "+
			"instance id: %v", instanceId)
	}

	instances := reservation[0].Instances
	if len(instances) < 1 {
		return nil, mci.Logger.Errorf(slogger.ERROR, "'%v' was not found in "+
			"reservation '%v'", instanceId, resp.Reservations[0].ReservationId)
	}
	return &instances[0], nil
}

//ec2StatusToMCIStatus returns a "universal" status code based on EC2's
//provider-specific status codes.
func ec2StatusToMCIStatus(ec2Status string) cloud.CloudStatus {
	switch ec2Status {
	case EC2StatusPending:
		return cloud.StatusInitializing
	case EC2StatusRunning:
		return cloud.StatusRunning
	case EC2StatusShuttingdown:
		return cloud.StatusTerminated
	case EC2StatusTerminated:
		return cloud.StatusTerminated
	case EC2StatusStopped:
		return cloud.StatusStopped
	default:
		return cloud.StatusUnknown
	}
}

//makeTags populates a map of tags based on a host object, which contain keys
//for the user, owner, hostname, and if it's a spawnhost or not.
func makeTags(intentHost *model.Host) map[string]string {
	// get requester host name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// get requester user name
	var username string
	user, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = user.Name
	}

	tags := map[string]string{
		"Name":       intentHost.Id,
		"distro":     intentHost.Distro,
		"hostname":   hostname,
		"username":   username,
		"owner":      intentHost.StartedBy,
		"mode":       "production",
		"start-time": intentHost.CreationTime.Format(NameTimeFormat),
	}

	if intentHost.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}

//attachTags makes a call to EC2 to attach the given map of tags to a resource.
func attachTags(ec2Handle *ec2.EC2,
	tags map[string]string, instance string) error {

	tagSlice := []ec2.Tag{}
	for tag, value := range tags {
		tagSlice = append(tagSlice, ec2.Tag{tag, value})
	}

	_, err := ec2Handle.CreateTags([]string{instance}, tagSlice)
	return err
}
