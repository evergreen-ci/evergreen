package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/model"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

const (
	// the threshold to consider as too long for a host to take provisioning
	ProvisioningCutoff = 35 * time.Minute
)

// function that takes in all distros - specified as a map of
// distro name -> distro info - as well as the mci settings,
// and spits out a list of hosts to be terminated
type hostFlaggingFunc func(map[string]model.Distro,
	*mci.MCISettings) ([]model.Host, error)

// flagDecommissionedHosts is a hostFlaggingFunc to get all hosts which should
// be terminated because they are decommissioned
func flagDecommissionedHosts(distros map[string]model.Distro,
	mciSettings *mci.MCISettings) ([]model.Host,
	error) {

	mci.Logger.Logf(slogger.INFO, "Finding decommissioned hosts...")

	// fetch the decommissioned hosts
	hosts, err := model.FindDecommissionedHosts()
	if err != nil {
		return nil, fmt.Errorf("error finding decommissioned hosts: %v", err)
	}

	mci.Logger.Logf(slogger.INFO, "Found %v decommissioned hosts", len(hosts))

	return hosts, nil
}

// flagIdleHosts is a hostFlaggingFunc to get all hosts which have spent too
// long without running a task
func flagIdleHosts(distros map[string]model.Distro,
	mciSettings *mci.MCISettings) ([]model.Host,
	error) {

	mci.Logger.Logf(slogger.INFO, "Finding idle hosts...")

	// will ultimately contain all of the hosts determined to be idle
	idleHosts := []model.Host{}

	// fetch all hosts not currently running a task
	freeHosts, err := model.FindFreeHosts()
	if err != nil {
		return nil, fmt.Errorf("error finding free hosts: %v", err)
	}

	// go through the hosts, and see if they have idled long enough to
	// be terminated
	for _, host := range freeHosts {

		// ask the host how long it has been idle
		idleTime := host.IdleTime()

		// get a cloud manager for the host
		cloudManager, err := providers.GetCloudManager(host.Provider,
			mciSettings)
		if err != nil {
			return nil, fmt.Errorf("error getting cloud manager for host %v:"+
				" %v", host.Id, err)
		}

		// if the host is not dynamically spun up (and can thus be terminated),
		// skip it
		canTerminate, err := hostCanBeTerminated(host, mciSettings)
		if err != nil {
			return nil, fmt.Errorf("error checking if host %v can be"+
				" terminated: %v", host.Id, err)
		}
		if !canTerminate {
			continue
		}

		// ask how long until the next payment for the host
		tilNextPayment := cloudManager.TimeTilNextPayment(&host)

		// current determinants for idle:
		//  idle for at least 15 minutes and
		//  less than 5 minutes til next payment
		if idleTime >= 15*time.Minute && tilNextPayment <= 5*time.Minute {
			idleHosts = append(idleHosts, host)
		}

	}

	mci.Logger.Logf(slogger.INFO, "Found %v idle hosts", len(idleHosts))

	return idleHosts, nil
}

