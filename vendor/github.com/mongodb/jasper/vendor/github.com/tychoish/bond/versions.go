/*
MongoDB Versions

The MongoDBVersion type provides support for interacting with MongoDB
versions. This type makes it possible to validate MongoDB version
numbers and ask common questions about MongoDB versions.
*/
package bond

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blang/semver"
)

// MongoDBVersion is a structure representing a version identifier for
// MongoDB. Use the associated methods to ask questions about MongoDB
// versions. All parsing of versions happens during construction, and
// individual method calls are very light-weight.
type MongoDBVersion struct {
	source   string
	parsed   semver.Version
	isRc     bool
	isDev    bool
	rcNumber int
	series   string
	tag      string
}

// NewMongoDBVersion takes a string representing a MongoDB version and
// returns a MongoDBVersion object. If the input string is not a valid
// version, or there were problems parsing the string, the error value
// is non-nil. All parsing of a version happens during this phase.
func NewMongoDBVersion(version string) (*MongoDBVersion, error) {
	v := &MongoDBVersion{source: version, rcNumber: -1}

	if strings.HasSuffix(version, "-") {
		v.isDev = true

		if !strings.Contains(version, "pre") {
			version += "pre-"
		}
	}
	if strings.Contains(version, "~") {
		versionParts := strings.Split(version, "~")
		version = versionParts[0]
		version += "-pre-"
		v.tag = strings.Join(versionParts[1:], "")
		v.isDev = true
	}

	parsed, err := semver.Parse(version)
	if err != nil {
		return nil, err
	}
	v.parsed = parsed

	if strings.Contains(version, "rc") {
		v.isRc = true
	}

	tagParts := strings.Split(version, "-")
	if len(tagParts) > 1 {
		v.tag = strings.Join(tagParts[1:], "-")

		if v.isRc {
			v.rcNumber, err = strconv.Atoi(tagParts[1][2:])
			if len(tagParts) > 2 {
				v.isDev = true
			}
		} else {
			v.isDev = true
		}

	}

	v.series = version[:3]
	return v, err
}

// ConvertVersion takes an un-typed object and attempts to convert it to a
// version object. For use with compactor functions.
func ConvertVersion(v interface{}) (*MongoDBVersion, error) {
	switch version := v.(type) {
	case *MongoDBVersion:
		return version, nil
	case MongoDBVersion:
		return &version, nil
	case string:
		output, err := NewMongoDBVersion(version)
		if err != nil {
			return nil, err
		}
		return output, nil
	case semver.Version:
		return NewMongoDBVersion(version.String())
	default:
		return nil, fmt.Errorf("%v is not a valid version type (%T)", version, version)
	}
}

// String returns a string representation of the MongoDB version
// number.
func (v *MongoDBVersion) String() string {
	return v.source
}

// Series return the release series, generally the first two
// components of a version. For example for 3.2.6, the series is 3.2.
func (v *MongoDBVersion) Series() string {
	return v.series
}

// IsReleaseCandidate returns true for releases that have the "rc[0-9]"
// tag and false otherwise.
func (v *MongoDBVersion) IsReleaseCandidate() bool {
	return v.IsRelease() && v.isRc
}

// IsStableSeries returns true for stable releases, ones where the
// second component of the version string (i.e. "Minor" in semantic
// versioning terms) are even, and false otherwise.
func (v *MongoDBVersion) IsStableSeries() bool {
	return v.parsed.Minor%2 == 0
}

// IsDevelopmentSeries returns true for development (snapshot)
// releases. These versions are those where the second component
// (e.g. "Minor" in semantic versioning terms) are odd, and false
// otherwise.
func (v *MongoDBVersion) IsDevelopmentSeries() bool {
	return !v.IsStableSeries()
}

// StableReleaseSeries returns a series string (e.g. X.Y) for this
// version. For stable releases, the output is the same as
// .Series(). For development releases, this method returns the *next*
// stable series.
func (v *MongoDBVersion) StableReleaseSeries() string {
	if v.IsStableSeries() {
		return v.Series()
	}

	if v.parsed.Minor < 9 {
		return fmt.Sprintf("%d.%d", v.parsed.Major, v.parsed.Minor+1)
	}

	return fmt.Sprintf("%d.0", v.parsed.Major+1)
}

