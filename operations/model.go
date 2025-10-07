package operations

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/kanopy-platform/kanopy-oidc-lib/pkg/dex"
	"github.com/kardianos/osext"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const localConfigPath = ".evergreen.local.yml"
const stagingCorpHost = "https://evergreen.staging.corp.mongodb.com/api"
const stagingNonCorpHost = "https://evergreen-staging.corp.mongodb.com/api"
const prodCorpHost = "https://evergreen.corp.mongodb.com/api"
const prodNonCorpHost = "https://evergreen.mongodb.com/api"

type ClientProjectConf struct {
	Name           string               `json:"name" yaml:"name,omitempty"`
	Default        bool                 `json:"default" yaml:"default,omitempty"`
	Alias          string               `json:"alias" yaml:"alias,omitempty"`
	Variants       []string             `json:"variants" yaml:"variants,omitempty"`
	Tasks          []string             `json:"tasks" yaml:"tasks,omitempty"`
	Parameters     map[string]string    `json:"parameters" yaml:"parameters,omitempty"`
	ModulePaths    map[string]string    `json:"module_paths" yaml:"module_paths,omitempty"`
	TriggerAliases []string             `json:"trigger_aliases" yaml:"trigger_aliases"`
	LocalAliases   []model.ProjectAlias `json:"local_aliases,omitempty" yaml:"local_aliases,omitempty"`
}

func findConfigFilePath(fn string) (string, error) {
	currentBinPath, _ := osext.Executable()

	userHome, _ := util.GetUserHome()

	if fn != "" {
		if isValidPath(fn) {
			return fn, nil
		}
		absfn, _ := filepath.Abs(fn)
		if isValidPath(absfn) {
			return absfn, nil
		}
	}
	defaultFiles := []string{
		filepath.Join(userHome, evergreen.DefaultEvergreenConfig),
		filepath.Join(filepath.Dir(currentBinPath), evergreen.DefaultEvergreenConfig),
	}
	for _, path := range defaultFiles {
		if isValidPath(path) {
			grip.WarningWhen(fn != "", "Couldn't find configuration file, falling back on default.")
			return path, nil
		}
	}

	return "", errors.New("could not find client configuration file on the local system")
}

func isValidPath(path string) bool {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) || stat.IsDir() {
		return false
	}
	return true
}

// OAuth contains the configuration and tokens for OAuth authentication
// with the Evergreen API.
type OAuth struct {
	// These are static fields needed to configure the OAuth client.
	Issuer      string `json:"issuer" yaml:"issuer,omitempty"`
	ClientID    string `json:"client_id" yaml:"client_id,omitempty"`
	ConnectorID string `json:"connector_id" yaml:"connector_id,omitempty"`

	// These are helpers that users can set.
	// DoNotUseBrowser indicates that the OAuth flow should not attempt to open a browser.
	// This setting is the final authority on the flow.
	DoNotUseBrowser bool `json:"do_not_use_browser" yaml:"do_not_use_browser,omitempty"`
}

