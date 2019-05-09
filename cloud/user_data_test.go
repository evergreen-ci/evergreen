package cloud

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteUserDataHeaders(t *testing.T) {
	buf := &bytes.Buffer{}
	boundary := "some_boundary"
	require.NoError(t, writeUserDataHeaders(buf, boundary))
	res := strings.ToLower(buf.String())
	assert.Contains(t, res, "mime-version: 1.0")
	assert.Contains(t, res, "content-type: multipart/mixed")
	assert.Contains(t, res, fmt.Sprintf("boundary=\"%s\"", boundary))
	assert.Equal(t, 1, strings.Count(res, boundary))
}

func TestParseUserDataContentType(t *testing.T) {
	for _, userData := range []string{
		"#!/bin/bash\necho 'foobar'",
		"#include\nhttps://example.com/foobar.txt",
		"#cloud-config\nruncmd:\n  - echo 'foobar'",
		"#upstart-job\ndescription: \"foobar\"",
		"#cloud-boothook\necho 'foobar'",
		"#part-handler\ndef list_types():\nreturn(['foobar'])\ndef handle_part(data,ctype,filename,payload):\nprint 'foobar'\nreturn",
	} {
		contentType, err := parseUserDataContentType(userData)
		require.NoError(t, err)
		assert.NotEmpty(t, contentType)
	}
	_, err := parseUserDataContentType("foo\nbar")
	assert.Error(t, err)
}

func TestWriteUserDataPart(t *testing.T) {
	buf := &bytes.Buffer{}
	mimeWriter := multipart.NewWriter(buf)
	boundary := "some_boundary"
	require.NoError(t, mimeWriter.SetBoundary(boundary))

	userData := "#!/bin/bash\necho 'foobar'"
	require.NoError(t, writeUserDataPart(mimeWriter, userData, "foobar.txt"))

	res := strings.ToLower(buf.String())
	assert.Contains(t, res, "mime-version: 1.0")
	assert.Contains(t, res, "content-type: text/x-shellscript")
	assert.Contains(t, res, "content-disposition: attachment; filename=\"foobar.txt\"")
	assert.Contains(t, res, userData)
	assert.Equal(t, 1, strings.Count(res, boundary))
}

func TestWriteUserDataPartDefaultForUnrecognizedFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	mimeWriter := multipart.NewWriter(buf)
	userData := "this user data has no cloud-init directive"
	require.NoError(t, writeUserDataPart(mimeWriter, userData, "foo.txt"))
	assert.Contains(t, buf.String(), "Content-Type: text/x-shellscript")
}

func TestWriteUserDataPartEmptyFileName(t *testing.T) {
	buf := &bytes.Buffer{}
	mimeWriter := multipart.NewWriter(buf)
	userData := "#!/bin/bash\necho 'foobar'"
	assert.Error(t, writeUserDataPart(mimeWriter, userData, ""))
}

func TestMakeMultipartUserData(t *testing.T) {
	userData := "#!/bin/bash\necho 'foobar'"
	noUserData := ""
	fileOne := "1.txt"
	fileTwo := "2.txt"

	res, err := makeMultipartUserData(map[string]string{})
	require.NoError(t, err)
	assert.NotEmpty(t, res)

	res, err = makeMultipartUserData(map[string]string{
		fileOne: noUserData,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res)
	assert.False(t, strings.Contains(res, fileOne))

	res, err = makeMultipartUserData(map[string]string{
		fileOne: userData,
		fileTwo: userData,
	})
	require.NoError(t, err)
	assert.Contains(t, res, fileOne)
	assert.Contains(t, res, fileTwo)
	assert.Equal(t, 2, strings.Count(res, userData))

	res, err = makeMultipartUserData(map[string]string{
		fileOne: noUserData,
		fileTwo: userData,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, res)
	assert.False(t, strings.Contains(res, fileOne))
	assert.Contains(t, res, fileTwo)
	assert.Contains(t, res, userData)
}
