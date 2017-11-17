package plugin

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
)

// PanelConfig stores all UI-related plugin hooks
type PanelConfig struct {
	// Handler is an http.Handler which receives plugin-specific HTTP requests from the UI
	Handler http.Handler

	// Panels is an array of UIPanels to inject into task, version,
	// and build pages
	Panels []UIPanel
}

// PageScope is a type for setting the page a panel appears on
type PageScope string

// pagePosition is a private type for setting where on a page a panel appears
type pagePosition string

const (
	// These PageScope constants determine which page a panel hooks into
	TaskPage    PageScope = "task"
	BuildPage   PageScope = "build"
	VersionPage PageScope = "version"

	// These pagePosition constants determine where on a page a panel is
	// injected. If no position is given, it defaults to PageCenter
	PageLeft   pagePosition = "Left"
	PageRight  pagePosition = "Right"
	PageCenter pagePosition = "Center"
)

type pluginUser int

const pluginUserKey pluginUser = 0

// UIDataFunction is a function which is called to populate panels
// which are injected into Task/Build/Version pages at runtime.
type UIDataFunction func(context UIContext) (interface{}, error)

// UIPage represents the information to be sent over to the ui server
// in order to render a page for an app level plugin.
// TemplatePath is the relative path to the template from the template root of the plugin.
// Data represents the data to send over.
type UIPage struct {
	TemplatePath string
	DataFunc     UIDataFunction
}

// UIPanel is a type for storing all the configuration to properly
// display one panel in one page of the UI.
type UIPanel struct {
	// Page is which page the panel appears on
	Page PageScope

	// Includes is a list of HTML tags to inject into the head of
	// the page. These are meant to be links to css and js code hosted
	// in the plugin's static web root
	Includes []template.HTML

	// PanelHTML is the HTML definition of the panel. Best practices dictate
	// using AngularJS to load up the html as a partial hosted by the plugin
	PanelHTML template.HTML

	// DataFunc is a function to populate plugin data injected into the js
	// of the page the panel is on. The function takes the page request as
	// an argument, and returns data (must be json-serializeable!) or an error
	DataFunc UIDataFunction
	// Position is the side of the page the panel appears in
	Position pagePosition
}

// UIContext stores all relevant models for a plugin page.
type UIContext struct {
	Settings   evergreen.Settings
	User       *user.DBUser
	Task       *task.Task
	Build      *build.Build
	Version    *version.Version
	Patch      *patch.Patch
	ProjectRef *model.ProjectRef
	Request    *http.Request
}

// PanelLayout tells the view renderer what panel HTML data to inject and where
// on the page to inject it.
type PanelLayout struct {
	Left   []template.HTML
	Right  []template.HTML
	Center []template.HTML
}

// PanelManager is the manager the UI server uses to register and load
// plugin UI information efficiently.
type PanelManager interface {
	RegisterPlugins([]UIPlugin) error
	Includes(PageScope) ([]template.HTML, error)
	Panels(PageScope) (PanelLayout, error)
	UIData(UIContext, PageScope) (map[string]interface{}, error)
	GetAppPlugins() []AppUIPlugin
}

// private type for sorting alphabetically,
// holds a pairing of plugin name and HTML
type pluginTemplatePair struct {
	Name     string
	Template template.HTML
}

// private type for sorting methods
type pairsByLexicographicalOrder []pluginTemplatePair

func (a pairsByLexicographicalOrder) Len() int {
	return len(a)
}
func (a pairsByLexicographicalOrder) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a pairsByLexicographicalOrder) Less(i, j int) bool {
	return a[i].Name < a[j].Name
}

// sortAndExtractHTML takes a list of pairings of <plugin name, html data>,
// sorts them by plugin name, and returns a list of the just
// the html data
func sortAndExtractHTML(pairs []pluginTemplatePair) []template.HTML {
	var tplList []template.HTML
	sort.Stable(pairsByLexicographicalOrder(pairs))
	for _, pair := range pairs {
		tplList = append(tplList, pair.Template)
	}
	return tplList
}

// SimplePanelManager is a basic implementation of a plugin panel manager.
type SimplePanelManager struct {
	includes    map[PageScope][]template.HTML
	panelHTML   map[PageScope]PanelLayout
	uiDataFuncs map[PageScope]map[string]UIDataFunction
	appPlugins  []AppUIPlugin
}

