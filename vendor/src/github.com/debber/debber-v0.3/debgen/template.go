/*
   Copyright 2013 Am Laher

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package debgen

import (
	"bytes"
	"github.com/debber/debber-v0.3/deb"
	"io/ioutil"
	"os"
	"text/template"
	"time"
)

// initialize "template data" object
func NewTemplateData(pkg *deb.Control) *TemplateData {
	//Entry date format day-of-week, dd month yyyy hh:mm:ss +zzzz
	t := time.Now()
	entryDate := t.Format(ChangelogDateLayout)
	templateVars := TemplateData{Package: pkg, EntryDate: entryDate, Checksums: nil,
		ExtraData: extraData()}
	return &templateVars
}

func extraData() map[string]interface{} {
	gpe := []string{}
	extraData := map[string]interface{}{}
	/*
		ext, ok := extraData["GoPathExtra"]
		if !ok {
			switch t := ext.(type) {
			case []string:
				gpe = t
			}
		}
	*/
	extraData["GoPathExtra"] = append(gpe, GoPathExtraDefault)
	return extraData
}

//Data for templates
type TemplateData struct {
	Package        *deb.Control
	Deb            *deb.Writer
	EntryDate      string
	ChangelogEntry string
	Checksums      *deb.Checksums
	ExtraData      map[string]interface{}
}

func TemplateFileOrString(templateFile string, templateDefault string, vars interface{}) ([]byte, error) {
	_, err := os.Stat(templateFile)
	var tplText string
	if os.IsNotExist(err) {
		tplText = templateDefault
		return TemplateString(tplText, vars)
	} else if err != nil {
		return nil, err
	} else {
		return TemplateFile(tplText, vars)
	}
}

func TemplateFile(templateFile string, vars interface{}) ([]byte, error) {
	tplBytes, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}
	tplText := string(tplBytes)
	return TemplateString(tplText, vars)
}

func TemplateString(tplText string, vars interface{}) ([]byte, error) {
	tpl, err := template.New("template").Parse(tplText)
	if err != nil {
		return nil, err
	}
	var dest bytes.Buffer
	err = tpl.Execute(&dest, vars)
	if err != nil {
		return nil, err
	}
	return dest.Bytes(), nil

}
