package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const ipAddressAllocatorJobName = "ip-address-allocator"

func init() {
	registry.AddJobType(ipAddressAllocatorJobName, func() amboy.Job {
		return makeIPAddressAllocator()
	})
}

type ipAddressAllocator struct {
	job.Base `bson:"job_base" json:"job_base"`

	env evergreen.Environment
}

func makeIPAddressAllocator() *ipAddressAllocator {
	return &ipAddressAllocator{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    ipAddressAllocatorJobName,
				Version: 0,
			},
		},
	}
}

// NewIPAddressAllocatorJob returns a job to allocate IP addresses.
func NewIPAddressAllocatorJob(env evergreen.Environment, ts string) amboy.Job {
	j := makeIPAddressAllocator()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", ipAddressAllocatorJobName, ts))
	j.SetScopes([]string{ipAddressAllocatorJobName})
	j.SetEnqueueAllScopes(true)
	return j
}

// TODO (DEVPROD-17195): remove this temporary job once all elastic IPs are
// allocated into collection.
func (j *ipAddressAllocator) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	poolID := j.env.Settings().Providers.AWS.IPAMPoolID
	if poolID == "" {
		return
	}

	const maxNumToAllocatePerJob = 500
	const waitBetweenAllocations = 500 * time.Millisecond
	timer := time.NewTimer(waitBetweenAllocations)
	numAllocated := 0
	for {
		select {
		case <-timer.C:
			if err := j.allocateIPAddress(ctx); err != nil {
				if strings.Contains(err.Error(), cloud.EC2InsufficientAddressCapacity) || strings.Contains(err.Error(), cloud.EC2AddressLimitExceeded) {
					// If the IP couldn't be allocated because the IPAM pool has
					// been exhausted, then the job is done.
					grip.Debug(message.Fields{
						"message": "no elastic IPs left to allocate, no-oping the job",
						"ticket":  "DEVPROD-17313",
						"job":     j.ID(),
					})
					return
				}
				j.AddError(errors.Wrap(err, "allocating IP address"))
				return
			}
			numAllocated++
			if numAllocated >= maxNumToAllocatePerJob {
				return
			}
			timer.Reset(waitBetweenAllocations)
		case <-ctx.Done():
			j.AddError(ctx.Err())
			return
		}
	}
}

func (j *ipAddressAllocator) allocateIPAddress(ctx context.Context) error {
	mgrOpts := cloud.ManagerOpts{
		Provider: evergreen.ProviderNameEc2Fleet,
		Region:   evergreen.DefaultEC2Region,
	}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager")
	}
	ipAddr, err := mgr.AllocateIP(ctx)
	if err != nil {
		return errors.Wrap(err, "allocating IP address")
	}
	// Intentionally ignore the context cancellation error here, since inserting
	// the IP address should be a fairly quick operation and the address is
	// already allocated. It's better to just try inserting the
	// already-allocated address rather than trying to exit early for SIGTERM.
	insertCtx := context.WithoutCancel(ctx)
	if err := ipAddr.Insert(insertCtx); err != nil {
		return errors.Wrapf(err, "inserting IP address with allocation ID '%s'", ipAddr.AllocationID)
	}
	return nil
}
