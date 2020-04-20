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

// kim: TODO: see if recursive scripts will just magic work, i.e.
// <powershell>...<powershell>...</powershell></powershell>

// userData represents a single user data part for cloud-init.
type userData struct {
	userdata.Options
}

func NewUserData(opts userdata.Options) (*userData, error) {
	u := userData{
		Options: opts,
	}
	if err := u.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid user data")
	}
	return &u, nil
}

// Merge combines one user data part with another.
// kim: TODO: test
func (u *userData) merge(other *userData) (*userData, error) {
	if u.Directive != other.Directive {
		return nil, errors.Errorf(
			"cannot combine user data parts of incompatible types: '%s' is incompatible with '%s'",
			u.Directive, other.Directive)
	}
	if u.ClosingTag != "" && other.ClosingTag != "" && u.ClosingTag != other.ClosingTag {
		return nil, errors.Errorf(
			"cannot combine user data with differing closing tags: '%s' is incompatible with '%s'",
			u.ClosingTag, other.ClosingTag)
	}
	return NewUserData(userdata.Options{
		Directive:  u.Directive,
		Content:    strings.Join([]string{u.Content, other.Content}, "\n"),
		ClosingTag: u.ClosingTag,
		Persist:    u.Persist || other.Persist,
	})
}

// String returns the entire user data part constructed from its components.
func (u *userData) String() string {
	s := strings.Join([]string{
		string(u.Directive),
		u.Content,
		string(u.ClosingTag),
	}, "\n")
	if u.Persist {
		s = strings.Join([]string{s, persistTag}, "\n")
	}
	return s
}

// ParseUserData returns the user data string as a structured user data.
// kim: TODO: test
func ParseUserData(userData string) (*userData, error) {
	var err error
	var persist bool
	persist, userData, err = splitPersistTags(userData)
	if err != nil {
		return nil, errors.Wrap(err, "problem splitting persist tags")
	}

	var directive userdata.Directive
	directive, userData, err = splitDirective(userData)
	if err != nil {
		return nil, errors.Wrap(err, "problem splitting directive")
	}

	var closingTag userdata.ClosingTag
	if userdata.NeedsClosingTag(directive) {
		closingTag = userdata.ClosingTagFor(directive)
		userData, err = splitClosingTagFor(userData, closingTag)
		if err != nil {
			return nil, errors.Wrapf(err, "problem splitting closing tag '%s'", closingTag)
		}
	}

	return NewUserData(userdata.Options{
		Directive:  directive,
		Content:    userData,
		ClosingTag: closingTag,
		Persist:    persist,
	})
}

// splitDirective finds the directive on the first line of user data. This
// should be called after splitPersistTags in order to ensure that there is no
// persist tag before the directive line.
func splitDirective(userData string) (directive userdata.Directive, userDataWithoutDirective string, err error) {
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
			userDataWithoutDirective = strings.Replace(userData, firstLine, "", 1)
			return userdata.Directive(firstLine), userDataWithoutDirective, nil
		}
	}
	return "", "", errors.Errorf("user data directive is not recognizable from first line: '%s'", firstLine)
}

// splitClosingTag finds the closing tag to match the end of the user data.
func splitClosingTag(userData string) (closingTag userdata.ClosingTag, userDataWithoutClosingTag string, err error) {
	for _, closingTag := range userdata.ClosingTags() {
		if count := strings.Count(userData, string(closingTag)); count != 0 {
			if count > 1 {
				return "", "", errors.Errorf("closing tag '%s' cannot occur more than once", closingTag)
			}
			userDataWithoutClosingTag = strings.Replace(userData, string(closingTag), "", 1)
			return closingTag, userDataWithoutClosingTag, nil
		}
	}
	return "", "", errors.New("user data does not have closing tags")
}

// splitClosingTagFor finds the closing tag to match
func splitClosingTagFor(userData string, closingTag userdata.ClosingTag) (userDataWithoutClosingTag string, err error) {
	count := strings.Count(userData, string(closingTag))
	if count == 0 {
		return "", errors.Errorf("user data does not have closing tag '%s'", closingTag)
	}
	if count > 1 {
		return "", errors.Errorf("cannot have multiple occurrences of closing tag '%s'", closingTag)
	}
	userDataWithoutClosingTag = strings.Replace(userData, string(closingTag), "", 1)
	return userDataWithoutClosingTag, nil
}

const (
	persistTag        = "<persist>true</persist>"
	persistTagPattern = "<persist>[[:space:]]true[[:space:]]</persist>"
)

// splitPersistTags returns whether the user data contains persist tags and
// the user data without those tags if any are present.
func splitPersistTags(userData string) (found bool, userDataWithoutPersist string, err error) {
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

	contentType, ok := userdata.DirectiveToContentType()[u.Directive]
	if !ok {
		return errors.Errorf("could not detect MIME content type of user data from directive '%s'", u.Directive)
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

// provisioningUserData returns the user data with logic to provision and set up
// the host as well as run custom user data.
// If mergeParts is true, the provisioning part and the custom part will
// be combined into a single user datapart, so they must be the same type (e.g.
// if shell scripts, both must be the same shell scripting language).
// Care should be taken when adding more content to user data, since the user
// data length is subject to a 16 kB hard limit
// (https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#RunInstancesInput).
// We could possibly increase the length by gzip compressing it.
func provisioningUserData(ctx context.Context, env evergreen.Environment, h *host.Host, custom string, mergeParts bool) (string, error) {
	customUserData, err := ParseUserData(custom)
	if err != nil {
		return "", errors.Wrap(err, "could not parse custom user data")
	}

	if h.Distro.BootstrapSettings.Method != distro.BootstrapMethodUserData {
		return ensureWindowsUserDataScriptPersists(h, customUserData).String(), nil
	}
	settings := env.Settings()

	creds, err := h.GenerateJasperCredentials(ctx, env)
	if err != nil {
		return "", errors.Wrapf(err, "problem generating Jasper credentials for host '%s'", h.Id)
	}

	provisionOpts, err := h.ProvisioningUserData(settings, creds)
	if err != nil {
		return "", errors.Wrap(err, "could not generate user data for provisioning host")
	}
	provision, err := NewUserData(*provisionOpts)
	if err != nil {
		return "", errors.Wrap(err, "could not create user data for provisioning from options")
	}

	if mergeParts {
		mergedUserData, err := provision.merge(customUserData)
		if err != nil {
			return "", errors.Wrap(err, "could not merge user data parts into single part")
		}
		return ensureWindowsUserDataScriptPersists(h, mergedUserData).String(), errors.Wrap(h.SaveJasperCredentials(ctx, env, creds), "problem saving Jasper credentials to host")
	}

	multipartUserData, err := makeMultipartUserData(map[string]*userData{
		"provision.txt": ensureWindowsUserDataScriptPersists(h, provision),
		"custom.txt":    ensureWindowsUserDataScriptPersists(h, customUserData),
	})
	if err != nil {
		return "", errors.Wrap(err, "error creating user data with multiple parts")
	}

	return multipartUserData, errors.Wrap(h.SaveJasperCredentials(ctx, env, creds), "problem saving Jasper credentials to host")
}

// ensureWindowsUserDataScriptPersists adds tags to user data scripts on Windows
// to ensure that they run on every boot.
func ensureWindowsUserDataScriptPersists(h *host.Host, u *userData) *userData {
	if !h.Distro.IsWindows() {
		return u
	}
	if u.Persist {
		return u
	}

	switch u.Directive {
	case userdata.PowerShellScript, userdata.BatchScript:
		u.Persist = true
	}
	return u
}
