package evergreen

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/grip"
)

var registry *configSectionRegistry

func init() {
	// add any new config sections to the variable below to register them
	configSections := []configSection{
		&AlertsConfig{},
		&AmboyConfig{},
		&APIConfig{},
		&AuthConfig{},
		&CloudProviders{},
		&HostInitConfig{},
		&JiraConfig{},
		&LoggerConfig{},
		&NewRelicConfig{},
		&NotifyConfig{},
		&RepoTrackerConfig{},
		&SchedulerConfig{},
		&ServiceFlags{},
		&SlackConfig{},
		&UIConfig{},
	}

	registry = newConfigSectionRegistry()
	catcher := grip.NewSimpleCatcher()

	for _, section := range configSections {
		catcher.Add(registry.registerSection(section.id(), section))
	}
	if catcher.HasErrors() {
		panic(fmt.Sprintf("error registering config sections: %s", catcher.String()))
	}
}

type configSectionRegistry struct {
	mux      *sync.RWMutex
	sections map[string]configSection
}

func newConfigSectionRegistry() *configSectionRegistry {
	return &configSectionRegistry{
		mux:      &sync.RWMutex{},
		sections: map[string]configSection{},
	}
}

func (r *configSectionRegistry) registerSection(id string, section configSection) error {
	r.mux.Lock()
	defer r.mux.Unlock()

	if id == "" {
		return errors.New("cannot register a section with no ID")
	}
	if _, exists := r.sections[id]; exists {
		return fmt.Errorf("section %s is already registered", id)
	}

	r.sections[id] = section
	return nil
}

func (r *configSectionRegistry) getSections() map[string]configSection {
	r.mux.RLock()
	defer r.mux.RUnlock()

	return r.sections
}
