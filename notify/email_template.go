package notify

import (
	"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/web"
	"path/filepath"
)

const (
	TemplatePath = "notify/templates"
	BaseTemplate = "layout.html"
)

func createEnvironment(mciSettings *evergreen.MCISettings, globals map[string]interface{}) (*web.App, error) {
	home, err := evergreen.FindMCIHome()
	if err != nil {
		return nil, err
	}
	templateHome := filepath.Join(home, TemplatePath)

	funcs, err := web.MakeCommonFunctionMap(mciSettings)
	if err != nil {
		return nil, fmt.Errorf("error creating templating functions: %v", err)
	}
	// Overwrite globals
	funcs["Global"] = func(input string) interface{} {
		return globals[input]
	}

	app := web.NewApp()
	app.TemplateFuncs = funcs
	app.TemplateFolder = templateHome
	app.CacheTemplates = true
	return app, nil
}

func TemplateEmailBody(ae *web.App, name string, data interface{}) (string, error) {
	templateResponse := ae.RespondTemplate(
		[]string{name, "email_layout_base.html"},
		"base",
		data).(*web.TemplateResponse)
	var buf bytes.Buffer
	err := templateResponse.TemplateSet.ExecuteTemplate(&buf, templateResponse.TemplateName,
		templateResponse.Data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
