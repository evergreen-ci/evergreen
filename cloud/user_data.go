package cloud

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// This file contains utilities to support cloud-init user data passed to
// providers to configure launch.

// directiveToContentType maps a cloud-init directive to its MIME content type.
func directiveToContentType() map[string]string {
	return map[string]string{
		"#!":              "text/x-shellscript",
		"#include":        "text/x-include-url",
		"#cloud-config":   "text/cloud-config",
		"#upstart-job":    "text/upstart-job",
		"#cloud-boothook": "text/cloud-boothook",
		"#part-handler":   "text/part-handler",
		"<powershell>":    "text/x-shellscript",
		"<script>":        "text/x-shellscript",
	}
}

// makeMultipartUserData returns user data in a multipart MIME format with the
// given files and their content.
func makeMultipartUserData(files map[string]string) (string, error) {
	buf := &bytes.Buffer{}
	parts := multipart.NewWriter(buf)

	if err := writeUserDataHeaders(buf, parts.Boundary()); err != nil {
		return "", errors.Wrap(err, "error writing MIME headers")
	}

	for fileName, content := range files {
		if err := writeUserDataPart(parts, content, fileName); err != nil {
			return "", errors.Wrapf(err, "error writing user data '%s'", fileName)
		}
	}

	if err := parts.Close(); err != nil {
		return "", errors.Wrap(err, "error closing MIME writer")
	}

	return buf.String(), nil
}

// writeUserDataHeaders writes the multipart MIME headers for user data.
func writeUserDataHeaders(writer io.Writer, boundary string) error {
	topLevelHeaders := textproto.MIMEHeader{}
	topLevelHeaders.Add("MIME-Version", "1.0")
	topLevelHeaders.Add("Content-Type", fmt.Sprintf("multipart/mixed; boundary=\"%s\"", boundary))

	for key := range topLevelHeaders {
		header := fmt.Sprintf("%s: %s", key, topLevelHeaders.Get(key))
		if _, err := writer.Write([]byte(header + "\r\n")); err != nil {
			return errors.Wrapf(err, "error writing top-level header '%s'", header)
		}
	}

	if _, err := writer.Write([]byte("\r\n")); err != nil {
		return errors.Wrap(err, "error writing top-level header line break")
	}

	return nil
}

// writeUserDataPart creates a part in the user data multipart with the given
// contents and name.
func writeUserDataPart(writer *multipart.Writer, userDataPart, fileName string) error {
	if userDataPart == "" {
		return nil
	}
	if fileName == "" {
		return errors.New("user data file name cannot be empty")
	}

	contentType, err := parseUserDataContentType(userDataPart)
	if err != nil {
		grip.Warning(errors.Wrap(err, "error determining user data content type"))
		contentType = directiveToContentType()["#!"]
	}

	header := textproto.MIMEHeader{}
	header.Add("MIME-Version", "1.0")
	header.Add("Content-Type", contentType)
	header.Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))

	part, err := writer.CreatePart(header)
	if err != nil {
		return errors.Wrap(err, "error making custom user data part")
	}

	if _, err := part.Write([]byte(userDataPart)); err != nil {
		return errors.Wrap(err, "error writing custom user data")
	}

	return nil
}

// parseUserDataContentType detects the content type based on the directive on
// the first line of the user data.
func parseUserDataContentType(userData string) (string, error) {
	var firstLine string
	index := strings.IndexByte(userData, '\n')
	if index == -1 {
		firstLine = userData
	} else {
		firstLine = userData[:index]
	}
	firstLine = strings.TrimSpace(firstLine)

	for directive, contentType := range directiveToContentType() {
		if strings.HasPrefix(firstLine, directive) {
			return contentType, nil
		}
	}
	return "", errors.Errorf("user data format is not recognized from first line: '%s'", firstLine)
}
