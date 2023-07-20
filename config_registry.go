package evergreen

import (
	"context"
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/pkg/errors"

	"github.com/mongodb/grip"
)

// In order to add a new config section:
// 1. modify the struct in config.go to add whatever you need to add
// 2. add the struct to the ConfigSections variable below
// 3. add a copy of the struct you added in 1 to rest/model/admin.go and implement the
//    conversion methods. The property name must be exactly the same in the DB/API model
// 4. add it to MockConfig in testutil/config.go for testing, if desired

var ConfigRegistry *ConfigSectionRegistry

func init() {
	if err := resetRegistry(); err != nil {
		panic(errors.Wrap(err, "registering config sections").Error())
	}
}

type ConfigSectionRegistry struct {
	mu       sync.RWMutex
	sections map[string]ConfigSection
}

func resetRegistry() error {
	ConfigSections := []ConfigSection{
		&AlertsConfig{},
		&AmboyConfig{},
		&APIConfig{},
		&AuthConfig{},
		&BucketConfig{},
		&CedarConfig{},
		&CloudProviders{},
		&CommitQueueConfig{},
		&ContainerPoolsConfig{},
		&DataPipesConfig{},
		&HostInitConfig{},
		&HostJasperConfig{},
		&JiraConfig{},
		&LoggerConfig{},
		&NewRelicConfig{},
		&NotifyConfig{},
		&PodLifecycleConfig{},
		&ProjectCreationConfig{},
		&RepoTrackerConfig{},
		&SchedulerConfig{},
		&ServiceFlags{},
		&SlackConfig{},
		&SplunkConfig{},
		&UIConfig{},
		&Settings{},
		&JIRANotificationsConfig{},
		&TriggerConfig{},
		&SpawnHostConfig{},
		&TracerConfig{},
	}

	ConfigRegistry = newConfigSectionRegistry()
	catcher := grip.NewSimpleCatcher()

	for _, section := range ConfigSections {
		catcher.Add(ConfigRegistry.registerSection(section.SectionId(), section))
	}

	return catcher.Resolve()
}

func newConfigSectionRegistry() *ConfigSectionRegistry {
	return &ConfigSectionRegistry{
		sections: map[string]ConfigSection{},
	}
}

func (r *ConfigSectionRegistry) populateSections(ctx context.Context) error {
	var sectionIDs = make([]string, 0, len(r.sections))
	for sectionID := range r.sections {
		sectionIDs = append(sectionIDs, sectionID)
	}

	rawSections, err := getSectionsBSON(ctx, sectionIDs)
	if err != nil {
		return errors.Wrap(err, "getting raw sections")
	}

	catcher := grip.NewBasicCatcher()
	for _, rawSection := range rawSections {
		id, err := rawSection.LookupErr("_id")
		if err != nil {
			continue
		}
		idString, ok := id.StringValueOK()
		if !ok {
			continue
		}
		section, ok := r.sections[idString]
		if !ok {
			continue
		}
		if err = bson.Unmarshal(rawSection, section); err != nil {
			catcher.Add(errors.Wrapf(err, "unmarshaling section '%s'", idString))
		}
	}

	return catcher.Resolve()
}

func (r *ConfigSectionRegistry) registerSection(id string, section ConfigSection) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id == "" {
		return errors.New("cannot register a section with no ID")
	}
	if _, exists := r.sections[id]; exists {
		return fmt.Errorf("section '%s' is already registered", id)
	}

	r.sections[id] = section
	return nil
}

func (r *ConfigSectionRegistry) GetSections() map[string]ConfigSection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections
}

func (r *ConfigSectionRegistry) GetSection(id string) ConfigSection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sections[id]
}
