package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/userdata"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// This file contains utilities to support cloud-init user data passed to
// providers to configure launch.

// userData represents a single user data part for cloud-init.
type userData struct {
	userdata.Options
}

// newUserData creates a single userData part from the given options.
func newUserData(opts userdata.Options) (*userData, error) {
	u := userData{
		Options: opts,
	}
	if err := u.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid user data")
	}
	return &u, nil
}

// merge combines one user data part with another to make a single part.
func (u *userData) merge(other *userData) (*userData, error) {
	if u.Directive != other.Directive {
		return nil, errors.Errorf(
			"cannot combine user data parts of incompatible types: '%s' is incompatible with '%s'",
			u.Directive, other.Directive)
	}

	return newUserData(userdata.Options{
		Directive: u.Directive,
		Content:   strings.Join([]string{u.Content, other.Content}, "\n"),
		Persist:   u.Persist || other.Persist,
	})
}

// String returns the entire user data part constructed from its components.
func (u *userData) String() string {
	components := []string{
		string(u.Directive),
		u.Content,
	}
	if u.Directive.NeedsClosingTag() {
		components = append(components, string(u.Directive.ClosingTag()), "\n")
	}
	if u.Persist {
		components = append(components, persistTag)
	}
	return strings.Join(components, "\n")
}

// parseUserData returns the user data string as a structured user data.
func parseUserData(userData string) (*userData, error) {
	var err error
	var persist bool
	persist, userData, err = extractPersistTags(userData)
	if err != nil {
		return nil, errors.Wrap(err, "problem extracting persist tags")
	}

	var directive userdata.Directive
	directive, userData, err = extractDirective(userData)
	if err != nil {
		return nil, errors.Wrap(err, "problem extracting directive")
	}

	var closingTag userdata.ClosingTag
	if directive.NeedsClosingTag() {
		userData, err = extractClosingTag(userData, directive.ClosingTag())
		if err != nil {
			return nil, errors.Wrapf(err, "problem extracting closing tag '%s'", closingTag)
		}
	}

	return newUserData(userdata.Options{
		Directive: directive,
		Content:   userData,
		Persist:   persist,
	})
}

// extractDirective finds the directive on the first line of user data. This
// should be called after extractPersistTags in order to ensure that there is no
// persist tag before the directive line.
func extractDirective(userData string) (directive userdata.Directive, userDataWithoutDirective string, err error) {
	userData = strings.TrimSpace(userData)
	var firstLine string
	index := strings.IndexByte(userData, '\n')
	if index == -1 {
		firstLine = userData
	} else {
		firstLine = userData[:index]
	}
	firstLine = strings.TrimSpace(firstLine)

	for _, directive := range userdata.Directives() {
		if strings.HasPrefix(firstLine, string(directive)) {
			userDataWithoutDirective = strings.TrimSpace(strings.TrimPrefix(userData, firstLine))
			return userdata.Directive(firstLine), userDataWithoutDirective, nil
		}
	}
	return "", "", errors.Errorf("user data directive is missing from first line: '%s'", firstLine)
}

// extractClosingTag finds the closing tag to match the end of the user data.
func extractClosingTag(userData string, closingTag userdata.ClosingTag) (userDataWithoutClosingTag string, err error) {
	count := strings.Count(userData, string(closingTag))
	if count == 0 {
		return "", errors.Errorf("user data does not have closing tag '%s'", closingTag)
	}
	if count > 1 {
		return "", errors.Errorf("cannot have multiple occurrences of closing tag '%s'", closingTag)
	}
	userDataWithoutClosingTag = strings.TrimSpace(strings.Replace(userData, string(closingTag), "", 1))
	return userDataWithoutClosingTag, nil
}

const (
	persistTag        = "<persist>true</persist>"
	persistTagPattern = `[[:space:]]*<persist>[[:space:]]*true[[:space:]]*</persist>[[:space:]]*`
)

// extractPersistTags returns whether the user data contains persist tags and
// the user data without those tags if any are present.
func extractPersistTags(userData string) (found bool, userDataWithoutPersist string, err error) {
	persistRegexp, err := regexp.Compile(persistTagPattern)
	if err != nil {
		return false, "", errors.Wrap(err, "could not compile persist tag pattern")
	}
	userDataWithoutPersist = persistRegexp.ReplaceAllString(userData, "")
	return len(userDataWithoutPersist) != len(userData), userDataWithoutPersist, nil
}

