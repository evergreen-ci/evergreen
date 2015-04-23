package hostinit

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	"10gen.com/mci/notify"
	"10gen.com/mci/remote"
	"10gen.com/mci/util"
	"errors"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"labix.org/v2/mgo"
	"strings"
	"sync"
	"time"
)

// Error indicating another hostinit got to the setup first
var (
	ErrHostAlreadyInitializing = errors.New("Host already initializing")
)

// HostInit is responsible for running setup scripts on MCI hosts.
type HostInit struct {
	MCISettings *mci.MCISettings
}

// SetupReadyHosts runs the setup script on all hosts that are up and reachable.
func (init *HostInit) SetupReadyHosts() error {

	// find all hosts in the uninitialized state
	uninitializedHosts, err := model.FindUninitializedHosts()
	if err != nil {
		return fmt.Errorf("error fetching uninitialized hosts: %v", err)
	}

	mci.Logger.Logf(slogger.DEBUG, "There are %v uninitialized hosts",
		len(uninitializedHosts))

	// used for making sure we don't exit before a setup script is done
	wg := &sync.WaitGroup{}

	for _, host := range uninitializedHosts {

		// check whether or not the host is ready for its setup script to be run
		ready, err := init.IsHostReady(&host)
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "Error checking host %v for readiness: %v",
				host.Id, err)
			continue
		}

		// if the host isn't ready (for instance, it might not be up yet), skip it
		if !ready {
			mci.Logger.Logf(slogger.DEBUG, "Host %v not ready for setup", host.Id)
			continue
		}

		// build the setup script
		setup, err := init.buildSetupScript(&host)
		if err != nil {
			return fmt.Errorf("error building setup script for host %v: %v", host.Id, err)
		}

		mci.Logger.Logf(slogger.INFO, "Running setup script for host %v", host.Id)

		// kick off the setup, in its own goroutine, so pending setups don't have
		// to wait for it to finish
		wg.Add(1)
		go func(host model.Host, setup string) {

			if err := init.ProvisionHost(&host, setup); err != nil {
				mci.Logger.Logf(slogger.ERROR, "Error provisioning host %v: %v",
					host.Id, err)

				// notify the mci team of the failure
				subject := fmt.Sprintf("%v MCI provisioning failure on %v",
					notify.ProvisionFailurePreface, host.Distro)
				hostLink := fmt.Sprintf("%v/host/%v", init.MCISettings.Ui.Url, host.Id)
				message := fmt.Sprintf("Provisioning failed on %v host -- %v: see %v",
					host.Distro, host.Id, hostLink)
				if err := notify.NotifyAdmins(subject, message, init.MCISettings); err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
				}
			}

			wg.Done()

		}(host, setup)

	}

	// let all setup routines finish
	wg.Wait()

	return nil
}

// IsHostReady returns whether or not the specified host is ready for its setup script
// to be run.
func (init *HostInit) IsHostReady(host *model.Host) (bool, error) {

	// fetch the appropriate cloud provider for the host
	cloudMgr, err := providers.GetCloudManager(host.Provider, init.MCISettings)
	if err != nil {
		return false,
			fmt.Errorf("failed to get cloud manager for provider %v: %v", host.Provider, err)
	}

	// ask for the instance's status
	hostStatus, err := cloudMgr.GetInstanceStatus(host)
	if err != nil {
		return false, fmt.Errorf("error checking instance status of host %v: %v", host.Id, err)
	}

	// if the host isn't up yet, we can't do anything
	if hostStatus != cloud.StatusRunning {
		return false, nil
	}

	// set the host's dns name, if it is not set
	// TODO: this code should be moved out of this function
	if host.Host == "" {

		// get the DNS name for the host
		hostDNS, err := cloudMgr.GetDNSName(host)
		if err != nil {
			return false, fmt.Errorf("error checking DNS name for host %v: %v", host.Id, err)
		}

		// sanity check for the host DNS name
		if hostDNS == "" {
			return false, fmt.Errorf("instance %v is running but not returning a DNS name",
				host.Id)
		}

		// update the host's DNS name
		if err := host.SetDNSName(hostDNS); err != nil {
			return false, fmt.Errorf("error setting DNS name for host %v: %v", host.Id, err)
		}

	}

	// check if the host is reachable via SSH
	cloudHost, err := providers.GetCloudHost(host, init.MCISettings)
	if err != nil {
		return false, fmt.Errorf("failed to get cloud host for %v: %v", host.Id, err)
	}
	reachable, err := cloudHost.IsSSHReachable()
	if err != nil {
		return false, fmt.Errorf("error checking if host %v is reachable: %v", host.Id, err)
	}

	// at this point, we can run the setup if the host is reachable
	return reachable, nil
}

