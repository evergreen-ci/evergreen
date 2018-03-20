package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMatrixIntermediateParsing(t *testing.T) {
	Convey("Testing different project files with matrix definitions", t, func() {
		Convey("a set of axes should parse", func() {
			axes := `
axes:
- id: os
  display_name: Operating System
  values:
  - id: ubuntu
    display_name: Ubuntu
    tags: "linux"
    variables:
      user: root
    run_on: ubuntu_small
  - id: rhel
    display_name: Red Hat
    tags: ["linux", "enterprise"]
    run_on:
    - rhel55
    - rhel62
buildvariants:
- matrix_name: tester
  display_name: ${os}
  matrix_spec:
    os: "*"
  tasks:
  rules:
  - if:
      os: "*"
    then:
      set:
        tags: "gotcha_boy"
`
			p, errs := createIntermediateProject([]byte(axes))
			So(errs, ShouldBeNil)
			axis := p.Axes[0]
			So(axis.Id, ShouldEqual, "os")
			So(axis.DisplayName, ShouldEqual, "Operating System")
			So(len(axis.Values), ShouldEqual, 2)
			So(axis.Values[0], ShouldResemble, axisValue{
				Id:          "ubuntu",
				DisplayName: "Ubuntu",
				Tags:        []string{"linux"},
				Variables:   map[string]string{"user": "root"},
				RunOn:       []string{"ubuntu_small"},
			})
			So(axis.Values[1], ShouldResemble, axisValue{
				Id:          "rhel",
				DisplayName: "Red Hat",
				Tags:        []string{"linux", "enterprise"},
				RunOn:       []string{"rhel55", "rhel62"},
			})
			tags := p.BuildVariants[0].matrix.Rules[0].Then.Set.Tags
			So(tags, ShouldResemble, parserStringSlice{"gotcha_boy"})
		})
		Convey("a barebones matrix definition should parse", func() {
			simple := `
buildvariants:
- matrix_name: "test"
  matrix_spec: {"os": ".linux", "bits":["32", "64"]}
  exclude_spec: [{"os":"ubuntu", "bits":"32"}]
- matrix_name: "test2"
  matrix_spec:
    os: "windows95"
    color:
    - red
    - blue
    - green
`
			p, errs := createIntermediateProject([]byte(simple))
			So(errs, ShouldBeNil)
			So(len(p.BuildVariants), ShouldEqual, 2)
			m1 := *p.BuildVariants[0].matrix
			So(m1, ShouldResemble, matrix{
				Id: "test",
				Spec: matrixDefinition{
					"os":   []string{".linux"},
					"bits": []string{"32", "64"},
				},
				Exclude: []matrixDefinition{
					{"os": []string{"ubuntu"}, "bits": []string{"32"}},
				},
			})
			m2 := *p.BuildVariants[1].matrix
			So(m2, ShouldResemble, matrix{
				Id: "test2",
				Spec: matrixDefinition{
					"os":    []string{"windows95"},
					"color": []string{"red", "blue", "green"},
				},
			})
		})
		Convey("a mixed definition should parse", func() {
			simple := `
buildvariants:
- matrix_name: "test"
  matrix_spec: {"os": "*", "bits": "*"}
- name: "single_variant"
  tasks: "*"
`
			p, errs := createIntermediateProject([]byte(simple))
			So(errs, ShouldBeNil)
			So(len(p.BuildVariants), ShouldEqual, 2)
			m1 := *p.BuildVariants[0].matrix
			So(m1.Id, ShouldEqual, "test")
			So(p.BuildVariants[1].Name, ShouldEqual, "single_variant")
			So(p.BuildVariants[1].Tasks, ShouldResemble, parserBVTaskUnits{parserBVTaskUnit{Name: "*"}})
		})
	})
}

