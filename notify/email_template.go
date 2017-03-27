package notify

import (
	"bytes"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/web"
	"github.com/pkg/errors"
)

const (
	TemplatePath = "notify/templates"
	BaseTemplate = "layout.html"
)

func createEnvironment(settings *evergreen.Settings, globals map[string]interface{}) (*web.App, error) {
	home := evergreen.FindEvergreenHome()
	templateHome := filepath.Join(home, TemplatePath)

	funcs, err := web.MakeCommonFunctionMap(settings)
	if err != nil {
		return nil, errors.Wrap(err, "error creating templating functions")
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
	templateResponse, ok := ae.RespondTemplate(
		[]string{name, "email_layout_base.html"},
		"base",
		data).(*web.TemplateResponse)

	if !ok {
		return "", errors.Errorf("problem converting template response for %s with data type %T",
			name, data)
	}

	var buf bytes.Buffer

	err := templateResponse.TemplateSet.ExecuteTemplate(&buf, templateResponse.TemplateName,
		templateResponse.Data)

	if err != nil {
		return "", errors.WithStack(err)
	}

	return buf.String(), nil
}
