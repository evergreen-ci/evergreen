package gimlet

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RenderSuite struct {
	render      Renderer
	opts        RendererOptions
	constructor func(RendererOptions) Renderer
	testData    interface{}
	expected    string
	suite.Suite
}

func TestHTMLRenderSuite(t *testing.T) {
	s := new(RenderSuite)
	s.constructor = func(opts RendererOptions) Renderer {
		return NewHTMLRenderer(opts)
	}
	suite.Run(t, s)
}

func TestTextRenderSuite(t *testing.T) {
	s := new(RenderSuite)
	s.constructor = func(opts RendererOptions) Renderer {
		return NewTextRenderer(opts)
	}
	suite.Run(t, s)
}

func (s *RenderSuite) SetupSuite() {
	s.expected = `<html><body><div>menu</div><p>hello Socrates setarcoS</p></body></html>`
	s.testData = struct {
		Name string
	}{"Socrates"}
	s.opts = RendererOptions{
		Directory: "testdata",
		Functions: map[string]interface{}{
			"Reverse": func(in string) string {
				runes := []rune(in)
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return string(runes)
			},
		},
	}

}
func (s *RenderSuite) SetupTest() {
	s.render = s.constructor(s.opts)

}

func (s *RenderSuite) TestBaseHTML() {
	for i := 0; i < 10; i++ {
		out := &bytes.Buffer{}
		err := s.render.Render(out, s.testData, "base", "test1.html", "test2.html")

		s.NoError(err)
		s.Equal(s.expected, out.String())
	}

	if r, ok := s.render.(*htmlRenderer); ok {
		for k, v := range r.cache {
			out := &bytes.Buffer{}
			err := v.ExecuteTemplate(out, "base", s.testData)
			s.NoError(err)
			r.cache[k] = v
		}

		out := &bytes.Buffer{}
		err := s.render.Render(out, s.testData, "base", "test1.html", "test2.html")
		s.Error(err)
	}
}

func (s *RenderSuite) TestBadTemplates() {
	out := &bytes.Buffer{}
	err := s.render.Render(out, s.testData, "base", "invalid_template.html")
	s.Error(err)

	err = s.render.Render(out, s.testData, "base", "template_does_not_exist.html")
	s.Error(err)

	err = s.render.Render(out, s.testData, "base", "badtemplate.html")
	s.Error(err)

	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	s.Require().NoError(err)

	htmlHandler := func(w http.ResponseWriter, r *http.Request) {
		s.render.WriteResponse(w, http.StatusOK, s.testData, "badtemplate.html")
	}

	w := httptest.NewRecorder()
	htmlHandler(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
}

func (s *RenderSuite) TestWriteHTTP() {
	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	s.Require().NoError(err)

	/* Test a handler that writes a rendered HTML template */
	w := httptest.NewRecorder()
	htmlHandler := func(w http.ResponseWriter, r *http.Request) {
		s.render.WriteResponse(w, http.StatusOK, s.testData, "base", "test1.html", "test2.html")
	}
	htmlHandler(w, req)

	s.Equal(s.expected, w.Body.String())
	s.Equal(http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	s.render.Stream(w, http.StatusOK, s.testData, "base", "test1.html", "test2.html")
	s.Equal(http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	s.render.Stream(w, http.StatusOK, s.testData, "base", "test1.html", "test2.html")
	s.Equal(http.StatusOK, w.Code)
}

func (s *RenderSuite) TestRenderingErrorToResponse() {
	w := httptest.NewRecorder()
	s.render.Stream(w, http.StatusOK, s.testData, "base2", "test1.html", "test2.html2")
	s.Equal(http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	s.render.Stream(w, http.StatusOK, s.testData, "base2", "test1.html", "test2.html2")
	s.Equal(http.StatusOK, w.Code)
}
