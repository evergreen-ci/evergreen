package event

import (
	"fmt"

	"github.com/mongodb/grip/message"
)

// RecentHostAgentDeploys is a type used to capture the results of an agent
// deploy.
type RecentHostAgentDeploys struct {
	HostID            string `bson:"host_id" json:"host_id" yaml:"host_id"`
	Count             int    `bson:"count" json:"count" yaml:"count"`
	Failed            int    `bson:"failed" json:"failed" yaml:"failed"`
	Success           int    `bson:"success" json:"success" yaml:"success"`
	HostStatusChanged int    `bson:"host_status_changed" json:"host_status_changed" yaml:"host_status_changed"`
	Last              string `bson:"last" json:"last" yaml:"last"`
	Total             int    `bson:"total" json:"total" yaml:"total"`
	message.Base      `bson:"metadata" json:"metadata" yaml:"metadata"`
}

////////////////////////////////////////////////////////////////////////
//
// Implementation of methods to satisfy the grip/message.Composer interface

func (m *RecentHostAgentDeploys) Raw() interface{} { return m }
func (m *RecentHostAgentDeploys) Loggable() bool {
	return m.HostID != "" && m.Last != "" && m.Count > 0
}
func (m *RecentHostAgentDeploys) String() string {
	return fmt.Sprintf("host=%s last=%s failed=%d success=%d num=%d",
		m.HostID, m.Last, m.Failed, m.Success, m.Count)
}

////////////////////////////////////////////////////////////////////////
//
// Predicates to support error checking during the agent deploy process
func (m *RecentHostAgentDeploys) LastAttemptFailed() bool {
	return m.Last == EventHostAgentDeployFailed || m.Last == EventHostAgentMonitorDeployFailed
}

func (m *RecentHostAgentDeploys) AllAttemptsFailed() bool {
	return m.Count > 0 && m.Success == 0 && m.HostStatusChanged == 0
}
