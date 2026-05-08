package ec2instancereferenceprice

import (
	"context"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const criticalRefPricingMsg = "EC2 reference pricing could not be applied, on_demand_rate and savings_plan_rate will be set to zero. To avoid inaccurate cost calculations, add a document to ec2_instance_reference_prices."

const (
	refPricingOSLinux   = "linux"
	refPricingOSWindows = "windows"
	refPricingOSSUSE    = "suse"
	refPricingOSMacOS   = "macos"
)

// OperatingSystemFromImageID maps distro image_id naming to lowercase OS keys for
// reference pricing. Substring checks are case-insensitive.
func OperatingSystemFromImageID(imageID string) string {
	lower := strings.ToLower(imageID)
	switch {
	case strings.Contains(lower, refPricingOSSUSE):
		return refPricingOSSUSE
	case strings.Contains(lower, refPricingOSWindows):
		return refPricingOSWindows
	case strings.Contains(lower, refPricingOSMacOS):
		return refPricingOSMacOS
	default:
		return refPricingOSLinux
	}
}

func ec2InstanceTypeFromDistro(d *distro.Distro) string {
	var ec2Settings cloud.EC2ProviderSettings
	err := ec2Settings.FromDistroSettings(*d, evergreen.DefaultEC2Region)
	if err != nil {
		err = ec2Settings.FromDistroSettings(*d, "")
	}
	if err != nil {
		return ""
	}
	return ec2Settings.InstanceType
}

// ApplyReferenceCostDataOrWarn fills CostData from the EC2 reference price row for this distro's
// instance type and OS, or zeros rates and logs critically if none are found.
func ApplyReferenceCostDataOrWarn(ctx context.Context, d *distro.Distro) {
	if d == nil || !evergreen.IsEc2Provider(d.Provider) {
		return
	}
	instanceType := ec2InstanceTypeFromDistro(d)
	if instanceType == "" {
		grip.Critical(ctx, message.Fields{
			"message":                criticalRefPricingMsg,
			"reason":                 "cannot_resolve_ec2_instance_type",
			"distro_id":              d.Id,
			"image_id":               d.ImageID,
			"cost_on_demand_rate":    d.CostData.OnDemandRate,
			"cost_savings_plan_rate": d.CostData.SavingsPlanRate,
		})
		d.CostData.OnDemandRate = 0
		d.CostData.SavingsPlanRate = 0
		return
	}
	osName := OperatingSystemFromImageID(d.ImageID)
	ref, err := FindOne(ctx, ByInstanceTypeAndOS(instanceType, osName))
	if err != nil {
		grip.Error(ctx, errors.Wrap(err, "loading EC2 reference price"))
		return
	}
	if ref == nil {
		grip.Critical(ctx, message.Fields{
			"message":                  criticalRefPricingMsg,
			"reason":                   "no_ec2_reference_price_row",
			"distro_id":                d.Id,
			"image_id":                 d.ImageID,
			"derived_operating_system": osName,
			"instance_type":            instanceType,
			"cost_on_demand_rate":      d.CostData.OnDemandRate,
			"cost_savings_plan_rate":   d.CostData.SavingsPlanRate,
		})
		d.CostData.OnDemandRate = 0
		d.CostData.SavingsPlanRate = 0
		return
	}
	d.CostData.OnDemandRate = ref.OnDemandMedian
	d.CostData.SavingsPlanRate = ref.AdjustedFinanceMedian
}