// Client represents the data stored in the user's config file, by default
// located at ~/.evergreen.yml
// If you change the JSON tags, you must also change an anonymous struct in hostinit/setup.go
type ClientSettings struct {
	APIServerHost              string                      `json:"api_server_host" yaml:"api_server_host,omitempty"`
	UIServerHost               string                      `json:"ui_server_host" yaml:"ui_server_host,omitempty"`
	APIKey                     string                      `json:"api_key" yaml:"api_key,omitempty"`
	User                       string                      `json:"user" yaml:"user,omitempty"`
	JWT                        string                      `json:"jwt" yaml:"jwt,omitempty"`
	UncommittedChanges         bool                        `json:"patch_uncommitted_changes" yaml:"patch_uncommitted_changes,omitempty"`
	AutoUpgradeCLI             bool                        `json:"auto_upgrade_cli" yaml:"auto_upgrade_cli,omitempty"`
	DoNotUseJWT                bool                        `json:"do_not_use_jwt" yaml:"do_not_use_jwt,omitempty"`
	PreserveCommits            bool                        `json:"preserve_commits" yaml:"preserve_commits,omitempty"`
	Projects                   []ClientProjectConf         `json:"projects" yaml:"projects,omitempty"`
	LoadedFrom                 string                      `json:"-" yaml:"-"`
	DisableAutoDefaulting      bool                        `json:"disable_auto_defaulting" yaml:"disable_auto_defaulting"`
	ProjectsForDirectory       map[string]string           `json:"projects_for_directory,omitempty" yaml:"projects_for_directory,omitempty"`
	LastRevisionCriteriaGroups []lastRevisionCriteriaGroup `json:"last_revision_criteria_groups,omitempty" yaml:"last_revision_criteria_groups,omitempty"`

	OAuth OAuth `json:"oauth,omitempty" yaml:"oauth,omitempty"`

	// StagingEnvironment configures which staging environment to point to.
	StagingEnvironment string `json:"staging_environment,omitempty" yaml:"staging_environment,omitempty"`
}

func NewClientSettings(fn string) (*ClientSettings, error) {
	path, err := findConfigFilePath(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "finding config file '%s'", fn)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "reading configuration from file '%s'", path)
	}

	conf := &ClientSettings{}
	if err = yaml.Unmarshal(data, conf); err != nil {
		return nil, errors.Wrapf(err, "reading YAML data from configuration file '%s'", path)
	}
	conf.LoadedFrom = path

	localData, err := os.ReadFile(localConfigPath)
	if os.IsNotExist(err) {
		return conf, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "reading local configuration from file '%s'", localConfigPath)
	}

	// Unmarshalling into the same struct will only override fields which are set
	// in the new YAML
	if err = yaml.Unmarshal(localData, conf); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling YAML data from local configuration file '%s'", localConfigPath)
	}

	return conf, nil
}

func (s *ClientSettings) Write(fn string) error {
	if fn == "" {
		if s.LoadedFrom != "" {
			fn = s.LoadedFrom
		}
	}
	if fn == "" {
		return errors.New("no output location specified")
	}

	yamlData, err := yaml.Marshal(s)
	if err != nil {
		return errors.Wrap(err, "marshalling data to write")
	}

	return errors.Wrapf(os.WriteFile(fn, yamlData, 0644), "writing file '%s'", fn)
}

// setupRestCommunicator returns the rest communicator and prints any available info messages if set.
// Callers are responsible for calling (Communicator).Close() when finished with the client.
// We want to avoid printing messages if output is requested in a specific format or silenced.
func (s *ClientSettings) setupRestCommunicator(ctx context.Context, printMessages bool) (client.Communicator, error) {
	c, err := client.NewCommunicator(s.APIServerHost)
	if err != nil {
		return nil, errors.Wrap(err, "getting REST communicator")
	}

	c.SetAPIUser(s.User)
	c.SetAPIKey(s.APIKey)
	if err = s.checkCLIVersion(ctx, c); err != nil {
		return nil, err
	}
	if printMessages {
		printUserMessages(ctx, c, !s.AutoUpgradeCLI)
	}

	shouldGenerate, reason := s.shouldGenerateJWT(ctx, c)
	if shouldGenerate {
		grip.Info(optOut)
		if err := s.SetOAuthToken(ctx, c); err != nil {
			return c, errors.Wrap(err, "setting OAuth token")
		}
		c.SetJWT(s.OAuth.AccessToken)
		c.SetAPIKey("")
		// To use JWT's, we need to use the corp API server host
		// URL.
		c.SetAPIServerHost(s.getApiServerHost(true))
	} else {
		if reason != "" {
			grip.Info(reason)
		}
	}

	return c, nil
}

