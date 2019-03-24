package bond

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

// VersionSuite contains tests of both the MongoDBVersion
// representation and the MongoDBVersionSlice type which implements
// the sort.Sorter interface. These tests confirm that the
// NewMongoDBVersion constructor is capable of validating MongoDB
// versions, and that methods associated with report the expected
// properties for a given version string.
type VersionSuite struct {
	baseVersion *MongoDBVersion
	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	suite.Run(t, new(VersionSuite))
}

func (s *VersionSuite) SetupSuite() {
	base, err := NewMongoDBVersion("3.2.6")
	s.NoError(err)
	s.baseVersion = base
}

func (s *VersionSuite) TestValidVersionsParseWithoutErrors() {
	versions := []string{
		"3.0.1",
		"3.3.6",
		"3.4.0-rc0",
		"3.4.0-rc24",
		"3.4.0-rc240",
		"3.4.2-rc1",
		"3.3.5-68-gdd3f158",
		"3.3.5-0-gdd3f158",
		"3.0.2-",
		"3.0.1-pre-",
	}
	for _, version := range versions {
		_, err := NewMongoDBVersion(version)
		s.NoError(err)
	}
}

func (s *VersionSuite) TestInvalidVersionsHaveParseErrors() {
	versions := []string{
		"notAVersion",
		"3.0",
		"30",
		"2.",
		"2",
		"",
		"3.0.0.0",
		"+2.3.4",
		"r3.0.1",
		"v5.3.0",
	}

	for _, version := range versions {
		v, err := NewMongoDBVersion(version)
		s.Error(err)
		s.Nil(v)
	}
}

func (s *VersionSuite) TestVersionParserIdentifiesReleaseCandidates() {
	rcs := []string{
		"3.4.0-rc0",
		"3.4.0-rc24",
		"3.4.0-rc240",
		"3.4.2-rc1",
		"2.6.0-rc2",
	}

	for _, version := range rcs {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsReleaseCandidate(), v.source)
	}

	notRcs := []string{
		"3.4.0",
		"3.2.1",
		"3.1.3",
		"2.6.8",
		"2.7.4",
		"2.6.0-rc2-32-ad273fe2",
		"3.3.5-68-gdd3f158",
		"3.3.5-0-gdd3f158",
		"3.0.2-",
		"3.0.1-pre-",
	}

	for _, version := range notRcs {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.False(v.IsReleaseCandidate())
		s.False(v.IsInitialStableReleaseCandidate())
	}
}

func (s *VersionSuite) TestReleaseSeriesDetection() {
	type testValue struct {
		input    string
		expected string
	}

	values := []*testValue{
		{"3.0.2", "3.0"},
		{"3.2.1", "3.2"},
		{"1.8.4", "1.8"},
		{"2.7.3", "2.7"},
		{"3.4.5", "3.4"},
		{"2.3.0", "2.3"},
		{"2.8.0-rc0", "2.8"},
		{"4.2.0-rc0", "4.2"},
		{"3.3.5-0-gdd3f158", "3.3"},
		{"3.0.2-", "3.0"},
	}

	for _, value := range values {
		v, err := NewMongoDBVersion(value.input)
		s.NoError(err)
		s.Equal(v.Series(), value.expected)
	}
}

func (s *VersionSuite) TestStableAndDevReleaseSeriesAttributes() {
	versions := []string{
		"3.4.0",
		"3.2.1",
		"2.6.8",
		"3.2.5-68-gdd3f158",
		"3.8.5-0-gdd3f158",
		"3.0.2-",
		"3.0.1-pre-",
	}

	for _, version := range versions {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsStableSeries())
		s.False(v.IsDevelopmentSeries())
	}

	devVersion := []string{
		"3.3.0",
		"3.1.1",
		"2.5.8",
		"3.1.5-68-gdd3f158",
		"3.9.5-0-gdd3f158",
		"3.3.2-",
		"3.3.1-pre-",
	}

	for _, version := range devVersion {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsDevelopmentSeries())
		s.False(v.IsStableSeries())
	}

}

func (s *VersionSuite) TestVersionCanIdentifyReleases() {
	releases := []string{
		"3.3.0",
		"3.1.1",
		"2.5.8",
		"3.4.0",
		"3.2.1",
		"2.6.8",
	}

	for _, version := range releases {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsRelease())
		s.False(v.IsDevelopmentBuild())
	}

	builds := []string{
		"3.1.5-68-gdd3f158",
		"3.8.5-23-gffd4a182",
		"3.2.1-",
		"2.6.1-pre-",
	}

	for _, version := range builds {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsDevelopmentBuild())
		s.False(v.IsRelease())
	}
}