// flagExcessHosts is a hostFlaggingFunc to get all hosts that push their
// distros over the specified max hosts
func flagExcessHosts(distros map[string]model.Distro,
	mciSettings *mci.MCISettings) ([]model.Host,
	error) {

	mci.Logger.Logf(slogger.INFO, "Finding excess hosts...")

	// will ultimately contain all the hosts that can be terminated
	excessHosts := []model.Host{}

	// figure out the excess hosts for each distro
	for distroName, distroInfo := range distros {

		// fetch any hosts for the distro that count towards max hosts
		allHostsForDistro, err := model.FindHostsForDistro(distroName)
		if err != nil {
			return nil, fmt.Errorf("error fetching hosts for distro %v: %v",
				distroName, err)
		}

		// if there are more than the specified max hosts, then terminate
		// some, if they are not running tasks
		numExcessHosts := len(allHostsForDistro) - distroInfo.MaxHosts
		if numExcessHosts > 0 {

			// track how many hosts for the distro are terminated
			counter := 0

			for _, host := range allHostsForDistro {

				// if the host is not dynamically spun up (and can
				// thus be terminated), skip it
				canTerminate, err := hostCanBeTerminated(host, mciSettings)
				if err != nil {
					return nil, fmt.Errorf("error checking if host %v can be"+
						" terminated: %v", host.Id, err)
				}
				if !canTerminate {
					continue
				}

				// if the host is not running a task, it can be
				// safely terminated
				if host.RunningTask == "" {
					excessHosts = append(excessHosts, host)
					counter++
				}

				// break if we've marked enough to be terminated
				if counter == numExcessHosts {
					break
				}
			}

			mci.Logger.Logf(slogger.INFO, "Flagged %v excess hosts for distro"+
				" %v", counter, distroName)

		}

	}

	mci.Logger.Logf(slogger.INFO, "Found %v total excess hosts",
		len(excessHosts))

	return excessHosts, nil
}

// flagUnprovisionedHosts is a hostFlaggingFunc to get all hosts that are
// taking too long to provision
func flagUnprovisionedHosts(distros map[string]model.Distro,
	mciSettings *mci.MCISettings) ([]model.Host,
	error) {

	mci.Logger.Logf(slogger.INFO, "Finding unprovisioned hosts...")

	// fetch all hosts that are taking too long to provision
	threshold := time.Now().Add(-ProvisioningCutoff)
	hosts, err := model.FindUnprovisionedHosts(threshold)
	if err != nil {
		return nil, fmt.Errorf("error finding unprovisioned hosts: %v", err)
	}

	mci.Logger.Logf(slogger.INFO, "Found %v unprovisioned hosts", len(hosts))

	return hosts, err
}

// flagProvisioningFailedHosts is a hostFlaggingFunc to get all hosts
// whose provisioning failed
func flagProvisioningFailedHosts(distros map[string]model.Distro,
	mciSettings *mci.MCISettings) ([]model.Host,
	error) {

	mci.Logger.Logf(slogger.INFO, "Finding hosts whose provisioning failed...")

	// fetch all hosts whose provisioning failed
	hosts, err := model.FindProvisioningFailedHosts()
	if err != nil {
		return nil, fmt.Errorf("error finding hosts whose provisioning"+
			" failed: %v", err)
	}

	mci.Logger.Logf(slogger.INFO, "Found %v hosts whose provisioning failed",
		len(hosts))

	return hosts, nil

}

// flagExpiredHosts is a hostFlaggingFunc to get all user-spawned hosts
// that have expired
func flagExpiredHosts(distros map[string]model.Distro,
	mciSettings *mci.MCISettings) ([]model.Host,
	error) {

	mci.Logger.Logf(slogger.INFO, "Finding expired hosts")

	// fetch the expired hosts
	hosts, err := model.FindExpiredSpawnedHosts()
	if err != nil {
		return nil, fmt.Errorf("error finding expired spawned hosts: %v", err)
	}

	mci.Logger.Logf(slogger.INFO, "Found %v expired hosts", len(hosts))

	return hosts, nil

}

// helper to check if a host can be terminated
func hostCanBeTerminated(host model.Host, mciSettings *mci.MCISettings) (bool,
	error) {

	// get a cloud manager for the host
	cloudManager, err := providers.GetCloudManager(host.Provider,
		mciSettings)
	if err != nil {
		return false, fmt.Errorf("error getting cloud manager for host %v:"+
			" %v", host.Id, err)
	}

	// if the host is not part of a spawnable distro, then it was not
	// dynamically spun up and as such cannot be terminated
	canSpawn, err := cloudManager.CanSpawn()
	if err != nil {
		return false, fmt.Errorf("error checking if cloud manager for host %v"+
			" supports spawning: %v", host.Id, err)
	}

	return canSpawn, nil

}
