package plugin

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/context"
	"html/template"
	"net/http"
	"path"
	"runtime"
	"sort"
)

// PanelConfig stores all UI-related plugin hooks
type PanelConfig struct {
	// StaticRoot is a filesystem path to a folder to establish
	// as the static web root for a plugin package. If this field is set,
	// the UI server will set up a file serving route for the plugin at
	//  /plugin/<plugin_name>/static
	// See StaticWebRootFromSourceFile() as a shortcut for setting this up
	StaticRoot string

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

type pluginContext int
type pluginUser int

const pluginContextKey pluginContext = 0
const pluginUserKey pluginUser = 0

// MustHaveContext loads a UIContext from the http.Request. It panics
// if the context is not set.
func MustHaveContext(request *http.Request) UIContext {
	if c := context.Get(request, pluginContextKey); c != nil {
		return c.(UIContext)
	}
	panic("no UI context found")
}

// SetUser sets the user for the request context. This is a helper for UI middleware.
func SetUser(u *user.DBUser, r *http.Request) {
	context.Set(r, pluginUserKey, u)
}

// GetUser fetches the user, if it exists, from the request context.
func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, pluginUserKey); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

// UIDataFunction is a function which is called to populate panels
// which are injected into Task/Build/Version pages at runtime.
type UIDataFunction func(context UIContext) (interface{}, error)

// UIPanel is a type for storing all the configuration to properly
// display one panel in one page of the UI.
type UIPanel struct {
	// Page is which page the panel appears on
	Page PageScope

	// Position is the side of the page the panel appears in
	Position pagePosition

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
}

// UIContext stores all relevant models for a plugin page.
type UIContext struct {
	Settings   evergreen.Settings
	User       *user.DBUser
	Task       *model.Task
	Build      *build.Build
	Version    *version.Version
	Patch      *patch.Patch
	Project    *model.Project
	ProjectRef *model.ProjectRef
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
}

// RegisterPlugins takes an array of plugins and registers them with the
// manager. After this step is done, the other manager functions may be used.
func (self *SimplePanelManager) RegisterPlugins(plugins []UIPlugin) error {
	//initialize temporary maps
	registered := map[string]bool{}
	includesWithPair := map[PageScope][]pluginTemplatePair{}
	panelHTMLWithPair := map[PageScope]map[pagePosition][]pluginTemplatePair{
		TaskPage:    map[pagePosition][]pluginTemplatePair{},
		BuildPage:   map[pagePosition][]pluginTemplatePair{},
		VersionPage: map[pagePosition][]pluginTemplatePair{},
	}
	dataFuncs := map[PageScope]map[string]UIDataFunction{
		TaskPage:    map[string]UIDataFunction{},
		BuildPage:   map[string]UIDataFunction{},
		VersionPage: map[string]UIDataFunction{},
	}

	for _, p := range plugins {
		// don't register plugins twice
		if registered[p.Name()] {
			return fmt.Errorf("plugin '%v' already registered", p.Name())
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
					return fmt.Errorf("plugin '%v': cannot register ui panel without a Page")
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
						return fmt.Errorf(
							"a data function is already registered for plugin %v on %v page",
							p.Name(), panel.Page)
					}
				}
			}
		} else if err != nil {
			return fmt.Errorf("GetPanelConfig for plugin '%v' returned an error: %v", p.Name(), err)
		}

		registered[p.Name()] = true
	}

	// sort registered plugins by name and cache their HTML
	self.includes = map[PageScope][]template.HTML{
		TaskPage:    sortAndExtractHTML(includesWithPair[TaskPage]),
		BuildPage:   sortAndExtractHTML(includesWithPair[BuildPage]),
		VersionPage: sortAndExtractHTML(includesWithPair[VersionPage]),
	}
	self.panelHTML = map[PageScope]PanelLayout{
		TaskPage: PanelLayout{
			Left:   sortAndExtractHTML(panelHTMLWithPair[TaskPage][PageLeft]),
			Right:  sortAndExtractHTML(panelHTMLWithPair[TaskPage][PageRight]),
			Center: sortAndExtractHTML(panelHTMLWithPair[TaskPage][PageCenter]),
		},
		BuildPage: PanelLayout{
			Left:   sortAndExtractHTML(panelHTMLWithPair[BuildPage][PageLeft]),
			Right:  sortAndExtractHTML(panelHTMLWithPair[BuildPage][PageRight]),
			Center: sortAndExtractHTML(panelHTMLWithPair[BuildPage][PageCenter]),
		},
		VersionPage: PanelLayout{
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
					err = fmt.Errorf("plugin function panicked: %v", r)
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
	*errs = append(*errs, fmt.Errorf("{'%v': %v}", name, err))
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

// StaticWebRootFromSourceFile is a helper for quickly grabbing
// the folder of the source file that calls it. This makes it
// very easy to establish a static root for a plugin, by simply
// declaring:
//  {StaticRoot: plugin.StaticWebRootFromSourceFile()}
// in a plugin's PanelConfig definition.
func StaticWebRootFromSourceFile() string {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("Error finding static web root from source file")
	}
	return path.Join(path.Dir(file), "static/")
}
