package evergreen

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/grip"
)

var registry *configSectionRegistry

func init() {

	if err := resetRegistry(); err != nil {
		panic(fmt.Sprintf("error registering config sections: %s", err.Error()))
	}
}

type configSectionRegistry struct {
	mu       *sync.RWMutex
	sections map[string]configSection
}

func resetRegistry() error {
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

	return catcher.Resolve()
}

func newConfigSectionRegistry() *configSectionRegistry {
	return &configSectionRegistry{
		mu:       &sync.RWMutex{},
		sections: map[string]configSection{},
	}
}

func (r *configSectionRegistry) registerSection(id string, section configSection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections
}