func (s *ClientSettings) shouldGenerateJWT(ctx context.Context, c client.Communicator) (bool, string) {
	if s.DoNotUseJWT {
		return false, ""
	}

	if s.APIKey == "" {
		return true, "No API key found in local Evergreen YAML, defaulting to a JWT token."
	}

	// always use the non-corp url for getting the service flags
	// because the corp url needs a JWT token which we haven't generated yet
	originalAPIServerHost := s.APIServerHost
	c.SetAPIServerHost(s.getApiServerHost(false))

	isServiceUser, err := c.IsServiceUser(ctx, s.User)

	if err != nil {
		errorMsg := "Failed to check if user is a service user"
		isUnauthorizedErr := strings.Contains(err.Error(), "401")
		if isUnauthorizedErr {
			// if we get a 401, the api key is likely invalid, so we should try to generate a token
			// because otherwise subsequent api requests will likely fail too.
			return true, fmt.Sprintf("%s, will try to generate a token: %s", errorMsg, err)
		}
		return false, fmt.Sprintf("%s: %s", errorMsg, err)
	}
	if isServiceUser {
		return false, ""
	}

	flags, err := c.GetServiceFlags(ctx)
	// reset the api server host to the original value once we have the flags
	c.SetAPIServerHost(originalAPIServerHost)

	if err == nil && !flags.JWTTokenForCLIDisabled {
		return true, ""
	}

	return false, ""
}

// getApiServerHost returns the API server host based on the APIServerHost and the useCorp parameter.
func (s *ClientSettings) getApiServerHost(useCorp bool) string {
	if useCorp {
		if s.APIServerHost == stagingNonCorpHost {
			return stagingCorpHost
		}
		if s.APIServerHost == prodNonCorpHost {
			return prodCorpHost
		}
	} else {
		if s.APIServerHost == stagingCorpHost {
			return stagingNonCorpHost
		}
		if s.APIServerHost == prodCorpHost {
			return prodNonCorpHost
		}
	}

	return s.APIServerHost
}

// checkCLIVersion checks if the CLI version is too old and errors if it is.
// It also temporarily sets the OAuth fields in the client settings and saves it.
// This is to support migrating to OAuth authentication for DEVPROD-4160.
func (s *ClientSettings) checkCLIVersion(ctx context.Context, c client.Communicator) error {
	clients, err := c.GetClientConfig(ctx)
	if err != nil {
		grip.Debug(errors.Wrap(err, "getting client config info"))
	}
	if clients == nil {
		return nil
	}
	if clients.OldestAllowedCLIVersion != "" {
		isCLIVersionTooOld, err := isFirstDateBefore(evergreen.ClientVersion, clients.OldestAllowedCLIVersion)
		if err != nil {
			grip.Warning(errors.Wrap(err, "checking if client is older than the latest version"))
		}
		if isCLIVersionTooOld {
			return errors.Errorf("CLI version '%s' is older than the oldest allowed CLI version '%s'. "+
				"Run '%s get-update --install' to update.\n", evergreen.ClientVersion, clients.OldestAllowedCLIVersion, os.Args[0])
		}
	}
	if clients.OAuthIssuer != "" && s.OAuth.Issuer == "" {
		s.OAuth.ClientID = clients.OAuthClientID
		s.OAuth.ConnectorID = clients.OAuthConnectorID
		s.OAuth.Issuer = clients.OAuthIssuer

		// save the configuration file
		if err := s.Write(""); err != nil {
			// This shouldn't prevent users from using the CLI so just log a warning.
			grip.Warning(errors.Wrap(err, "saving configuration file"))
		}
	}
	return nil
}