func TestMatrixDefinitionAllCells(t *testing.T) {
	Convey("With a set of test definitions", t, func() {
		Convey("an empty definition should return an empty list", func() {
			a := matrixDefinition{}
			cells := a.allCells()
			So(len(cells), ShouldEqual, 0)
		})
		Convey("an empty axis should cause a panic", func() {
			a := matrixDefinition{
				"a": []string{},
				"b": []string{"1"},
			}
			So(func() { a.allCells() }, ShouldPanic)
		})
		Convey("a one-cell matrix should return a one-item list", func() {
			a := matrixDefinition{
				"a": []string{"0"},
			}
			cells := a.allCells()
			So(len(cells), ShouldEqual, 1)
			So(cells, ShouldContainResembling, matrixValue{"a": "0"})
			b := matrixDefinition{
				"a": []string{"0"},
				"b": []string{"1"},
				"c": []string{"2"},
			}
			cells = b.allCells()
			So(len(cells), ShouldEqual, 1)
			So(cells, ShouldContainResembling, matrixValue{"a": "0", "b": "1", "c": "2"})
		})
		Convey("a one-axis matrix should return an equivalent list", func() {
			a := matrixDefinition{
				"a": []string{"0", "1", "2"},
			}
			cells := a.allCells()
			So(len(cells), ShouldEqual, 3)
			So(cells, ShouldContainResembling, matrixValue{"a": "0"})
			So(cells, ShouldContainResembling, matrixValue{"a": "1"})
			So(cells, ShouldContainResembling, matrixValue{"a": "2"})
			b := matrixDefinition{
				"a": []string{"0"},
				"b": []string{"0", "1", "2"},
			}
			cells = b.allCells()
			So(len(cells), ShouldEqual, 3)
			So(cells, ShouldContainResembling, matrixValue{"b": "0", "a": "0"})
			So(cells, ShouldContainResembling, matrixValue{"b": "1", "a": "0"})
			So(cells, ShouldContainResembling, matrixValue{"b": "2", "a": "0"})
			c := matrixDefinition{
				"c": []string{"0", "1", "2"},
				"d": []string{"0"},
			}
			cells = c.allCells()
			So(len(cells), ShouldEqual, 3)
			So(cells, ShouldContainResembling, matrixValue{"c": "0", "d": "0"})
			So(cells, ShouldContainResembling, matrixValue{"c": "1", "d": "0"})
			So(cells, ShouldContainResembling, matrixValue{"c": "2", "d": "0"})
		})
		Convey("a 2x2 matrix should expand properly", func() {
			a := matrixDefinition{
				"a": []string{"0", "1"},
				"b": []string{"0", "1"},
			}
			cells := a.allCells()
			So(len(cells), ShouldEqual, 4)
			So(cells, ShouldContainResembling, matrixValue{"a": "0", "b": "0"})
			So(cells, ShouldContainResembling, matrixValue{"a": "1", "b": "0"})
			So(cells, ShouldContainResembling, matrixValue{"a": "0", "b": "1"})
			So(cells, ShouldContainResembling, matrixValue{"a": "1", "b": "1"})
		})
		Convey("a disgustingly large matrix should expand properly", func() {
			bigList := func(max int) []string {
				out := []string{}
				for i := 0; i < max; i++ {
					out = append(out, fmt.Sprint(i))
				}
				return out
			}

			huge := matrixDefinition{
				"a": bigList(15),
				"b": bigList(290),
				"c": bigList(20),
			}
			cells := huge.allCells()
			So(len(cells), ShouldEqual, 15*290*20)
			So(cells, ShouldContainResembling, matrixValue{"a": "0", "b": "0", "c": "0"})
			So(cells, ShouldContainResembling, matrixValue{"a": "14", "b": "289", "c": "19"})
			// some random guesses just for fun
			So(cells, ShouldContainResembling, matrixValue{"a": "10", "b": "29", "c": "1"})
			So(cells, ShouldContainResembling, matrixValue{"a": "1", "b": "2", "c": "17"})
			So(cells, ShouldContainResembling, matrixValue{"a": "8", "b": "100", "c": "5"})
		})
	})
}

