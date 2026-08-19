package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFiles(t *testing.T) {
	for testName, testCase := range map[string]struct {
		files []File
		want  bool
	}{
		"AllowsHTTPAndHTTPSLinks": {
			files: []File{
				{
					Link: "http://example.com/artifact.txt",
					AssociatedLinks: []AssociatedLink{
						{Link: "https://example.com/report.html"},
					},
				},
			},
			want: true,
		},
		"AllowsEmptyLinks": {
			files: []File{{}},
			want:  true,
		},
		"RejectsUnsafeArtifactLinkSchemes": {
			files: []File{
				{Link: "javascript:alert(document.domain)"},
				{Link: "data:text/html,<script>alert(1)</script>"},
				{Link: "vbscript:msgbox(1)"},
			},
		},
		"RejectsUnsafeAssociatedArtifactLinkScheme": {
			files: []File{
				{
					Link: "https://example.com/artifact.txt",
					AssociatedLinks: []AssociatedLink{
						{Link: "javascript:alert(document.domain)"},
					},
				},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			err := ValidateFiles(testCase.files)
			if testCase.want {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