// printUserMessages prints any available info messages.
func printUserMessages(ctx context.Context, c client.Communicator, checkForUpdate bool) {
	banner, err := c.GetBannerMessage(ctx)
	if err != nil {
		grip.Debug(errors.Wrap(err, "getting banner messages"))
	} else if len(banner) > 0 {
		grip.Noticef("Banner: %s", banner)
	}

	if checkForUpdate {
		update, err := checkUpdate(c, true, false)
		if err != nil {
			grip.Debug(err)
		}
		if update.needsUpdate {
			if runtime.GOOS == "windows" {
				fmt.Fprintf(os.Stderr, "A new version is available. Run '%s get-update' to fetch it.\n", os.Args[0])
			} else {
				fmt.Fprintf(os.Stderr, "A new version is available. Run '%s get-update --install' to download and install it.\n", os.Args[0])
			}
		}
	}
}

func isFirstDateBefore(dateString1, dateString2 string) (bool, error) {
	layout := "2006-01-02"
	// Extract just the date portion if the string is long enough.
	// This allows values such as "2024-08-10a" to be parsed as "2024-08-10"
	if len(dateString1) >= 10 {
		dateString1 = dateString1[:10]
	}
	if len(dateString2) >= 10 {
		dateString2 = dateString2[:10]
	}
	t1, err := time.Parse(layout, dateString1)
	if err != nil {
		return false, fmt.Errorf("error parsing first date '%s': %w", dateString1, err)
	}
	t2, err := time.Parse(layout, dateString2)
	if err != nil {
		return false, fmt.Errorf("error parsing second date '%s': %w", dateString2, err)
	}
	return t1.Before(t2), nil
}

func (s *ClientSettings) getLegacyClients() (*legacyClient, *legacyClient, error) {
	// create client for the REST APIs
	apiURL, err := url.Parse(s.APIServerHost)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parsing API server URL from settings file")
	}

	root := s.getApiServerHost(s.JWT != "")
	ac := &legacyClient{
		APIRoot:            root,
		APIRootV2:          root + "/rest/v2",
		User:               s.User,
		APIKey:             s.APIKey,
		JWT:                s.JWT,
		stagingEnvironment: s.StagingEnvironment,
	}

	rc := &legacyClient{
		APIRoot:            apiURL.Scheme + "://" + apiURL.Host + "/rest/v1",
		APIRootV2:          apiURL.Scheme + "://" + apiURL.Host + "/rest/v2",
		User:               s.User,
		APIKey:             s.APIKey,
		JWT:                s.JWT,
		stagingEnvironment: s.StagingEnvironment,
	}

	return ac, rc, nil
}

func (s *ClientSettings) getModule(patchId, moduleName string) (*model.Module, error) {
	_, rc, err := s.getLegacyClients()
	if err != nil {
		return nil, errors.Wrap(err, "setting up legacy Evergreen client")
	}
	proj, err := rc.GetPatchedConfig(patchId)
	if err != nil {
		return nil, err
	}

	const helpText = "Note: In order to set a module, you need to be in the directory for the module project, not the directory for the project that the module is being applied onto."

	if len(proj.Modules) == 0 {
		return nil, errors.Errorf("Project has no configured modules. Specify different project or "+
			"see the evergreen configuration file for module configuration.\n %s", helpText)
	}
	module, err := model.GetModuleByName(proj.Modules, moduleName)
	if err != nil {
		moduleNames := []string{}
		for _, m := range proj.Modules {
			moduleNames = append(moduleNames, m.Name)
		}
		return nil, errors.Errorf("Could not find module named '%s' for project; specify different project or select correct module from:\n\t%s\n%s",
			moduleName, strings.Join(moduleNames, "\n\t"), helpText)
	}
	return module, nil
}

func (s *ClientSettings) FindDefaultProject(cwd string, useRoot bool) string {
	if project, exists := s.ProjectsForDirectory[cwd]; exists {
		return project
	}

	if useRoot {
		for _, p := range s.Projects {
			if p.Default {
				return p.Name
			}
		}
	}
	return ""
}

// getModulePathsForProject returns the map of modules to local paths for the given project.
func (s *ClientSettings) getModulePathsForProject(project string) map[string]string {
	for _, p := range s.Projects {
		if p.Name == project && p.ModulePaths != nil {
			return p.ModulePaths
		}
	}
	return map[string]string{}
}