// RegisterPlugins takes an array of plugins and registers them with the
// manager. After this step is done, the other manager functions may be used.
func (self *SimplePanelManager) RegisterPlugins(plugins []UIPlugin) error {
	//initialize temporary maps
	registered := map[string]bool{}
	includesWithPair := map[PageScope][]pluginTemplatePair{}
	panelHTMLWithPair := map[PageScope]map[pagePosition][]pluginTemplatePair{
		TaskPage:    {},
		BuildPage:   {},
		VersionPage: {},
	}
	dataFuncs := map[PageScope]map[string]UIDataFunction{
		TaskPage:    {},
		BuildPage:   {},
		VersionPage: {},
	}

	appPluginNames := []AppUIPlugin{}
	for _, p := range plugins {
		// don't register plugins twice
		if registered[p.Name()] {
			return errors.Errorf("plugin '%v' already registered", p.Name())
		}
		// check if a plugin is an app level plugin first
		if appPlugin, ok := p.(AppUIPlugin); ok {
			appPluginNames = append(appPluginNames, appPlugin)
		}

		if uiConf, err := p.GetPanelConfig(); uiConf != nil && err == nil {
			for _, panel := range uiConf.Panels {

				// register all includes to their proper scope
				for _, include := range panel.Includes {
					includesWithPair[panel.Page] = append(
						includesWithPair[panel.Page],
						pluginTemplatePair{p.Name(), include},
					)
				}

				// register all panels to their proper scope and position
				if panel.Page == "" {
					return errors.New("plugin '%v': cannot register ui panel without a Page")
				}
				if panel.Position == "" {
					panel.Position = PageCenter // Default to center
				}
				panelHTMLWithPair[panel.Page][panel.Position] = append(
					panelHTMLWithPair[panel.Page][panel.Position],
					pluginTemplatePair{p.Name(), panel.PanelHTML},
				)

				// register all data functions to their proper scope, if they exist
				// Note: only one function can be registered per plugin per page
				if dataFuncs[panel.Page][p.Name()] == nil {
					if panel.DataFunc != nil {
						dataFuncs[panel.Page][p.Name()] = panel.DataFunc
					}
				} else {
					if panel.DataFunc != nil {
						return errors.Errorf(
							"a data function is already registered for plugin %v on %v page",
							p.Name(), panel.Page)
					}
				}
			}
		} else if err != nil {
			return errors.Wrapf(err, "GetPanelConfig for plugin '%v' returned an error", p.Name())
		}

		registered[p.Name()] = true
	}

	self.appPlugins = appPluginNames

	// sort registered plugins by name and cache their HTML
	self.includes = map[PageScope][]template.HTML{
		TaskPage:    sortAndExtractHTML(includesWithPair[TaskPage]),
		BuildPage:   sortAndExtractHTML(includesWithPair[BuildPage]),
		VersionPage: sortAndExtractHTML(includesWithPair[VersionPage]),
	}
	self.panelHTML = map[PageScope]PanelLayout{
		TaskPage: {
			Left:   sortAndExtractHTML(panelHTMLWithPair[TaskPage][PageLeft]),
			Right:  sortAndExtractHTML(panelHTMLWithPair[TaskPage][PageRight]),
			Center: sortAndExtractHTML(panelHTMLWithPair[TaskPage][PageCenter]),
		},
		BuildPage: {
			Left:   sortAndExtractHTML(panelHTMLWithPair[BuildPage][PageLeft]),
			Right:  sortAndExtractHTML(panelHTMLWithPair[BuildPage][PageRight]),
			Center: sortAndExtractHTML(panelHTMLWithPair[BuildPage][PageCenter]),
		},
		VersionPage: {
			Left:   sortAndExtractHTML(panelHTMLWithPair[VersionPage][PageLeft]),
			Right:  sortAndExtractHTML(panelHTMLWithPair[VersionPage][PageRight]),
			Center: sortAndExtractHTML(panelHTMLWithPair[VersionPage][PageCenter]),
		},
	}
	self.uiDataFuncs = dataFuncs

	return nil
}

// Includes returns a properly-ordered list of html tags to inject into the
// head of the view for the given page.
func (self *SimplePanelManager) Includes(page PageScope) ([]template.HTML, error) {
	return self.includes[page], nil
}

// Panels returns a PanelLayout for the view renderer to inject panels into
// the given page.
func (self *SimplePanelManager) Panels(page PageScope) (PanelLayout, error) {
	return self.panelHTML[page], nil
}

func (self *SimplePanelManager) GetAppPlugins() []AppUIPlugin {
	return self.appPlugins
}

// UIData returns a map of plugin name -> data for inclusion
// in the view's javascript.
func (self *SimplePanelManager) UIData(context UIContext, page PageScope) (map[string]interface{}, error) {
	pluginUIData := map[string]interface{}{}
	errs := &UIDataFunctionError{}
	for plName, dataFunc := range self.uiDataFuncs[page] {
		// run the data function, catching all sorts of errors
		plData, err := func() (data interface{}, err error) {
			defer func() {
				if r := recover(); r != nil {
					err = errors.Errorf("plugin function panicked: %v", r)
				}
			}()
			data, err = dataFunc(context)
			return
		}()
		if err != nil {
			// append error to error list and continue
			plData = nil
			errs.AppendError(plName, err)
		}
		pluginUIData[plName] = plData
	}
	if errs.HasErrors() {
		return pluginUIData, errs
	}
	return pluginUIData, nil
}

// UIDataFunctionError is a special error type for data function processing
// which can record and aggregate multiple error messages.
type UIDataFunctionError []error

// AppendError adds an error onto the array of data function errors.
func (errs *UIDataFunctionError) AppendError(name string, err error) {
	*errs = append(*errs, errors.Errorf("{'%v': %v}", name, err))
}

// HasErrors returns a boolean representing if the UIDataFunctionError
// contains any errors.
func (errs *UIDataFunctionError) HasErrors() bool {
	return len(*errs) > 0
}

// Error returns a string aggregating the stored error messages.
// Implements the error interface.
func (errs *UIDataFunctionError) Error() string {
	return fmt.Sprintf(
		"encountered errors processing UI data functions: %v",
		*errs,
	)
}

func TemplateRoot(name string) string {
	return filepath.Join(evergreen.FindEvergreenHome(), "service", "plugins", name, "templates")
}