// IsRelease returns true for all version strings that refer to a
// release, including development, release candidate and GA releases,
// and false otherwise. Other builds, including test builds and
// "nightly" snapshots of MongoDB have version strings, but are not
// releases.
func (v *MongoDBVersion) IsRelease() bool {
	return !v.isDev
}

// IsDevelopmentBuild returns true for all non-release builds,
// including nightly snapshots and all testing and development
// builds.
func (v *MongoDBVersion) IsDevelopmentBuild() bool {
	return v.isDev
}

// IsInitialStableReleaseCandidate returns true for release
// candidates for the initial public release of a new stable release
// series.
func (v *MongoDBVersion) IsInitialStableReleaseCandidate() bool {
	if v.IsStableSeries() {
		return v.parsed.Patch == 0 && v.IsReleaseCandidate()
	}
	return false
}

// RcNumber returns an integer for the RC counter. For non-rc releases,
// returns -1.
func (v *MongoDBVersion) RcNumber() int {
	return v.rcNumber
}

// IsLessThan returns true when "version" is less than (e.g. earlier)
// than the object itself.
func (v *MongoDBVersion) IsLessThan(version *MongoDBVersion) bool {
	return v.parsed.LT(version.parsed)
}

// IsLessThanOrEqualTo returns true when "version" is less than or
// equal to (e.g. earlier or the same as) the object itself.
func (v *MongoDBVersion) IsLessThanOrEqualTo(version *MongoDBVersion) bool {
	// semver considers release candidates equal to GA, so we have to special case this

	if v.IsEqualTo(version) {
		return true
	}

	return v.parsed.LT(version.parsed)
}

// IsGreaterThan returns true when "version" is greater than (e.g. later)
// than the object itself.
func (v *MongoDBVersion) IsGreaterThan(version *MongoDBVersion) bool {
	return v.parsed.GT(version.parsed)
}

// IsGreaterThanOrEqualTo returns true when "version" is greater than
// or equal to (e.g. the same as or later than) the object itself.
func (v *MongoDBVersion) IsGreaterThanOrEqualTo(version *MongoDBVersion) bool {
	if v.IsEqualTo(version) {
		return true
	}
	return v.parsed.GT(version.parsed)
}

// IsEqualTo returns true when "version" is the same as the object
// itself.
func (v *MongoDBVersion) IsEqualTo(version *MongoDBVersion) bool {
	return v.source == version.source
}

// IsNotEqualTo returns true when "version" is the different from the
// object itself.
func (v *MongoDBVersion) IsNotEqualTo(version *MongoDBVersion) bool {
	return v.source != version.source
}

/////////////////////////////////////////////
//
// Support for Sorting Slices of MongoDB Versions
//
/////////////////////////////////////////////

// MongoDBVersionSlice is an alias for []MongoDBVersion that supports
// the sort.Sorter interface, and makes it possible to sort slices of
// MongoDB versions.
type MongoDBVersionSlice []MongoDBVersion

// Len is  required  by the sort.Sorter interface. Returns
// the length of the slice.
func (s MongoDBVersionSlice) Len() int {
	return len(s)
}

// Less is a required by the sort.Sorter interface. Uses blang/semver
// to compare two versions.
func (s MongoDBVersionSlice) Less(i, j int) bool {
	left := s[i]
	right := s[j]

	return left.parsed.LT(right.parsed)
}

// Swap is a required by the sort.Sorter interface. Changes the
// position of two elements in the slice.
func (s MongoDBVersionSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// String() adds suport for the Stringer interface, which makes it
// possible to print slices of MongoDB versions as comma separated
// lists.
func (s MongoDBVersionSlice) String() string {
	var out []string

	for _, v := range s {
		if len(v.source) == 0 {
			// some elements end up empty.
			continue
		}

		out = append(out, v.source)
	}

	return strings.Join(out, ", ")
}

// Sort provides a wrapper around sort.Sort() for slices of MongoDB
// versions objects.
func (s MongoDBVersionSlice) Sort() {
	sort.Sort(s)
}
