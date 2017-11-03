// +build go1.7

package gce

import (
	"encoding/json"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const (
	// MinUptime is the minimum time to run a host before shutting it down.
	MinUptime = 30 * time.Minute
	// BufferTime is the time to leave an idle host running after it has completed
	// a task before shutting it down, given that it has already run for MinUptime.
	BufferTime = 10 * time.Minute

	// pricesJSON is the JSON data using by Google Cloud Platform's pricing
	// calculator. There is no actual Google Compute pricing API.
	pricesJSON = "https://cloudpricingcalculator.appspot.com/static/data/pricelist.json"
)

// Regular expressions used to parse pricesJSON, machine paths, and other
// strings unique to Google Compute Engine.
var (
	computeVMCoreRegex  = regexp.MustCompile(`CP-COMPUTEENGINE-CUSTOM-VM-CORE`)
	computeVMRAMRegex   = regexp.MustCompile(`CP-COMPUTEENGINE-CUSTOM-VM-RAM`)
	computeVMImageRegex = regexp.MustCompile(`CP-COMPUTEENGINE-VMIMAGE-(.+)`)
	customMachineRegex  = regexp.MustCompile(`zones/(.+)/machineTypes/custom-(\d+)-(\d+)`)
	normalMachineRegex  = regexp.MustCompile(`zones/(.+)/machineTypes/(.+)`)
	zoneToRegionRegex   = regexp.MustCompile(`(\w+-\w+)-\w+`)
)

// A custom machine type has NumCPUs and MemoryMB defined. A non-custom
// machine type only has its Name defined.
type machineType struct {
	Region   string
	Custom   bool
	Name     string
	NumCPUs  int
	MemoryMB int
}

type computePrices struct {
	// StandardMachine maps a standard machine type >> price per hour.
	StandardMachine map[machineType]float64
	// CustomCPUs maps a region >> price per CPU per hour.
	CustomCPUs map[string]float64
	// CustomMemory maps a region >> price per MB memory per hour.
	CustomMemory map[string]float64
}

// TimeTilNextPayment returns the time until when the host should be destroyed.
//
// In general, TimeTilNextPayment aims to run hosts for a minimum of MinUptime, but
// leaves a buffer time of at least BufferTime after a task has completed before
// shutting a host down. We assume the host is currently free (not running a task).
func (m *Manager) TimeTilNextPayment(h *host.Host) time.Duration {
	now := time.Now()

	// potential end times given certain milestones in the host lifespan
	endMinUptime := h.CreationTime.Add(MinUptime)
	endBufferTime := h.LastTaskCompletedTime.Add(BufferTime)

	// durations until the potential end times
	tilEndMinUptime := endMinUptime.Sub(now)
	tilEndBufferTime := endBufferTime.Sub(now)

	// check that last task completed time is not zero
	if util.IsZeroTime(h.LastTaskCompletedTime) {
		return tilEndMinUptime
	}

	// return the greater of the two durations
	if tilEndBufferTime.Minutes() < tilEndMinUptime.Minutes() {
		return tilEndMinUptime
	}
	return tilEndBufferTime
}

// CostForDuration estimates the cost for a span of time on the given host.
// The method assumes the duration is in the range up to a day, as certain
// discounts apply when used for sustained periods of time.
//
// This is a rough estimate considering number of CPUs and amount of memory,
// as there may be other factors to consider such as GPU usage, premium CPU
// platforms (e.g. Skylake VMs), premium images (e.g. SUSE), networks, etc.
//
// Source: https://cloud.google.com/compute/pricing
func (m *Manager) CostForDuration(h *host.Host, start, end time.Time) (float64, error) {
	// Sanity check.
	if end.Before(start) || util.IsZeroTime(start) || util.IsZeroTime(end) {
		return 0, errors.New("task timing data is malformed")
	}

	// Grab and parse instance details.
	instance, err := m.client.GetInstance(h)
	if err != nil {
		return 0, err
	}

	duration := end.Sub(start)
	machine, err := parseMachineType(instance.MachineType)
	if err != nil {
		return 0, errors.Wrap(err, "error parsing machine")
	}

	// Calculate the instance cost.
	cost, err := instanceCost(machine, duration)
	if err != nil {
		return 0, errors.Wrap(err, "error calculating costs")
	}

	return cost, nil
}

// zoneToRegion returns the Google Compute region given a zone.
// For example, given a zone like us-east1-c, it returns us-east1.
func zoneToRegion(zone string) (string, error) {
	if m := zoneToRegionRegex.FindStringSubmatch(zone); len(m) > 0 {
		return m[1], nil
	}

	return "", errors.Errorf("error parsing zone %s", zone)
}

// parseMachineType returns a machine type struct given the path to a
// Google Compute machine type. Predefined machine types have the format
//
//     zones/<zone>/machineTypes/<name>
//
// while custom machine types have the format
//
//     zones/<zone>/machineTypes/custom-<numCPUs>-<memoryMB>
func parseMachineType(m string) (*machineType, error) {
	custom := customMachineRegex.FindStringSubmatch(m)
	if len(custom) > 0 {
		region, err := zoneToRegion(custom[1])
		if err != nil {
			return nil, errors.Wrap(err, "error parsing zone")
		}

		numCPUs, err := strconv.Atoi(custom[2])
		if err != nil {
			return nil, errors.Errorf("not an int: %s", custom[1])
		}

		memoryMB, err := strconv.Atoi(custom[3])
		if err != nil {
			return nil, errors.Errorf("not an int: %s", custom[2])
		}

		return &machineType{
			Region:   region,
			Custom:   true,
			NumCPUs:  numCPUs,
			MemoryMB: memoryMB,
		}, nil
	}

	normal := normalMachineRegex.FindStringSubmatch(m)
	if len(normal) > 0 {
		region, err := zoneToRegion(normal[1])
		if err != nil {
			return nil, errors.Wrap(err, "error parsing zone")
		}

		return &machineType{
			Region: region,
			Custom: false,
			Name:   normal[2],
		}, nil
	}

	return nil, errors.Errorf("error parsing machine type %s", m)
}

// getComputePrices parses Google Compute pricing information from the
// endpoint used by the Google Cloud pricing calculator, and returns the
// data as a map of machine type >> region >> price per hour.
//
// If this function errors, that means Google may have changed the way
// it structures its pricing data in the JSON file.
func getComputePrices() (*computePrices, error) {
	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	// Read the data from the endpoint.
	res, err := client.Get(pricesJSON)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "fetching %s", pricesJSON)
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	var obj map[string]interface{}
	if err = json.Unmarshal(data, &obj); err != nil {
		return nil, errors.Wrap(err, "parsing price JSON")
	}

	// Parse the data into a compute prices struct.
	prices := &computePrices{
		StandardMachine: make(map[machineType]float64),
		CustomCPUs:      make(map[string]float64),
		CustomMemory:    make(map[string]float64),
	}

	allGCPPrices, ok := obj["gcp_price_list"].(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("error casting %v", obj["gcp_price_list"])
	}

	for machine, regionToPrice := range allGCPPrices {
		vmImage := computeVMImageRegex.FindStringSubmatch(machine)

		matchVMImage := len(vmImage) > 0
		matchVMCore := computeVMCoreRegex.MatchString(machine)
		matchVMRAM := computeVMRAMRegex.MatchString(machine)

		// If there are no matches, it was not a relevant field.
		if !(matchVMImage || matchVMCore || matchVMRAM) {
			continue
		}

		regionToPrice, ok := regionToPrice.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("error casting %v", regionToPrice)
		}

		// Get float values for the region only (prices).
		for r, p := range regionToPrice {
			if f, ok := p.(float64); ok {
				if matchVMImage {
					machineName := strings.ToLower(vmImage[1])
					machine := machineType{
						Region: r,
						Name:   machineName,
					}
					prices.StandardMachine[machine] = f
				} else if matchVMCore {
					prices.CustomCPUs[r] = f
				} else if matchVMRAM {
					prices.CustomMemory[r] = f
				}
			}
		}
	}

	return prices, nil
}

// instanceCost returns the estimated cost for a machine of type
// machine in the given region for the given duration.
func instanceCost(machine *machineType, dur time.Duration) (float64, error) {
	prices, err := getComputePrices()
	if err != nil {
		return 0, errors.Wrap(err, "error parsing prices")
	}

	// Predefined machine type
	if !machine.Custom {
		unitPrice := prices.StandardMachine[*machine]
		return unitPrice * dur.Hours(), nil
	}

	// Machine type with custom CPUs and RAM
	cpuPrice := prices.CustomCPUs[machine.Region] * float64(machine.NumCPUs)
	memPrice := prices.CustomMemory[machine.Region] * float64(machine.MemoryMB)
	return (cpuPrice + memPrice) * dur.Hours(), nil
}
