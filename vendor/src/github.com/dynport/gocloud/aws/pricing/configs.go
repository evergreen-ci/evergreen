package pricing

import "strings"

type InstanceTypeConfig struct {
	Name               string
	Cpus               int
	Memory             float64
	Storage            string
	NetworkPerformance string
	ClockSpeed         float64

	Turbo              bool
	AVX                bool
	AES                bool
	PhysicalProcessor  string
	EbsOptimizable     bool
	EnhancedNetworking bool
}

func (c *InstanceTypeConfig) Family() string {
	return strings.Split(c.Name, ".")[0]
}