func TestMatrixDefinitionContains(t *testing.T) {
	Convey("With a set of test definitions", t, func() {
		Convey("an empty definition should match nothing", func() {
			a := matrixDefinition{}
			So(a.contains(matrixValue{"a": "0"}), ShouldBeFalse)
		})
		Convey("all definitions contain the empty value", func() {
			a := matrixDefinition{}
			So(a.contains(matrixValue{}), ShouldBeTrue)
			b := matrixDefinition{
				"a": []string{"0", "1"},
				"b": []string{"0", "1"},
			}
			So(b.contains(matrixValue{}), ShouldBeTrue)
		})
		Convey("a one-axis matrix should match all of its elements", func() {
			a := matrixDefinition{
				"a": []string{"0", "1", "2"},
			}
			So(a.contains(matrixValue{"a": "0"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "1"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "2"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "3"}), ShouldBeFalse)
		})
		Convey("a 2x2 matrix should match all of its elements", func() {
			a := matrixDefinition{
				"a": []string{"0", "1"},
				"b": []string{"0", "1"},
			}
			cells := a.allCells()
			So(len(cells), ShouldEqual, 4)
			So(a.contains(matrixValue{"a": "0", "b": "0"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "1", "b": "0"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "0", "b": "1"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "1", "b": "1"}), ShouldBeTrue)
			So(a.contains(matrixValue{"a": "1", "b": "2"}), ShouldBeFalse)
			Convey("and sub-match all of its individual axis values", func() {
				So(a.contains(matrixValue{"a": "0"}), ShouldBeTrue)
				So(a.contains(matrixValue{"a": "1"}), ShouldBeTrue)
				So(a.contains(matrixValue{"b": "0"}), ShouldBeTrue)
				So(a.contains(matrixValue{"b": "1"}), ShouldBeTrue)
				So(a.contains(matrixValue{"b": "7"}), ShouldBeFalse)
				So(a.contains(matrixValue{"c": "1"}), ShouldBeFalse)
				So(a.contains(matrixValue{"a": "1", "b": "1", "c": "1"}), ShouldBeFalse)
			})
		})
	})
}