// makeMultipartUserData returns user data in a multipart MIME format with the
// given files and their content.
func makeMultipartUserData(files map[string]*userData) (string, error) {
	buf := &bytes.Buffer{}
	parts := multipart.NewWriter(buf)

	if err := writeUserDataHeaders(buf, parts.Boundary()); err != nil {
		return "", errors.Wrap(err, "error writing MIME headers")
	}

	for fileName, userData := range files {
		if err := writeUserDataPart(parts, userData, fileName); err != nil {
			return "", errors.Wrapf(err, "error writing user data '%s'", fileName)
		}
	}

	if err := parts.Close(); err != nil {
		return "", errors.Wrap(err, "error closing MIME writer")
	}

	return buf.String(), nil
}

// writeUserDataHeaders writes the required multipart MIME headers for user
// data.
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
func writeUserDataPart(writer *multipart.Writer, u *userData, fileName string) error {
	if fileName == "" {
		return errors.New("user data file name cannot be empty")
	}

	contentType, err := userdata.DirectiveToContentType(u.Directive)
	if err != nil {
		return errors.Wrap(err, "could not detect MIME content type of user data")
	}

	header := textproto.MIMEHeader{}
	header.Add("MIME-Version", "1.0")
	header.Add("Content-Type", contentType)
	header.Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))

	part, err := writer.CreatePart(header)
	if err != nil {
		return errors.Wrap(err, "error making custom user data part")
	}

	if _, err := part.Write([]byte(u.String())); err != nil {
		return errors.Wrap(err, "error writing custom user data")
	}

	return nil
}

// makeUserData returns the user data with logic to provision and set up the
// host as well as run custom user data.
// If mergeParts is true, the provisioning part and the custom part will
// be combined into a single user data part, so they must be the same type (e.g.
// if shell scripts, both must be the same shell scripting language).
func makeUserData(ctx context.Context, settings *evergreen.Settings, h *host.Host, custom string, mergeParts bool) (string, error) {
	var err error
	var customUserData *userData
	if custom != "" {
		customUserData, err = parseUserData(custom)
		if err != nil {
			return "", errors.Wrap(err, "could not parse custom user data")
		}
	}

	if h.Distro.BootstrapSettings.Method != distro.BootstrapMethodUserData {
		if customUserData != nil {
			return ensureWindowsUserDataScriptPersists(h, customUserData).String(), nil
		}
		return "", nil
	}

	provisionOpts, err := h.GenerateFetchProvisioningScriptUserData(settings)
	if err != nil {
		return "", errors.Wrap(err, "creating user data script to fetch provisioning script")
	}
	provision, err := newUserData(*provisionOpts)
	if err != nil {
		return "", errors.Wrap(err, "could not create provisioning user data from options")
	}

	if mergeParts {
		var mergedUserData *userData
		if customUserData != nil {
			mergedUserData, err = provision.merge(customUserData)
			if err != nil {
				return "", errors.Wrap(err, "could not merge user data parts into single part")
			}
		} else {
			mergedUserData = provision
		}

		return ensureWindowsUserDataScriptPersists(h, mergedUserData).String(), nil
	}

	parts := map[string]*userData{
		"provision.txt": ensureWindowsUserDataScriptPersists(h, provision),
	}
	if customUserData != nil {
		parts["custom.txt"] = ensureWindowsUserDataScriptPersists(h, customUserData)
	}
	multipartUserData, err := makeMultipartUserData(parts)
	if err != nil {
		return "", errors.Wrap(err, "error creating user data with multiple parts")
	}

	return multipartUserData, nil
}

// ensureWindowsUserDataScriptPersists adds tags to user data scripts on Windows
// to ensure that they run on every boot.
func ensureWindowsUserDataScriptPersists(h *host.Host, u *userData) *userData {
	if !h.Distro.IsWindows() {
		return u
	}

	u.Persist = u.Directive.CanPersist()

	return u
}
