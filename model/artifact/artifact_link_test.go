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
		{name: "S3VirtualHostedURLShouldValidate", link: "https://bucket.s3.amazonaws.com/report.html"},
		{name: "HTTPSAssociatedLinkShouldValidate", link: "https://example.com/coverage.html", associated: true},
		{name: "MixedCaseHTTPShouldValidate", link: "HtTpS://example.com/report.html"},
		{name: "SingleLabelHostShouldValidate", link: "http://fileserver/report.html"},
		{name: "IPv6HostShouldValidate", link: "http://[::1]:8080/report.html"},
		{
			name:        "S3PresignedQueryAndFragmentShouldValidate",
			link:        "https://bucket.s3.amazonaws.com/report.html?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=key%2F20261028%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Security-Token=token%2Bvalue%3D&X-Amz-Signature=signature&X-Amz-Expires=3600#details",
			doNotEncode: true,
		},
		{name: "S3EscapedObjectKeyShouldValidate", link: "https://bucket.s3.amazonaws.com/report%2Ffile.html"},
		{name: "S3RawObjectKeyShouldValidate", link: "https://bucket.s3.amazonaws.com/file #1%+?.txt"},
		{name: "S3PathStyleDottedBucketAndRawPercentDirectoryShouldValidate", link: "https://s3.amazonaws.com/bucket.with.dots/coverage/100%/report.html"},
		{name: "T2MainLinkShouldValidate", link: "t2://internal.example.com/report"},
		{name: "T2AssociatedLinkShouldValidate", link: "t2://internal.example.com/report", associated: true},
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

func TestRemoveInvalidLinks(t *testing.T) {
	t.Run("AllValidLinksShouldBeKept", func(t *testing.T) {
		files := []File{
			{Name: "Report", Link: "https://example.com/report.html", AssociatedLinks: []AssociatedLink{
				{Name: "Coverage", Link: "t2://internal.example.com/coverage"},
			}},
			{Name: "Log", Link: "http://fileserver/log.txt"},
		}
		valid, err := RemoveInvalidLinks(files)
		assert.NoError(t, err)
		assert.Equal(t, files, valid)
	})
	t.Run("InvalidLinkShouldRemoveOnlyThatArtifact", func(t *testing.T) {
		valid, err := RemoveInvalidLinks([]File{
			{Name: "Report", Link: "https://example.com/report.html"},
			{Name: "Unsafe", Link: "javascript:alert(document.domain)", AssociatedLinks: []AssociatedLink{
				{Name: "Coverage", Link: "https://example.com/coverage.html"},
			}},
			{Name: "Log", Link: "https://example.com/log.txt"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating link for artifact 'Unsafe'")
		assert.Equal(t, []File{
			{Name: "Report", Link: "https://example.com/report.html"},
			{Name: "Log", Link: "https://example.com/log.txt"},
		}, valid)
	})
	t.Run("InvalidAssociatedLinkShouldRemoveOnlyThatAssociatedLink", func(t *testing.T) {
		valid, err := RemoveInvalidLinks([]File{
			{Name: "Report", Link: "https://example.com/report.html", AssociatedLinks: []AssociatedLink{
				{Name: "Unsafe", Link: "javascript:alert(document.domain)"},
				{Name: "Coverage", Link: "https://example.com/coverage.html"},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating associated link 'Unsafe' for artifact 'Report'")
		assert.Equal(t, []File{
			{Name: "Report", Link: "https://example.com/report.html", AssociatedLinks: []AssociatedLink{
				{Name: "Coverage", Link: "https://example.com/coverage.html"},
			}},
		}, valid)
	})
	t.Run("MultipleInvalidLinksShouldAllBeReported", func(t *testing.T) {
		valid, err := RemoveInvalidLinks([]File{
			{Name: "First", Link: "s3://bucket/first.txt"},
			{Name: "Second"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "artifact 'First'")
		assert.Contains(t, err.Error(), "artifact 'Second'")
		assert.Empty(t, valid)
	})
}
