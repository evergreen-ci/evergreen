package plugin_test

import (
	"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/smartystreets/goconvey/convey"
	"html/template"
	"net/http"
	"testing"
)

// ===== Mock UI Plugin =====

// simple plugin type that has a name and ui config
type MockUIPlugin struct {
	NickName string
	Conf     *plugin.PanelConfig
}

func (self *MockUIPlugin) Name() string {
	return self.NickName
}
func (self *MockUIPlugin) GetAPIHandler() http.Handler {
	return nil
}
func (self *MockUIPlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *MockUIPlugin) Configure(conf map[string]interface{}) error {
	return nil
}

func (self *MockUIPlugin) NewCommand(commandName string) (plugin.Command, error) {
	return nil, nil
}
func (self *MockUIPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return self.Conf, nil
}

// ===== Tests =====

func TestPanelManagerRegistration(t *testing.T) {
	var ppm plugin.PanelManager
	Convey("With a simple plugin panel manager", t, func() {
		ppm = &plugin.SimplePanelManager{}

		Convey("and a registered set of test plugins without panels", func() {
			uselessPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "no_ui_config",
					Conf:     nil,
				},
				&MockUIPlugin{
					NickName: "config_with_no_panels",
					Conf:     &plugin.PanelConfig{},
				},
			}
			err := ppm.RegisterPlugins(uselessPlugins)
			So(err, ShouldBeNil)

			Convey("no ui panel data should be returned for any scope", func() {
				data, err := ppm.UIData(plugin.UIContext{}, plugin.TaskPage)
				So(err, ShouldBeNil)
				So(data["no_ui_config"], ShouldBeNil)
				So(data["config_with_no_panels"], ShouldBeNil)
				data, err = ppm.UIData(plugin.UIContext{}, plugin.BuildPage)
				So(err, ShouldBeNil)
				So(data["no_ui_config"], ShouldBeNil)
				So(data["config_with_no_panels"], ShouldBeNil)
				data, err = ppm.UIData(plugin.UIContext{}, plugin.VersionPage)
				So(err, ShouldBeNil)
				So(data["no_ui_config"], ShouldBeNil)
				So(data["config_with_no_panels"], ShouldBeNil)
			})
		})

		Convey("registering a plugin panel with no page should fail", func() {
			badPanelPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "bad_panel",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{PanelHTML: "<marquee> PANEL </marquee>"},
						},
					},
				},
			}
			err := ppm.RegisterPlugins(badPanelPlugins)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "Page")
		})

		Convey("registering the same plugin name twice should fail", func() {
			conflictingPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "a",
					Conf:     nil,
				},
				&MockUIPlugin{
					NickName: "a",
					Conf:     &plugin.PanelConfig{},
				},
			}
			err := ppm.RegisterPlugins(conflictingPlugins)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "already")
		})

		Convey("registering more than one data function to the same page "+
			"for the same plugin should fail", func() {
			dataPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "data_function_fan",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page: plugin.TaskPage,
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return 100, nil
								}},
							{
								Page: plugin.TaskPage,
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return nil, fmt.Errorf("this function just errors")
								}},
						},
					},
				},
			}
			err := ppm.RegisterPlugins(dataPlugins)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "function is already registered")
		})
	})
}

func TestPanelManagerRetrieval(t *testing.T) {
	var ppm plugin.PanelManager

	Convey("With a simple plugin panel manager", t, func() {
		ppm = &plugin.SimplePanelManager{}

		Convey("and a registered set of test plugins with panels", func() {
			// These 3 plugins exist to check the sort output of the manager.
			// For consistency, plugin panels and includes are ordered by plugin name
			// and then by the order of their declaration in the Panels array.
			// This test asserts that the panels in A come before B which come
			// before C, even though they are not in the plugin array in that order.
			testPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "A_the_first_letter",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								Includes: []template.HTML{
									"0",
									"1",
								},
								PanelHTML: "0",
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return 1000, nil
								},
							},
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								Includes: []template.HTML{
									"2",
									"3",
								},
								PanelHTML: "1",
							},
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageLeft,
								Includes: []template.HTML{
									"4",
								},
								PanelHTML: "X",
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "C_the_third_letter",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								Includes: []template.HTML{
									"7",
									"8",
								},
								PanelHTML: "3",
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return 2112, nil
								},
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "B_the_middle_letter",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								Includes: []template.HTML{
									"5",
								},
								PanelHTML: "2",
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return 1776, nil
								},
							},
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageLeft,
								Includes: []template.HTML{
									"6",
								},
								PanelHTML: "Z",
							},
						},
					},
				},
			}

			err := ppm.RegisterPlugins(testPlugins)
			So(err, ShouldBeNil)

			Convey("retrieved includes for the task page should be in correct "+
				"stable alphabetical order by plugin name", func() {
				includes, err := ppm.Includes(plugin.TaskPage)
				So(err, ShouldBeNil)
				So(includes, ShouldNotBeNil)

				// includes == [0 1 2 ... ]
				for i := 1; i < len(includes); i++ {
					So(includes[i], ShouldBeGreaterThan, includes[i-1])
				}
			})
			Convey("retrieved panel HTML for the task page should be in correct "+
				"stable alphabetical order by plugin name", func() {
				panels, err := ppm.Panels(plugin.TaskPage)
				So(err, ShouldBeNil)
				So(len(panels.Right), ShouldEqual, 0)
				So(len(panels.Left), ShouldBeGreaterThan, 0)
				So(len(panels.Center), ShouldBeGreaterThan, 0)

				// left == [X Z]
				So(panels.Left[0], ShouldBeLessThan, panels.Left[1])

				// center == [0 1 2 3]
				for i := 1; i < len(panels.Center); i++ {
					So(panels.Center[i], ShouldBeGreaterThan, panels.Center[i-1])
				}
			})
			Convey("data functions populate the results map with their return values", func() {
				uiData, err := ppm.UIData(plugin.UIContext{}, plugin.TaskPage)
				So(err, ShouldBeNil)
				So(len(uiData), ShouldBeGreaterThan, 0)
				So(uiData["A_the_first_letter"], ShouldEqual, 1000)
				So(uiData["B_the_middle_letter"], ShouldEqual, 1776)
				So(uiData["C_the_third_letter"], ShouldEqual, 2112)
			})
		})
	})
}