// setupHost runs the specified setup script for an individual host. Returns
// the output from running the script remotely, as well as any error that
// occurs. If the script exits with a non-zero exit code, the error will be non-nil.
func (init *HostInit) setupHost(host *model.Host, setup string) ([]byte, error) {

	// fetch the appropriate cloud provider for the host
	cloudMgr, err := providers.GetCloudManager(host.Provider, init.MCISettings)
	if err != nil {
		return nil,
			fmt.Errorf("failed to get cloud manager for host %v with provider %v: %v",
				host.Id, host.Provider, err)
	}

	// fetch the host's distro
	distro, err := model.LoadOneDistro(init.MCISettings.ConfigDir, host.Distro)
	if err != nil {
		return nil,
			fmt.Errorf("error getting distro %v for host %v: %v",
				host.Distro, host.Id, err)
	}

	// mark the host as initializing
	if err := host.SetInitializing(); err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrHostAlreadyInitializing
		} else {
			return nil, fmt.Errorf("database error: %v", err)
		}
	}

	// run the function scheduled for when the host is up
	err = cloudMgr.OnUp(host)
	if err != nil {
		// if this fails it is probably due to an API hiccup, so we keep going.
		mci.Logger.Logf(slogger.WARN, "OnUp callback failed for host '%v': '%v'", host.Id, err)
	}

	// get the local path to the keyfile, if not specified
	keyfile := init.MCISettings.Keys[distro.Key]

	// run the remote setup script as sudo, if appropriate
	sudoStr := ""
	if distro.SetupAsSudo {
		sudoStr = "sudo "
	}

	// parse the hostname into the user, host and port
	hostInfo, err := util.ParseSSHInfo(host.Host)
	if err != nil {
		return nil, err
	}
	user := distro.User
	if hostInfo.User != "" {
		user = hostInfo.User
	}

	// initialize a gateway for creating the script on the remote machine
	gateway := &remote.SFTPGateway{
		Host:    hostInfo.Hostname + ":" + hostInfo.Port,
		User:    user,
		Keyfile: keyfile,
	}
	if err := gateway.Init(); err != nil {
		return nil, fmt.Errorf("error connecting via sftp: %v", err)
	}
	defer gateway.Close()

	// create the remote file
	remoteFileName := "setup.sh"
	file, err := gateway.Client.Create(remoteFileName)
	if err != nil {
		return nil, fmt.Errorf("error creating remote setup script: %v", err)
	}
	defer file.Close()

	// write the setup script to the file
	if _, err := file.Write([]byte(setup)); err != nil {
		return nil, fmt.Errorf("error writing remote setup script: %v", err)
	}

	// set up remote running of the script
	script := &remote.SSHCommand{
		Command: sudoStr + "sh " + remoteFileName,
		Host:    hostInfo.Hostname + ":" + hostInfo.Port,
		User:    user,
		Keyfile: keyfile,
		Timeout: time.Minute * 30,
	}

	// run the setup script
	return script.Run()

}

// Build the setup script that will need to be run on the specified host.
func (init *HostInit) buildSetupScript(host *model.Host) (string, error) {

	// fetch the host's distro
	distro, err := model.LoadOneDistro(init.MCISettings.ConfigDir, host.Distro)
	if err != nil {
		return "", fmt.Errorf("error getting distro %v for host %v: %v", host.Distro, host.Id, err)
	}

	// include MCI-only setup in production
	setupScript := strings.Join([]string{distro.Setup, distro.SetupMciOnly}, "\n")

	// replace expansions in the script
	exp := command.NewExpansions(init.MCISettings.Expansions)
	setupScript, err = exp.ExpandString(setupScript)
	if err != nil {
		return "", fmt.Errorf("expansions error: %v", err)
	}

	return setupScript, err

}

// Provision the host, and update the database accordingly.
func (init *HostInit) ProvisionHost(host *model.Host, setup string) error {

	// run the setup script
	output, err := init.setupHost(host, setup)

	// deal with any errors that occured while running the setup
	if err != nil {

		mci.Logger.Logf(slogger.ERROR, "Error running setup script: %v", err)

		// another hostinit process beat us there
		if err == ErrHostAlreadyInitializing {
			mci.Logger.Logf(slogger.DEBUG,
				"Attempted to initialize already initializing host %v", host.Id)
			return nil
		}

		// log the provisioning failure
		setupLog := ""
		if output != nil {
			setupLog = string(output)
		}
		model.LogProvisionFailedEvent(host.Id, setupLog)

		// setup script failed, mark the host's provisioning as failed
		if err := host.SetUnprovisioned(); err != nil {
			mci.Logger.Logf(slogger.ERROR, "unprovisioning host %v failed: %v", host.Id, err)
		}

		return fmt.Errorf("error initializing host %v: %v", host.Id, err)

	}

	// the setup was successful. update the host accordingly in the database
	if err := host.MarkAsProvisioned(); err != nil {
		return fmt.Errorf("error marking host %v as provisioned: %v", err)
	}

	mci.Logger.Logf(slogger.INFO, "Host %v successfully provisioned", host.Id)

	return nil

}