func TestBuildMatrixVariantSimple(t *testing.T) {
	testMatrix := &matrix{Id: "test"}
	Convey("With a set of test axes", t, func() {
		axes := []matrixAxis{
			{
				Id: "a",
				Values: []axisValue{
					{Id: "0", Tags: []string{"zero"}},
					{Id: "1", Tags: []string{"odd"}},
					{Id: "2", Tags: []string{"even", "prime"}},
					{Id: "3", Tags: []string{"odd", "prime"}},
				},
			},
			{
				Id: "b",
				Values: []axisValue{
					{Id: "0", Tags: []string{"zero"}},
					{Id: "1", Tags: []string{"odd"}},
					{Id: "2", Tags: []string{"even", "prime"}},
					{Id: "3", Tags: []string{"odd", "prime"}},
				},
			},
		}
		Convey("and matrix value test:{a:0, b:0}", func() {
			mv := matrixValue{"a": "0", "b": "0"}
			Convey("the variant should build without error", func() {
				v, err := buildMatrixVariant(axes, mv, testMatrix, nil)
				So(err, ShouldBeNil)
				Convey("with id='test__a~0_b~0', tags=[zero]", func() {
					So(v.Name, ShouldEqual, "test__a~0_b~0")
					So(v.matrixVal, ShouldResemble, mv)
					So(v.Tags, ShouldContain, "zero")
					So(v.matrixId, ShouldEqual, "test")
				})
			})
		})
		Convey("and matrix value test:{a:1, b:3}", func() {
			mv := matrixValue{"b": "3", "a": "1"}
			Convey("the variant should build without error", func() {
				v, err := buildMatrixVariant(axes, mv, testMatrix, nil)
				So(err, ShouldBeNil)
				Convey("with id='test__a~1_b~3', tags=[odd, prime]", func() {
					So(v.Name, ShouldEqual, "test__a~1_b~3")
					So(v.Tags, ShouldContain, "odd")
					So(v.Tags, ShouldContain, "prime")
				})
			})
		})
		Convey("and a matrix value that references non-existent axis values", func() {
			mv := matrixValue{"b": "2", "a": "4"}
			Convey("should return an error", func() {
				_, err := buildMatrixVariant(axes, mv, testMatrix, nil)
				So(err, ShouldNotBeNil)
			})
		})
		Convey("and a matrix value that references non-existent axis names", func() {
			mv := matrixValue{"b": "2", "coolfun": "4"}
			Convey("should return an error", func() {
				_, err := buildMatrixVariant(axes, mv, testMatrix, nil)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

// helper for pulling variants out of a list
func findVariant(vs []parserBV, id string) parserBV {
	for _, v := range vs {
		if v.Name == id {
			return v
		}
	}
	panic("not found")
}

func TestMatrixVariantsSimple(t *testing.T) {
	Convey("With a delicious set of test axes", t, func() {
		// These tests are structured around a magical project that tests
		// colorful candies. We will be testing M&Ms, Skittles, and Necco Wafers
		// (all candies copyright their respective holders). We need to test
		// each color of each candy individually, so we've decided to simplify
		// our variant definitions with a matrix! The colors are as follows:
		//  M&Ms:     red, orange, yellow, green, blue, brown (6)
		//  Skittles: red, orange, yellow, green, purple (5)
		//  Necco:    orange, yellow, green, purple, pink, brown, black, white (8)
		// TODO: maybe move this up top for multiple tests
		axes := []matrixAxis{
			{
				Id: "color",
				Values: []axisValue{
					{Id: "red", Tags: []string{"hot_color"}},
					{Id: "pink", Tags: []string{"hot_color"}},
					{Id: "orange", Tags: []string{"hot_color"}},
					{Id: "yellow", Tags: []string{"hot_color"}},
					{Id: "brown", Tags: []string{"hot_color"}},
					{Id: "green", Tags: []string{"cool_color"}},
					{Id: "blue", Tags: []string{"cool_color"}},
					{Id: "purple", Tags: []string{"cool_color"}},
					{Id: "black"},
					{Id: "white"},
				},
			},
			{
				Id: "brand",
				Values: []axisValue{
					{Id: "m&ms", Tags: []string{"chocolate"}},
					{Id: "skittles", Tags: []string{"chewy"}},
					{Id: "necco", Tags: []string{"chalk"}},
				},
			},
		}
		ase := NewAxisSelectorEvaluator(axes)
		So(ase, ShouldNotBeNil)
		Convey("and a valid matrix", func() {
			m := matrix{
				Id: "candy",
				Spec: matrixDefinition{
					"color": []string{
						"red", "orange", "yellow", "brown", "green",
						"blue", "purple", "black", "white", "pink",
					},
					"brand": []string{"m&ms", "skittles", "necco"},
				},
				Exclude: []matrixDefinition{
					{"brand": []string{"skittles"}, "color": []string{"brown", "blue"}},
					{"brand": []string{"m&ms"}, "color": []string{"purple"}},
					{"brand": []string{"m&ms", "skittles"},
						"color": []string{"pink", "black", "white"}},
					{"brand": []string{"necco"}, "color": []string{"red", "blue"}},
				},
			}
			Convey("building a list of variants should succeed", func() {
				vs, errs := buildMatrixVariants(axes, ase, []matrix{m})
				So(errs, ShouldBeNil)
				Convey("and return the correct list of combinations", func() {
					So(len(vs), ShouldEqual, 19)
					// check a couple random samples
					d1 := findVariant(vs, "candy__color~yellow_brand~skittles")
					So(d1.Tags, ShouldContain, "hot_color")
					So(d1.Tags, ShouldContain, "chewy")
					d2 := findVariant(vs, "candy__color~black_brand~necco")
					So(len(d2.Tags), ShouldEqual, 1)
					So(d2.Tags, ShouldContain, "chalk")
					// ensure all values are in there...
					vals := []matrixValue{}
					for _, v := range vs {
						vals = append(vals, v.matrixVal)
					}
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "red"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "orange"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "yellow"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "green"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "blue"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "brown"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "red"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "orange"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "yellow"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "green"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "purple"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "orange"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "yellow"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "green"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "purple"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "pink"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "white"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "black"})
				})
			})
		})
		Convey("and a valid matrix using tag selectors", func() {
			m := matrix{
				Id: "candy",
				Spec: matrixDefinition{
					"color": []string{".hot_color", ".cool_color"}, // all but white and black
					"brand": []string{"*"},
				},
				Exclude: []matrixDefinition{
					{"brand": []string{".chewy"}, "color": []string{"brown", "blue"}},
					{"brand": []string{".chocolate"}, "color": []string{"purple"}},
					{"brand": []string{"!.chewy", "skittles"}, "color": []string{"pink"}},
					{"brand": []string{"!skittles !m&ms"}, "color": []string{"red", "blue"}},
				},
			}
			Convey("building a list of variations should succeed", func() {
				vs, errs := buildMatrixVariants(axes, ase, []matrix{m})
				So(errs, ShouldBeNil)
				Convey("and return the correct list of combinations", func() {
					// ensure all values are in there...
					So(len(vs), ShouldEqual, 16)
					vals := []matrixValue{}
					for _, d := range vs {
						vals = append(vals, d.matrixVal)
					}
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "red"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "orange"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "yellow"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "green"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "blue"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "m&ms", "color": "brown"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "red"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "orange"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "yellow"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "green"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "skittles", "color": "purple"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "orange"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "yellow"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "green"})
					So(vals, ShouldContainResembling, matrixValue{"brand": "necco", "color": "purple"})
				})
			})
		})
		Convey("and a matrix that uses wrong axes", func() {
			m := matrix{
				Id: "candy",
				Spec: matrixDefinition{
					"strength": []string{"weak", "middle", "big-n-tough"},
				},
			}
			Convey("should fail to build", func() {
				vs, errs := buildMatrixVariants(axes, ase, []matrix{m})
				So(len(vs), ShouldEqual, 0)
				So(len(errs), ShouldEqual, 3)
			})
		})
		Convey("and a matrix that uses wrong axis values", func() {
			m := matrix{
				Id: "candy",
				Spec: matrixDefinition{
					"color": []string{"salmon", "infrared"},
				},
			}
			Convey("should fail to build", func() {
				vs, errs := buildMatrixVariants(axes, ase, []matrix{m})
				So(len(vs), ShouldEqual, 0)
				So(len(errs), ShouldEqual, 2)
			})
		})
	})
}

func TestMergeAxisValue(t *testing.T) {
	Convey("With a parserBV", t, func() {
		pbv := parserBV{
			RunOn:     []string{"basic_distro"},
			Modules:   []string{"basic_module"},
			Tags:      []string{"basic"},
			BatchTime: nil,
			Stepback:  nil,
			Expansions: map[string]string{
				"v1": "test",
			},
		}
		Convey("a valid axis value should merge successfully", func() {
			av := axisValue{
				RunOn:     []string{"special_distro"},
				Modules:   []string{"module++"},
				Tags:      []string{"enterprise"},
				BatchTime: new(int),
				Stepback:  new(bool),
				Variables: map[string]string{
					"v2": "new",
				},
			}
			So(pbv.mergeAxisValue(av), ShouldBeNil)
			So(pbv.RunOn, ShouldResemble, av.RunOn)
			So(pbv.Modules, ShouldResemble, av.Modules)
			So(pbv.Tags, ShouldContain, "basic")
			So(pbv.Tags, ShouldContain, "enterprise")
			So(pbv.Stepback, ShouldNotBeNil)
			So(pbv.BatchTime, ShouldNotBeNil)
			So(pbv.Expansions, ShouldResemble, util.Expansions{
				"v1": "test",
				"v2": "new",
			})
		})
		Convey("a valid axis value full of expansions should merge successfully", func() {
			av := axisValue{
				RunOn:   []string{"${v1}", "${v2}"},
				Modules: []string{"${v1}__"},
				Tags:    []string{"fat${v2}"},
				Variables: map[string]string{
					"v2": "${v1}!",
				},
			}
			So(pbv.mergeAxisValue(av), ShouldBeNil)
			So(pbv.RunOn, ShouldResemble, parserStringSlice{"test", "test!"})
			So(pbv.Modules, ShouldResemble, parserStringSlice{"test__"})
			So(pbv.Tags, ShouldContain, "basic")
			So(pbv.Tags, ShouldContain, "fattest!")
			So(pbv.Expansions, ShouldResemble, util.Expansions{
				"v1": "test",
				"v2": "test!",
			})
		})
		Convey("an axis value with a bad tag expansion should fail", func() {
			av := axisValue{
				Tags: []string{"fat${"},
			}
			So(pbv.mergeAxisValue(av), ShouldNotBeNil)
		})
		Convey("an axis value with a bad variables expansion should fail", func() {
			av := axisValue{
				Variables: map[string]string{
					"v2": "${sdsad",
				},
			}
			So(pbv.mergeAxisValue(av), ShouldNotBeNil)
		})
	})
}

func TestRulesEvaluation(t *testing.T) {
	Convey("With a series of test parserBVs and tasks", t, func() {
		taskDefs := []parserTask{
			{Name: "red", Tags: []string{"primary", "warm"}},
			{Name: "orange", Tags: []string{"secondary", "warm"}},
			{Name: "yellow", Tags: []string{"primary", "warm"}},
			{Name: "green", Tags: []string{"secondary", "cool"}},
			{Name: "blue", Tags: []string{"primary", "cool"}},
			{Name: "purple", Tags: []string{"secondary", "cool"}},
			{Name: "brown", Tags: []string{"tertiary"}},
			{Name: "black", Tags: []string{"special"}},
			{Name: "white", Tags: []string{"special"}},
		}
		tse := NewParserTaskSelectorEvaluator(taskDefs)
		Convey("a variant with a 'remove' rule should remove the given tasks", func() {
			bvs := []parserBV{{
				Name: "test",
				Tasks: parserBVTaskUnits{
					{Name: "blue"},
					{Name: ".special"},
					{Name: ".tertiary"},
				},
				matrixRules: []ruleAction{
					{RemoveTasks: []string{".primary !.warm"}}, //remove blue
					{RemoveTasks: []string{"brown"}},
				},
			}}
			evaluated, errs := evaluateBuildVariants(tse, nil, nil, bvs, taskDefs, nil)
			So(errs, ShouldBeNil)
			v1 := evaluated[0]
			So(v1.Name, ShouldEqual, "test")
			So(len(v1.Tasks), ShouldEqual, 2)
		})
		Convey("a variant with an 'add' rule should add the given tasks", func() {
			bvs := []parserBV{{
				Name: "test",
				Tasks: parserBVTaskUnits{
					{Name: ".special"},
				},
				matrixRules: []ruleAction{
					{AddTasks: []parserBVTaskUnit{{Name: ".primary"}}},
					{AddTasks: []parserBVTaskUnit{{Name: ".warm"}}},
					{AddTasks: []parserBVTaskUnit{{Name: "green", DependsOn: []parserDependency{{
						TaskSelector: taskSelector{Name: ".warm"},
					}}}}},
				},
			}}
			evaluated, errs := evaluateBuildVariants(tse, nil, nil, bvs, taskDefs, nil)
			So(errs, ShouldBeNil)
			v1 := evaluated[0]
			So(v1.Name, ShouldEqual, "test")
			So(len(v1.Tasks), ShouldEqual, 7)
		})
		Convey("a series of add and remove rules should execute in order", func() {
			bvs := []parserBV{{
				Name: "test",
				Tasks: parserBVTaskUnits{
					{Name: ".secondary"},
				},
				matrixRules: []ruleAction{
					{AddTasks: []parserBVTaskUnit{{Name: ".primary"}}},
					{RemoveTasks: []string{".secondary"}},
					{AddTasks: []parserBVTaskUnit{{Name: ".warm"}}},
					{RemoveTasks: []string{"orange"}},
					{AddTasks: []parserBVTaskUnit{{Name: "orange", DependsOn: []parserDependency{{
						TaskSelector: taskSelector{Name: ".warm"},
					}}}}},
				},
			}}
			evaluated, errs := evaluateBuildVariants(tse, nil, nil, bvs, taskDefs, nil)
			So(errs, ShouldBeNil)
			v1 := evaluated[0]
			So(v1.Name, ShouldEqual, "test")
			So(len(v1.Tasks), ShouldEqual, 4)
		})
		Convey("conflicting added tasks should fail", func() {
			bvs := []parserBV{{
				// case where conflicts take place against existing tasks
				Name: "test1",
				Tasks: parserBVTaskUnits{
					{Name: ".warm"},
				},
				matrixRules: []ruleAction{
					{AddTasks: []parserBVTaskUnit{{Name: "orange", DependsOn: []parserDependency{{
						TaskSelector: taskSelector{Name: ".warm"},
					}}}}},
				},
			}, {
				// case where conflicts are within the same rule
				Name:  "test2",
				Tasks: parserBVTaskUnits{},
				matrixRules: []ruleAction{
					{AddTasks: []parserBVTaskUnit{{Name: ".warm"}, {Name: "orange", DependsOn: []parserDependency{{
						TaskSelector: taskSelector{Name: ".warm"},
					}}}}},
				},
			}}
			_, errs := evaluateBuildVariants(tse, nil, nil, bvs, taskDefs, nil)
			So(errs, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 3)
		})
		Convey("a 'remove' rule for an unknown task should fail", func() {
			bvs := []parserBV{{
				Name: "test",
				Tasks: parserBVTaskUnits{
					{Name: "blue"},
					{Name: ".special"},
					{Name: ".tertiary"},
				},
				matrixRules: []ruleAction{
					{RemoveTasks: []string{".amazing"}}, //remove blue
					{RemoveTasks: []string{"rainbow"}},
				},
			}}
			_, errs := evaluateBuildVariants(tse, nil, nil, bvs, taskDefs, nil)
			So(errs, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 2)
		})
	})
}