// setModulePath updates the given client settings to match the given module patch cache.
func (s *ClientSettings) setModulePath(project string, modulePathCache map[string]string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].ModulePaths = modulePathCache
			return
		}
	}
	s.Projects = append(s.Projects, ClientProjectConf{
		Name:        project,
		ModulePaths: modulePathCache,
	})
}

func (s *ClientSettings) FindDefaultVariants(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Variants
		}
	}
	return nil
}

func (s *ClientSettings) SetDefaultVariants(project string, variants ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Variants = variants
			return
		}
	}
	s.Projects = append(s.Projects, ClientProjectConf{
		Name:     project,
		Default:  true,
		Alias:    "",
		Variants: variants,
		Tasks:    nil,
	})
}

func (s *ClientSettings) FindDefaultTriggerAliases(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.TriggerAliases
		}
	}
	return nil
}

func (s *ClientSettings) SetDefaultTriggerAliases(project string, triggerAliases []string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].TriggerAliases = triggerAliases
			return
		}
	}

	s.Projects = append(s.Projects, ClientProjectConf{
		Name:           project,
		Default:        true,
		TriggerAliases: triggerAliases,
	})
}

func (s *ClientSettings) FindDefaultParameters(project string) []patch.Parameter {
	for _, p := range s.Projects {
		if p.Name == project {
			return parametersFromMap(p.Parameters)
		}
	}
	return nil
}

func parametersFromMap(params map[string]string) []patch.Parameter {
	res := []patch.Parameter{}
	for key, val := range params {
		res = append(res, patch.Parameter{Key: key, Value: val})
	}
	return res
}

func (s *ClientSettings) FindDefaultTasks(project string) []string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Tasks
		}
	}
	return nil
}

func (s *ClientSettings) SetDefaultTasks(project string, tasks ...string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Tasks = tasks
			return
		}
	}
	s.Projects = append(s.Projects, ClientProjectConf{
		Name:     project,
		Default:  true,
		Alias:    "",
		Variants: nil,
		Tasks:    tasks,
	})
}

func (s *ClientSettings) FindDefaultAlias(project string) string {
	for _, p := range s.Projects {
		if p.Name == project {
			return p.Alias
		}
	}
	return ""
}

func (s *ClientSettings) SetDefaultAlias(project string, alias string) {
	for i, p := range s.Projects {
		if p.Name == project {
			s.Projects[i].Alias = alias
			return
		}
	}
	s.Projects = append(s.Projects, ClientProjectConf{
		Name:     project,
		Default:  true,
		Alias:    alias,
		Variants: nil,
		Tasks:    nil,
	})
}

func (s *ClientSettings) SetDefaultProject(cwd, project string) {
	if s.DisableAutoDefaulting {
		return
	}

	_, found := s.ProjectsForDirectory[cwd]
	if found {
		return
	}
	if s.ProjectsForDirectory == nil {
		s.ProjectsForDirectory = map[string]string{}
	}
	s.ProjectsForDirectory[cwd] = project
	grip.Infof("Project '%s' will be set as the one to use for directory '%s'. To disable automatic defaulting, set 'disable_auto_defaulting' to true.", project, cwd)
}

func (s *ClientSettings) SetAutoUpgradeCLI() {
	s.AutoUpgradeCLI = true
	grip.Info("Evergreen CLI will be automatically updated and installed before each command if a more recent version is detected.")
}

func (s *ClientSettings) SetOAuthToken(ctx context.Context, comm client.Communicator) error {
	_, err := comm.GetOIDCToken(ctx,
		dex.WithIssuer(s.OAuth.Issuer),
		dex.WithClientID(s.OAuth.ClientID),
		dex.WithConnectorID(s.OAuth.ConnectorID),
		dex.WithTokenLoader(&configurationTokenLoader{conf: s}),
	)
	return errors.Wrap(err, "setting OAuth token")
}