func (s *VersionSuite) TestDistinguishInitialStableVersionRC() {
	releases := []string{
		"3.4.0-rc0",
		"3.4.0-rc2",
		"3.4.0-rc4",
		"3.4.0-rc8",
		"3.2.0-rc10",
		"3.2.0-rc58",
		"3.2.0-rc112",
		"2.6.0-rc1",
		"2.8.0-rc3",
	}

	for _, version := range releases {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsReleaseCandidate())
		s.True(v.IsInitialStableReleaseCandidate(), version)
	}

	otherRcs := []string{
		"2.3.0-rc5",
		"3.1.0-rc0",
		"2.3.1-rc0",
		"2.2.4-rc1",
		"2.6.8-rc2",
		"3.4.2-rc3",
		"3.4.2-rc8",
	}

	for _, version := range otherRcs {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.IsReleaseCandidate())
		s.False(v.IsInitialStableReleaseCandidate())
	}
}

func (s *VersionSuite) TestSortingVersions() {
	type testValue struct {
		input    []string
		willSwap bool
	}

	values := []*testValue{
		{input: []string{"3.0.2", "2.2.8"}, willSwap: true},
		{input: []string{"3.2.1", "3.2.4"}, willSwap: false},
		{input: []string{"1.8.4", "1.7.2"}, willSwap: true},
		{input: []string{"2.7.3", "3.0.2"}, willSwap: false},
		{input: []string{"3.8.5", "3.7.0"}, willSwap: true},
		{input: []string{"2.3.0", "2.3.7"}, willSwap: false},
	}

	for _, value := range values {
		var v []MongoDBVersion
		for _, input := range value.input {
			version, err := NewMongoDBVersion(input)
			s.NoError(err)
			v = append(v, *version)
		}

		versions := MongoDBVersionSlice(v)
		original := make(MongoDBVersionSlice, len(versions))
		copy(original, versions)
		versions.Sort()

		if value.willSwap {
			s.NotEqual(versions, original)
		} else {
			s.Equal(versions, original)
		}
	}
}

func (s *VersionSuite) TestVersionSliceStringFormating() {
	versions := []string{"3.2.1", "2.6.18", "2.4.3", "1.8.4"}
	slice := make(MongoDBVersionSlice, len(versions))

	for _, v := range versions {
		ver, err := NewMongoDBVersion(v)
		s.NoError(err)
		slice = append(slice, *ver)
	}

	s.Equal(strings.Join(versions, ", "), slice.String())
}

func (s *VersionSuite) TestParsingIdentifiesRCForRCs() {
	cases := map[string]int{
		"3.2.0-rc0":  0,
		"2.4.0-rc42": 42,
		"1.8.4-rc1":  1,
		"4.6.1-rc12": 12,
	}

	for version, rcNumber := range cases {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.Equal(v.RcNumber(), rcNumber)
	}

}

func (s *VersionSuite) TestRCNumberIsLessThanZeroForNonRCs() {
	cases := []string{
		"2.3.0", "1.5.0-pre", "1.8.5-pre-", "3.2.1", "3.5.0",
		"3.3.5-68-gdd3f158", "3.3.5-0-gdd3f158",
	}

	for _, version := range cases {
		v, err := NewMongoDBVersion(version)
		s.NoError(err)
		s.True(v.RcNumber() < 0)
	}
}

func (s *VersionSuite) TestVersionConversionProducesExpectedVersionObjectsWithoutError() {
	expectedVersion := "3.2.6-rc3"

	// should convert a string to a version object.
	vString, err := ConvertVersion(expectedVersion)
	s.NoError(err)
	s.Equal(vString.source, expectedVersion)

	// pass a pointer to a version object the converter
	vVersionPointer, err := ConvertVersion(vString)
	s.NoError(err)
	s.Equal(vVersionPointer.source, expectedVersion)

	// pass a version object itself rather than a ref.
	vVersionObj, err := ConvertVersion(*vVersionPointer)
	s.NoError(err)
	s.Equal(vVersionObj.source, expectedVersion)

	// try a smevar.Version object
	vSemVar, err := ConvertVersion(vVersionObj.parsed)
	s.NoError(err)
	s.Equal(vSemVar.source, expectedVersion)
}

func (s *VersionSuite) TestVersionConverterErrorsForInvalidVersions() {
	var cases []interface{}
	cases = append(cases, nil, true, false, 2, 3.2, 43, s, "string", "3.2not-release")

	for _, v := range cases {
		version, err := ConvertVersion(v)
		s.Error(err)
		s.Nil(version)
	}
}

