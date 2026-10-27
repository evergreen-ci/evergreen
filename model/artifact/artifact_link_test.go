package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFiles(t *testing.T) {
	for _, tc := range []struct {
		name        string
		link        string
		wantErr     bool
		associated  bool
		doNotEncode bool
	}{
		{name: "HTTPSShouldValidate", link: "https://example.com/report.html"},
		{name: "HTTPSAssociatedLinkShouldValidate", link: "https://example.com/coverage.html", associated: true},
		{name: "MixedCaseHTTPShouldValidate", link: "HtTpS://example.com/report.html"},
		{name: "SingleLabelHostShouldValidate", link: "http://fileserver/report.html"},
		{name: "IPv6HostShouldValidate", link: "http://[::1]:8080/report.html"},
		{name: "QueryAndFragmentShouldValidate", link: "https://example.com/report?task=123&build=456#details"},
		{name: "EscapedFilenameShouldValidate", link: "https://example.com/report%2Ffile.html"},
		{name: "SpecialFilenameShouldValidate", link: "https://example.com/file#1%.txt"},
		{name: "RawPercentInDirectoryShouldValidate", link: "https://example.com/coverage/100%/report.html"},
		{name: "T2MainLinkShouldError", link: "t2://internal.example.com/report.ftdc", wantErr: true},
		{name: "T2AssociatedLinkShouldValidate", link: "t2://internal.example.com/report.ftdc", associated: true},
		{name: "JavaScriptShouldError", link: "javascript:alert(document.domain)", wantErr: true},
		{name: "UnencodedJavaScriptShouldError", link: "javascript:alert(document.domain)", wantErr: true, doNotEncode: true},
		{name: "AssociatedJavaScriptShouldError", link: "javascript:alert(document.domain)//%2F", wantErr: true, associated: true},
		{name: "UnencodedAssociatedJavaScriptShouldError", link: "javascript:alert(document.domain)", wantErr: true, associated: true, doNotEncode: true},
		{name: "EscapedJavaScriptShouldError", link: "javascript:alert(document.domain)//%2F", wantErr: true},
		{name: "JavaScriptWithAuthorityAndRawPercentPathShouldError", link: "javascript://example.com/100%/report.html", wantErr: true},
		{name: "MixedCaseJavaScriptShouldError", link: "JaVaScRiPt:alert(document.domain)//%2F", wantErr: true},
		{name: "PaddedJavaScriptShouldError", link: " \tjavascript:alert(document.domain)//%2F \r\n", wantErr: true},
		{name: "ControlCharacterInSchemeShouldError", link: "java\nscript:alert(document.domain)//%2F", wantErr: true},
		{name: "PaddedHTTPSShouldError", link: " https://example.com/report.html ", wantErr: true},
		{name: "DataShouldError", link: "data:text/html,<script>alert(1)</script>", wantErr: true},
		{name: "VBScriptShouldError", link: "vbscript:msgbox(1)", wantErr: true},
		{name: "FileShouldError", link: "file:///etc/passwd", wantErr: true},
		{name: "S3ShouldError", link: "s3://bucket/report.html", wantErr: true},
		{name: "RelativeURLShouldError", link: "/report.html", wantErr: true},
		{name: "ProtocolRelativeURLShouldError", link: "//example.com/report.html", wantErr: true},
		{name: "MissingHostShouldError", link: "https:///report.html", wantErr: true},
		{name: "OpaqueHTTPSShouldError", link: "https:report.html", wantErr: true},
		{name: "MalformedHostShouldError", link: "https://[invalid/report.html", wantErr: true},
		{name: "EmptyURLShouldError", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := File{Name: "Report", Link: tc.link, DoNotEncodeLink: tc.doNotEncode}
			if tc.associated {
				file.Link = "https://example.com/report.html"
				file.DoNotEncodeLink = true
				file.AssociatedLinks = []AssociatedLink{{Name: "Coverage", Link: tc.link, DoNotEncodeLink: tc.doNotEncode}}
			}
			err := ValidateFiles([]File{file})
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "artifact 'Report'")
				if tc.associated {
					assert.Contains(t, err.Error(), "associated link 'Coverage'")
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
