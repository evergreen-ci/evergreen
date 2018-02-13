package evergreen

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/grip"
)

var configRegistry *ConfigSectionRegistry

func init() {

	if err := resetRegistry(); err != nil {
		panic(fmt.Sprintf("error registering config sections: %s", err.Error()))
	}
}

type ConfigSectionRegistry struct {
	mu       sync.RWMutex
	sections map[string]ConfigSection
}

func resetRegistry() error {
	// add any new config sections to the variable below to register them
	ConfigSections := []ConfigSection{
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

	configRegistry = newConfigSectionRegistry()
	catcher := grip.NewSimpleCatcher()

	for _, section := range ConfigSections {
		catcher.Add(configRegistry.registerSection(section.id(), section))
	}

	return catcher.Resolve()
}

func newConfigSectionRegistry() *ConfigSectionRegistry {
	return &ConfigSectionRegistry{
		sections: map[string]ConfigSection{},
	}
}

func (r *ConfigSectionRegistry) registerSection(id string, section ConfigSection) error {
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

func (r *ConfigSectionRegistry) getSections() map[string]ConfigSection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections
}