func (s *VersionSuite) TestLessThanComparator() {
	// map of input strings to expected output of <
	cases := map[string]bool{
		"1.8.0-rc0": true,
		"3.2.1":     true,
		"3.2.6-rc0": true,
		"3.2.6-rc1": true,
		"3.2.6":     false,
		"3.2.7":     false,
		"3.4.0":     false,
	}

	for version, expectedValue := range cases {
		v, err := ConvertVersion(version)
		s.NoError(err)

		if expectedValue {
			s.True(v.IsLessThan(s.baseVersion))

			// test inverse
			s.True(s.baseVersion.IsGreaterThanOrEqualTo(v))
		} else {
			s.False(v.IsLessThan(s.baseVersion))
			if v.source == s.baseVersion.source {
				continue
			}
			s.False(s.baseVersion.IsGreaterThanOrEqualTo(v),
				fmt.Sprintf("%s %s", s.baseVersion.source, v.source))
		}
	}
}

func (s *VersionSuite) TestLessThanOrEqualToComparator() {
	// map of input strings to expected output of <=
	cases := map[string]bool{
		"1.8.0-rc0": true,
		"3.2.1":     true,
		"3.2.6-rc0": true,
		"3.2.6":     true,
		"3.2.7":     false,
		"3.4.0":     false,
	}

	for version, expectedValue := range cases {
		v, err := ConvertVersion(version)
		s.NoError(err)

		if expectedValue {
			s.True(v.IsLessThanOrEqualTo(s.baseVersion))

			// test inverse
			if v.source == s.baseVersion.source {
				continue
			}

			s.True(s.baseVersion.IsGreaterThan(v), fmt.Sprintf("%s == %s",
				v.source, s.baseVersion.source))
		} else {
			s.False(v.IsLessThanOrEqualTo(s.baseVersion))

			s.False(s.baseVersion.IsGreaterThan(v))
		}
	}
}

func (s *VersionSuite) TestGreaterThanComparator() {
	// map of input strings to expected output of >
	cases := map[string]bool{
		"1.8.0-rc0": false,
		"3.2.1":     false,
		"3.2.6-rc0": false,
		"3.2.6-rc1": false,
		"3.2.6":     false,
		"3.2.7":     true,
		"3.4.0":     true,
	}

	for version, expectedValue := range cases {
		v, err := ConvertVersion(version)
		s.NoError(err)

		if expectedValue {
			s.True(v.IsGreaterThan(s.baseVersion))

			// test inverse
			s.True(s.baseVersion.IsLessThanOrEqualTo(v))
		} else {
			s.False(v.IsGreaterThan(s.baseVersion))

			// test inverse
			s.True(v.IsLessThanOrEqualTo(s.baseVersion))
		}
	}
}

func (s *VersionSuite) TestGreaterThanOrEqualToComparator() {
	// map of input strings to expected output of >=
	cases := map[string]bool{
		"1.8.0-rc0": false,
		"3.2.1":     false,
		"3.2.6-rc0": false,
		"3.2.6-rc1": false,
		"3.2.6":     true,
		"3.2.7":     true,
		"3.4.0":     true,
	}

	for version, expectedValue := range cases {
		v, err := ConvertVersion(version)
		s.NoError(err)

		if expectedValue {
			s.True(v.IsGreaterThanOrEqualTo(s.baseVersion))

			// test inverse
			s.False(v.IsLessThan(s.baseVersion))
		} else {
			s.False(v.IsGreaterThanOrEqualTo(s.baseVersion))

			// test inverse
			s.True(v.IsLessThan(s.baseVersion))
		}
	}
}

func (s *VersionSuite) TestVersionEqualityOperators() {
	// map on input strings to "is Equal" value
	cases := map[string]bool{
		"1.8.0-rc0": false,
		"3.2.1":     false,
		"3.2.6-rc0": false,
		"3.2.6-rc1": false,
		"3.2.6":     true,
		"3.2.7":     false,
		"3.4.0":     false,
	}

	for version, isEqual := range cases {
		v, err := ConvertVersion(version)
		s.NoError(err)

		if isEqual {
			s.True(v.IsEqualTo(s.baseVersion))
			s.True(s.baseVersion.IsEqualTo(v))

			// inverse
			s.False(v.IsNotEqualTo(s.baseVersion))
			s.False(s.baseVersion.IsNotEqualTo(v))

			// also should be true
			s.True(v.IsGreaterThanOrEqualTo(s.baseVersion))
			s.True(s.baseVersion.IsGreaterThanOrEqualTo(v))
			s.True(v.IsLessThanOrEqualTo(s.baseVersion))
			s.True(s.baseVersion.IsLessThanOrEqualTo(v))
		} else {
			s.True(v.IsNotEqualTo(s.baseVersion))
			s.True(s.baseVersion.IsNotEqualTo(v))

			// Inverse
			s.False(v.IsEqualTo(s.baseVersion))
			s.False(s.baseVersion.IsEqualTo(v))
		}
	}
}
