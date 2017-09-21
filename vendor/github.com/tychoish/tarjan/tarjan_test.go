// Copyright (c) 2013 - Max Persson <max@looplab.se>
// Copyright (c) 2010-2013 - Gustavo Niemeyer <gustavo@niemeyer.net>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tarjan

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestTypeString(t *testing.T) {
	graph := make(map[string][]string)
	graph["1"] = []string{"2"}
	graph["2"] = []string{"3"}
	graph["3"] = []string{"1"}
	graph["4"] = []string{"2", "3", "5"}
	graph["5"] = []string{"4", "6"}
	graph["6"] = []string{"3", "7"}
	graph["7"] = []string{"6"}
	graph["8"] = []string{"5", "7", "8"}

	o := Connections(graph)
	output := sortConnections(o)
	exp := [][]string{
		{"1", "2", "3"},
		{"6", "7"},
		{"4", "5"},
		{"8"},
	}
	if !reflect.DeepEqual(output, exp) {
		t.Fatalf("FAIL.\nexp=%v\ngot=%v\n", exp, output)
	}
}

func TestEmptyGraph(t *testing.T) {
	graph := make(map[string][]string)

	output := Connections(graph)
	if len(output) != 0 {
		t.FailNow()
	}
}

func TestSingleVertex(t *testing.T) {
	graph := make(map[string][]string)
	graph["1"] = []string{}

	o := Connections(graph)
	output := sortConnections(o)
	if output[0][0] != "1" {
		t.FailNow()
	}
}

func TestSingleVertexLoop(t *testing.T) {
	graph := make(map[string][]string)
	graph["1"] = []string{"1"}

	o := Connections(graph)
	output := sortConnections(o)
	if output[0][0] != "1" {
		t.FailNow()
	}
}

func TestMultipleVertexLoop(t *testing.T) {
	graph := make(map[string][]string)
	graph["1"] = []string{"2"}
	graph["2"] = []string{"3"}
	graph["3"] = []string{"1"}

	o := Connections(graph)
	output := sortConnections(o)
	if output[0][0] != "1" {
		t.FailNow()
	}
	if output[0][1] != "2" {
		t.FailNow()
	}
	if output[0][2] != "3" {
		t.FailNow()
	}
}

func ExampleConnections() {
	graph := make(map[string][]string)
	graph["1"] = []string{"2"}
	graph["2"] = []string{"3"}
	graph["3"] = []string{"1"}
	graph["4"] = []string{"2", "3", "5"}
	graph["5"] = []string{"4", "6"}
	graph["6"] = []string{"3", "7"}
	graph["7"] = []string{"6"}
	graph["8"] = []string{"5", "7", "8"}

	o := Connections(graph)
	output := sortConnections(o)
	fmt.Println(output)

	// Output:
	// [[1 2 3] [6 7] [4 5] [8]]
}

func sortConnections(connections [][]string) [][]string {
	out := make([][]string, 0, len(connections))
	for _, cons := range connections {
		sort.Strings(cons)
		out = append(out, cons)
	}
	return out
}
