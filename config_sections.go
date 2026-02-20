package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mongodb/grip"
)

// In order to add a new config section:
//  1. modify the struct in config.go to add whatever you need to add
//  2. add the struct to the ConfigSections constructor below
//  3. add a copy of the struct you added in 1 to rest/model/admin.go and implement the
//     conversion methods. The property name must be exactly the same in the DB/API model
//  4. add it to MockConfig in testutil/config.go for testing, if desired

type ConfigSections struct {
	Sections map[string]ConfigSection
}

func NewConfigSections() ConfigSections {
	sections := []ConfigSection{
		&AmboyConfig{},
		&AmboyDBConfig{},
		&APIConfig{},
		&AuthConfig{},
		&BucketsConfig{},
		&CedarConfig{},
		&CloudProviders{},
		&ContainerPoolsConfig{},
		&CostConfig{},
		&DebugSpawnHostsConfig{},
		&FWSConfig{},
		&GraphiteConfig{},
		&HostInitConfig{},
		&HostJasperConfig{},
		&JiraConfig{},
		&LoggerConfig{},
		&NotifyConfig{},
		&OverridesConfig{},
		&ParameterStoreConfig{},
		&ProjectCreationConfig{},
		&RepoTrackerConfig{},
		&ReleaseModeConfig{},
		&RuntimeEnvironmentsConfig{},
		&SageConfig{},
		&SchedulerConfig{},
		&ServiceFlags{},
		&SingleTaskDistroConfig{},
		&SlackConfig{},
		&SleepScheduleConfig{},
		&SplunkConfig{},
		&SSHConfig{},
		&UIConfig{},
		&Settings{},
		&JIRANotificationsConfig{},
		&TaskLimitsConfig{},
		&TestSelectionConfig{},
		&TriggerConfig{},
		&SpawnHostConfig{},
		&TracerConfig{},
		&GitHubCheckRunConfig{},
	}

	sectionMap := make(map[string]ConfigSection, len(sections))
	for _, section := range sections {
		sectionMap[section.SectionId()] = section
	}
	return ConfigSections{Sections: sectionMap}
}

func (c *ConfigSections) populateSections(ctx context.Context, includeOverrides bool) error {
	sectionIDs := make([]string, 0, len(c.Sections))
	for sectionID := range c.Sections {
		sectionIDs = append(sectionIDs, sectionID)
	}

	rawSections, err := getSectionsBSON(ctx, sectionIDs, includeOverrides)
	if err != nil {
		return errors.Wrap(err, "getting raw sections")
	}
	catcher := grip.NewBasicCatcher()
	for _, rawSection := range rawSections {
		catcher.Add(c.unmarshallSection(rawSection))
	}

	return catcher.Resolve()
}

func (c *ConfigSections) unmarshallSection(rawSection bson.Raw) error {
	id, err := rawSection.LookupErr("_id")
	if err != nil {
		return nil
	}
	idString, ok := id.StringValueOK()
	if !ok {
		return nil
	}
	section, ok := c.Sections[idString]
	if !ok {
		return nil
	}
	return errors.Wrapf(bson.Unmarshal(rawSection, section), "unmarshalling section '%s'", idString)
}
