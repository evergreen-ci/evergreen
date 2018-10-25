package stats

import (
	"testing"
)

type distanceFunctionType func([]float64, []float64) (float64, error)

var minkowskiDistanceTestMatrix = []struct {
	dataPointX []float64
	dataPointY []float64
	lambda     float64
	distance   float64
}{
	{[]float64{2, 3, 4, 5, 6, 7, 8}, []float64{8, 7, 6, 5, 4, 3, 2}, 1, 24},
	{[]float64{2, 3, 4, 5, 6, 7, 8}, []float64{8, 7, 6, 5, 4, 3, 2}, 2, 10.583005244258363},
	{[]float64{2, 3, 4, 5, 6, 7, 8}, []float64{8, 7, 6, 5, 4, 3, 2}, 99, 6},
}

var distanceTestMatrix = []struct {
	dataPointX       []float64
	dataPointY       []float64
	distance         float64
	distanceFunction distanceFunctionType
}{
	{[]float64{2, 3, 4, 5, 6, 7, 8}, []float64{8, 7, 6, 5, 4, 3, 2}, 6, ChebyshevDistance},
	{[]float64{2, 3, 4, 5, 6, 7, 8}, []float64{8, 7, 6, 5, 4, 3, 2}, 24, ManhattanDistance},
	{[]float64{2, 3, 4, 5, 6, 7, 8}, []float64{8, 7, 6, 5, 4, 3, 2}, 10.583005244258363, EuclideanDistance},
}

func TestDataSetDistances(t *testing.T) {

	// Test Minkowski Distance with different lambda values.
	for _, testData := range minkowskiDistanceTestMatrix {
		distance, err := MinkowskiDistance(testData.dataPointX, testData.dataPointY, testData.lambda)
		if err != nil && distance != testData.distance {
			t.Errorf("Failed to compute Minkowski distance.")
		}

		_, err = MinkowskiDistance([]float64{}, []float64{}, 3)
		if err == nil {
			t.Errorf("Empty slices should have resulted in an error")
		}

		_, err = MinkowskiDistance([]float64{1, 2, 3}, []float64{1, 4}, 3)
		if err == nil {
			t.Errorf("Different length slices should have resulted in an error")
		}

		_, err = MinkowskiDistance([]float64{999, 999, 999}, []float64{1, 1, 1}, 1000)
		if err == nil {
			t.Errorf("Infinite distance should have resulted in an error")
		}
	}

	// Compute distance with the help of all
	// algorithms.
	for _, testSet := range distanceTestMatrix {
		distance, err := testSet.distanceFunction(testSet.dataPointX, testSet.dataPointY)
		if err != nil && testSet.distance != distance {
			t.Errorf("Failed to compute distance.")
		}

		_, err = testSet.distanceFunction([]float64{}, []float64{})
		if err == nil {
			t.Errorf("Empty slices should have resulted in an error")
		}
	}
}
