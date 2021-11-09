package poplar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	for _, test := range []struct {
		name     string
		artifact TestArtifact
		test     func(TestArtifact)
		hasErr   bool
	}{
		{
			name:     "ConverGzip",
			artifact: TestArtifact{ConvertGzip: true},
			test: func(a TestArtifact) {
				assert.True(t, a.ConvertGzip)
				assert.True(t, a.DataGzipped)
			},
		},
		{
			name: "ConvertCSV2FTDC",
			artifact: TestArtifact{
				PayloadCSV:      true,
				ConvertCSV2FTDC: true,
			},
			test: func(a TestArtifact) {
				assert.False(t, a.PayloadCSV)
				assert.True(t, a.PayloadFTDC)
				assert.True(t, a.ConvertCSV2FTDC)
			},
		},
		{
			name: "ConvertBSON2FTDC",
			artifact: TestArtifact{
				PayloadBSON:      true,
				ConvertBSON2FTDC: true,
			},
			test: func(a TestArtifact) {
				assert.False(t, a.PayloadBSON)
				assert.True(t, a.PayloadFTDC)
				assert.True(t, a.ConvertBSON2FTDC)
			},
		},
		{
			name: "ConvertJSON2FTDC",
			artifact: TestArtifact{
				PayloadJSON:      true,
				ConvertJSON2FTDC: true,
			},
			test: func(a TestArtifact) {
				assert.False(t, a.PayloadJSON)
				assert.True(t, a.PayloadFTDC)
				assert.True(t, a.ConvertJSON2FTDC)
			},
		},
		{
			name:     "ContradictoryConversions",
			artifact: TestArtifact{},
			test: func(a TestArtifact) {
				for _, combination := range exhaustContradictions(5) {
					a.ConvertCSV2FTDC = combination[0]
					a.ConvertBSON2FTDC = combination[1]
					a.ConvertJSON2FTDC = combination[2]
					a.ConvertGzip = combination[3]

					if isMoreThanOneTrue([]bool{a.ConvertCSV2FTDC, a.ConvertBSON2FTDC, a.ConvertJSON2FTDC, a.ConvertGzip}) {
						assert.Error(t, a.Validate())
					} else {
						assert.NoError(t, a.Validate())
					}
				}
			},
		},
		{
			name:     "ContradictoryPayloads",
			artifact: TestArtifact{},
			test: func(a TestArtifact) {
				for _, combination := range exhaustContradictions(5) {
					a.PayloadTEXT = combination[0]
					a.PayloadCSV = combination[1]
					a.PayloadBSON = combination[2]
					a.PayloadJSON = combination[3]
					a.PayloadFTDC = combination[4]

					if isMoreThanOneTrue([]bool{a.PayloadTEXT, a.PayloadCSV, a.PayloadBSON, a.PayloadJSON, a.PayloadFTDC}) {
						assert.Error(t, a.Validate())
					} else {
						assert.NoError(t, a.Validate())
					}
				}
			},
		},
		{
			name:     "ContradictoryFormats",
			artifact: TestArtifact{},
			test: func(a TestArtifact) {
				for _, combination := range exhaustContradictions(4) {
					a.DataGzipped = combination[0]
					a.DataTarball = combination[1]
					a.DataUncompressed = combination[2]

					if isMoreThanOneTrue([]bool{a.DataGzipped, a.DataTarball, a.DataUncompressed}) {
						assert.Error(t, a.Validate())
					} else {
						assert.NoError(t, a.Validate())
					}
				}
			},
		},
		{
			name:     "ContradictorySchema",
			artifact: TestArtifact{},
			test: func(a TestArtifact) {
				for _, combination := range exhaustContradictions(4) {
					a.EventsCollapsed = combination[0]
					a.EventsHistogram = combination[1]
					a.EventsIntervalSummary = combination[2]
					a.EventsRaw = combination[3]

					if isMoreThanOneTrue([]bool{a.EventsCollapsed, a.EventsHistogram, a.EventsIntervalSummary, a.EventsRaw}) {
						assert.Error(t, a.Validate())
					} else {
						assert.NoError(t, a.Validate())
					}
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.hasErr {
				assert.Error(t, test.artifact.Validate())
			} else {
				assert.NoError(t, test.artifact.Validate())
			}
			if test.test != nil {
				test.test(test.artifact)
			}
		})
	}
}

func exhaustContradictions(numOpts int) [][]bool {
	if numOpts == 1 {
		return [][]bool{[]bool{true}, []bool{false}}
	}

	i := 0
	combinations := [][]bool{}
	for _, combination := range exhaustContradictions(numOpts - 1) {
		combinations = append(combinations, make([]bool, numOpts))
		combinations = append(combinations, make([]bool, numOpts))
		copy(combinations[i], append(combination, true))
		i++
		copy(combinations[i], append(combination, false))
		i++
	}

	return combinations
}
