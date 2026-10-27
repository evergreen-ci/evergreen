package artifact

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateFiles(t *testing.T) {
	for _, tc := range []struct {
		name           string
		link           string
		wantErr        bool
		associatedOnly bool
	}{
		{name: "HTTPSShouldValidate", link: "https://example.com/report.html"},
		{name: "MixedCaseHTTPShouldValidate", link: "HtTpS://example.com/report.html"},
		{name: "SingleLabelHostShouldValidate", link: "http://fileserver/report.html"},
		{name: "IPv6HostShouldValidate", link: "http://[::1]:8080/report.html"},
		{name: "QueryAndFragmentShouldValidate", link: "https://example.com/report?task=123&build=456#details"},
		{name: "EscapedFilenameShouldValidate", link: "https://example.com/report%2Ffile.html"},
		{name: "SpecialFilenameShouldValidate", link: "https://example.com/file#1%.txt"},
		{name: "RawPercentInDirectoryShouldValidate", link: "https://example.com/coverage/100%/report.html"},
		{name: "T2ShouldValidateOnlyForAssociatedLinks", link: "t2://internal.example.com/report.ftdc", associatedOnly: true},
		{name: "JavaScriptShouldError", link: "javascript:alert(document.domain)", wantErr: true},
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
			for _, doNotEncode := range []bool{false, true} {
				encodingName := "EncodeLinks"
				if doNotEncode {
					encodingName = "DoNotEncodeLinks"
				}
				t.Run(encodingName, func(t *testing.T) {
					for _, associated := range []bool{false, true} {
						fieldName := "MainLink"
						if associated {
							fieldName = "AssociatedLink"
						}
						t.Run(fieldName, func(t *testing.T) {
							file := File{Name: "Report", Link: "https://example.com/report.html", DoNotEncodeLink: doNotEncode}
							if associated {
								file.AssociatedLinks = []AssociatedLink{{Name: "Coverage", Link: tc.link, DoNotEncodeLink: doNotEncode}}
							} else {
								file.Link = tc.link
							}
							err := ValidateFiles([]File{file})
							if tc.wantErr || (tc.associatedOnly && !associated) {
								require.Error(t, err)
								assert.Contains(t, err.Error(), "artifact 'Report'")
								if associated {
									assert.Contains(t, err.Error(), "associated link 'Coverage'")
								}
							} else {
								assert.NoError(t, err)
							}
						})
					}
				})
			}
		})
	}
}

func TestValidateSignedArtifactWithRawPercentInDirectory(t *testing.T) {
	file := File{
		Name:       "Coverage",
		Link:       "https://bucket.s3.amazonaws.com/coverage/100%/report.html",
		Bucket:     "bucket",
		FileKey:    "coverage/100%/report.html",
		Visibility: Signed,
	}
	require.NoError(t, ValidateFiles([]File{file}))
	stored := EscapeFiles([]File{file})
	require.Len(t, stored, 1)
	assert.Equal(t, file.Link, stored[0].Link)

	signedURL, err := PresignFile(t.Context(), stored[0], func(context.Context, File) (*Credentials, error) {
		return &Credentials{AWSKey: "AKIAFAKEKEY", AWSSecret: "fake-secret"}, nil
	})
	require.NoError(t, err)
	parsed, err := url.Parse(signedURL)
	require.NoError(t, err)
	assert.Equal(t, "/coverage/100%25/report.html", parsed.EscapedPath())
}