func TestPluginUIDataFunctionErrorHandling(t *testing.T) {
	var ppm plugin.PanelManager

	Convey("With a simple plugin panel manager", t, func() {
		ppm = &plugin.SimplePanelManager{}

		Convey("and a set of plugins, some with erroring data functions", func() {
			errorPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "error1",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return nil, fmt.Errorf("Error #1")
								},
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "error2",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(context plugin.UIContext) (interface{}, error) {
									return nil, fmt.Errorf("Error #2")
								},
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "error3 not found",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(_ plugin.UIContext) (interface{}, error) {
									return nil, fmt.Errorf("Error")
								},
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "good",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(_ plugin.UIContext) (interface{}, error) {
									return "fine", nil
								},
							},
						},
					},
				},
			}
			err := ppm.RegisterPlugins(errorPlugins)
			So(err, ShouldBeNil)
			data, err := ppm.UIData(plugin.UIContext{}, plugin.TaskPage)
			So(err, ShouldNotBeNil)

			Convey("non-broken functions should succeed", func() {
				So(data["good"], ShouldEqual, "fine")
			})

			Convey("and reasonable error messages should be produced for failures", func() {
				So(err.Error(), ShouldContainSubstring, "error1")
				So(err.Error(), ShouldContainSubstring, "Error #1")
				So(err.Error(), ShouldContainSubstring, "error2")
				So(err.Error(), ShouldContainSubstring, "Error #2")
				So(err.Error(), ShouldContainSubstring, "error3")
			})
		})
		Convey("and a plugin that panics", func() {
			errorPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "busted",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(_ plugin.UIContext) (interface{}, error) {
									panic("BOOM")
									return "like perfection I am unreachable", nil
								},
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "good",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(_ plugin.UIContext) (interface{}, error) {
									return "still fine", nil
								},
							},
						},
					},
				},
			}

			Convey("reasonable error messages should be produced", func() {
				err := ppm.RegisterPlugins(errorPlugins)
				So(err, ShouldBeNil)
				data, err := ppm.UIData(plugin.UIContext{}, plugin.TaskPage)
				So(err, ShouldNotBeNil)
				So(data["good"], ShouldEqual, "still fine")
				So(err.Error(), ShouldContainSubstring, "panic")
				So(err.Error(), ShouldContainSubstring, "BOOM")
				So(err.Error(), ShouldContainSubstring, "busted")
			})
		})
	})
}

func TestUIDataInjection(t *testing.T) {
	var ppm plugin.PanelManager

	Convey("With a simple plugin panel manager", t, func() {
		ppm = &plugin.SimplePanelManager{}

		Convey("and a registered set of test plugins with injection needs", func() {
			funcPlugins := []plugin.UIPlugin{
				&MockUIPlugin{
					NickName: "combine",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(ctx plugin.UIContext) (interface{}, error) {
									return ctx.Task.Id + ctx.Build.Id + ctx.Version.Id, nil
								},
							},
						},
					},
				},
				&MockUIPlugin{
					NickName: "userhttpapiserver",
					Conf: &plugin.PanelConfig{
						Panels: []plugin.UIPanel{
							{
								Page:     plugin.TaskPage,
								Position: plugin.PageCenter,
								DataFunc: func(ctx plugin.UIContext) (interface{}, error) {
									return fmt.Sprintf("%v.%v@%v", ctx.User.Email, ctx.Settings.ApiUrl), nil
								},
							},
						},
					},
				},
			}
			err := ppm.RegisterPlugins(funcPlugins)
			So(err, ShouldBeNil)
		})
	})
}

func TestUserInjection(t *testing.T) {
	Convey("With a dbUser and a request", t, func() {
		u := &user.DBUser{Id: "name1"}
		r, err := http.NewRequest("GET", "/", bytes.NewBufferString("{}"))
		So(err, ShouldBeNil)

		Convey("the user should possible to set and retrieve", func() {
			plugin.SetUser(u, r)
			So(plugin.GetUser(r), ShouldResemble, u)
		})
	})
}
