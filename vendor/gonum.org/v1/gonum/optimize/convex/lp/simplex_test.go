// Copyright ©2016 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lp

import (
	"math/rand"
	"testing"

	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

const convergenceTol = 1e-10

func TestSimplex(t *testing.T) {
	// First test specific inputs. These were collected from failures
	// during randomized testing.
	// TODO(btracey): Test specific problems with known solutions.
	for _, test := range []struct {
		A            mat.Matrix
		b            []float64
		c            []float64
		tol          float64
		initialBasic []int
	}{
		{
			// Basic feasible LP
			A: mat.NewDense(2, 4, []float64{
				-1, 2, 1, 0,
				3, 1, 0, 1,
			}),
			b: []float64{4, 9},
			c: []float64{-1, -2, 0, 0},
			//initialBasic: nil,
			tol: 0,
		},
		{
			// Zero row that caused linear solver failure
			A: mat.NewDense(3, 5, []float64{0.09917822373225804, 0, 0, -0.2588175087223661, -0.5935518220870567, 1.301111422556007, 0.12220247487326946, 0, 0, -1.9194869979254463, 0, 0, 0, 0, -0.8588221231396473}),
			b: []float64{0, 0, 0},
			c: []float64{0, 0.598992624019304, 0, 0, 0},
		},
		{
			// Case that caused linear solver failure
			A: mat.NewDense(13, 26, []float64{-0.7001209024399848, -0.7027502615621812, -0, 0.7354444798695736, -0, 0.476457578966189, 0.7001209024399848, 0.7027502615621812, 0, -0.7354444798695736, 0, -0.476457578966189, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1.8446087238391438, -0, -0, 0.7705609478497938, -0, -0, -2.7311218710244463, 0, 0, -0.7705609478497938, 0, 0, 2.7311218710244463, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, -0, 0.8332519091897401, 0.7762132098737671, -0, -0, -0.052470638647269585, 0, -0.8332519091897401, -0.7762132098737671, 0, 0, 0.052470638647269585, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.9898577208292023, 0.31653724289408824, -0, -0, 0.17797227766447388, 1.2702427184954932, -0.7998764021535656, -0.31653724289408824, 0, 0, -0.17797227766447388, -1.2702427184954932, 0.7998764021535656, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, -0, -0, -0, -0, 0.4206278126213235, -0.7253374879437113, 0, 0, 0, 0, -0.4206278126213235, 0.7253374879437113, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, -1, -0, -0.7567988418466963, 0.3304567624749696, 0.8385927625193501, -0.0021606686026376387, -0, 0, 0.7567988418466963, -0.3304567624749696, -0.8385927625193501, 0.0021606686026376387, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, -2.230107839590404, -0.9897104202085316, -0, 0.24703471683023603, -0, -2.382860345431941, 0.6206871648345162, 0.9897104202085316, 0, -0.24703471683023603, 0, 2.382860345431941, -0.6206871648345162, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, -1, 1.4350469221322282, -0.9730343818431852, -0, 2.326429855201535, -0, -0.14347849887004038, -1.4350469221322282, 0.9730343818431852, 0, -2.326429855201535, 0, 0.14347849887004038, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, -0.7943912888763849, -0.13735037357335078, -0.5101161104860161, -0, -0, -1.4790634590370297, 0.050911195996747316, 0.13735037357335078, 0.5101161104860161, 0, 0, 1.4790634590370297, -0.050911195996747316, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, -1, -0, -0.2515400440591492, 0.2058339272568599, -0, -0, -1.314023802253438, 0, 0.2515400440591492, -0.2058339272568599, 0, 0, 1.314023802253438, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, -1, 0.08279503413614919, -0.16669071891829756, -0, -0.6208413721884664, -0, -0.6348258970402827, -0.08279503413614919, 0.16669071891829756, 0, 0.6208413721884664, 0, 0.6348258970402827, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, -1, -0, -0, 0.49634739711260845, -0, -0, -0, 0, 0, -0.49634739711260845, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, -0.2797437186631715, -0.8356683570259136, 1.8970426594969672, -0.4095711945594497, 0.45831284820623924, -0.6109615338552246, 0.2797437186631715, 0.8356683570259136, -1.8970426594969672, 0.4095711945594497, -0.45831284820623924, 0.6109615338552246, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, -1}),
			b: []float64{-0.8446087238391436, 0, 1.9898577208292023, 0, 0, -2.230107839590404, 0, 0.20560871112361512, 0, 0, 0, 0, 0},
			c: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		},
		{
			// Phase 1 of the above that panicked
			A: mat.NewDense(26, 52, []float64{0.7001209024399848, -0, -0, -0.31653724289408824, -0, -0, 0.9897104202085316, -1.4350469221322282, 0.13735037357335078, -0, -0.08279503413614919, -0, 0.2797437186631715, -0.7001209024399848, 0, 0, 0.31653724289408824, 0, 0, -0.9897104202085316, 1.4350469221322282, -0.13735037357335078, 0, 0.08279503413614919, 0, -0.2797437186631715, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.7027502615621812, -0, -0.8332519091897401, -0, -0, 0.7567988418466963, -0, 0.9730343818431852, 0.5101161104860161, 0.2515400440591492, 0.16669071891829756, -0, 0.8356683570259136, -0.7027502615621812, 0, 0.8332519091897401, 0, 0, -0.7567988418466963, 0, -0.9730343818431852, -0.5101161104860161, -0.2515400440591492, -0.16669071891829756, 0, -0.8356683570259136, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0.7705609478497938, -0.7762132098737671, -0, -0, -0.3304567624749696, -0.24703471683023603, -0, -0, -0.2058339272568599, -0, -0.49634739711260845, -1.8970426594969672, 0, 0.7705609478497938, 0.7762132098737671, 0, 0, 0.3304567624749696, 0.24703471683023603, 0, 0, 0.2058339272568599, 0, 0.49634739711260845, 1.8970426594969672, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0.7354444798695736, -0, -0, -0.17797227766447388, -0, -0.8385927625193501, -0, -2.326429855201535, -0, -0, 0.6208413721884664, -0, 0.4095711945594497, 0.7354444798695736, 0, 0, 0.17797227766447388, 0, 0.8385927625193501, 0, 2.326429855201535, 0, 0, -0.6208413721884664, 0, -0.4095711945594497, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, -1.2702427184954932, -0.4206278126213235, 0.0021606686026376387, 2.382860345431941, -0, 1.4790634590370297, -0, -0, -0, -0.45831284820623924, 0, 0, 0, 1.2702427184954932, 0.4206278126213235, -0.0021606686026376387, -2.382860345431941, 0, -1.4790634590370297, 0, 0, 0, 0.45831284820623924, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0.476457578966189, 2.7311218710244463, 0.052470638647269585, 0.7998764021535656, 0.7253374879437113, -0, -0.6206871648345162, 0.14347849887004038, -0.050911195996747316, 1.314023802253438, 0.6348258970402827, -0, 0.6109615338552246, 0.476457578966189, -2.7311218710244463, -0.052470638647269585, -0.7998764021535656, -0.7253374879437113, 0, 0.6206871648345162, -0.14347849887004038, 0.050911195996747316, -1.314023802253438, -0.6348258970402827, 0, -0.6109615338552246, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0.7001209024399848, -0, -0, 0.31653724289408824, -0, -0, -0.9897104202085316, 1.4350469221322282, -0.13735037357335078, -0, 0.08279503413614919, -0, -0.2797437186631715, 0.7001209024399848, 0, 0, -0.31653724289408824, 0, 0, 0.9897104202085316, -1.4350469221322282, 0.13735037357335078, 0, -0.08279503413614919, 0, 0.2797437186631715, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0.7027502615621812, -0, 0.8332519091897401, -0, -0, -0.7567988418466963, -0, -0.9730343818431852, -0.5101161104860161, -0.2515400440591492, -0.16669071891829756, -0, -0.8356683570259136, 0.7027502615621812, 0, -0.8332519091897401, 0, 0, 0.7567988418466963, 0, 0.9730343818431852, 0.5101161104860161, 0.2515400440591492, 0.16669071891829756, 0, 0.8356683570259136, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, 0.7705609478497938, 0.7762132098737671, -0, -0, 0.3304567624749696, 0.24703471683023603, -0, -0, 0.2058339272568599, -0, 0.49634739711260845, 1.8970426594969672, 0, -0.7705609478497938, -0.7762132098737671, 0, 0, -0.3304567624749696, -0.24703471683023603, 0, 0, -0.2058339272568599, 0, -0.49634739711260845, -1.8970426594969672, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.7354444798695736, -0, -0, 0.17797227766447388, -0, 0.8385927625193501, -0, 2.326429855201535, -0, -0, -0.6208413721884664, -0, -0.4095711945594497, -0.7354444798695736, 0, 0, -0.17797227766447388, 0, -0.8385927625193501, 0, -2.326429855201535, 0, 0, 0.6208413721884664, 0, 0.4095711945594497, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, 1.2702427184954932, 0.4206278126213235, -0.0021606686026376387, -2.382860345431941, -0, -1.4790634590370297, -0, -0, -0, 0.45831284820623924, 0, 0, 0, -1.2702427184954932, -0.4206278126213235, 0.0021606686026376387, 2.382860345431941, 0, 1.4790634590370297, 0, 0, 0, -0.45831284820623924, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0.476457578966189, -2.7311218710244463, -0.052470638647269585, -0.7998764021535656, -0.7253374879437113, -0, 0.6206871648345162, -0.14347849887004038, 0.050911195996747316, -1.314023802253438, -0.6348258970402827, -0, -0.6109615338552246, -0.476457578966189, 2.7311218710244463, 0.052470638647269585, 0.7998764021535656, 0.7253374879437113, 0, -0.6206871648345162, 0.14347849887004038, -0.050911195996747316, 1.314023802253438, 0.6348258970402827, 0, 0.6109615338552246, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -1, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -1, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, -1, -0, -0, -0, -0, -0, -0, -0, -0, -0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, -0, -1, -0, -0, -0, -0, -0, -0, -0, -0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, -0, -0, -1, -0, -0, -0, -0, -0, -0, -0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, -0, -0, -0, -1, -0, -0, -0, -0, -0, -0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, -0, -0, -0, -0, -0, -0, -0, -1, -0, -0, -0, -0, -0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, -0, -0, -0, -0, -0, -0, -0, -0, -1, -0, -0, -0, -0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -1, -0, -0, -0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -1, -0, -0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -1, -0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -0, -1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1.8446087238391438, 1, -0.9898577208292023, 1, 1, 2.230107839590404, 1, 0.7943912888763849, 1, 1, 1, 1, 1, -1.8446087238391438, -1, 0.9898577208292023, -1, -1, -2.230107839590404, -1, -0.7943912888763849, -1, -1, -1, -1, -1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
			b: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			c: []float64{-0.8446087238391436, 0, 1.9898577208292023, 0, 0, -2.230107839590404, 0, 0.20560871112361512, 0, 0, 0, 0, 0, 0.8446087238391436, -0, -1.9898577208292023, -0, -0, 2.230107839590404, -0, -0.20560871112361512, -0, -0, -0, -0, -0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			// Dense case that panicked.
			A: mat.NewDense(6, 15, []float64{0.3279477313560112, 0.04126296122557327, 0.24121743535067522, -0.8676933623438741, -0.3279477313560112, -0.04126296122557327, -0.24121743535067522, 0.8676933623438741, 1, 0, 0, 0, 0, 0, 1.3702148909442915, 0.43713186538468607, 0.8613818492485417, -0.9298615442657688, -0.037784779008231184, -0.43713186538468607, -0.8613818492485417, 0.9298615442657688, 0.037784779008231184, 0, 1, 0, 0, 0, 0, 0.3478112701177931, -0, 0.748352668598051, -0.4294796840343912, -0, 0, -0.748352668598051, 0.4294796840343912, 0, 0, 0, 1, 0, 0, 0, -1, -0, 1.1913912184457485, 1.732132186658447, 0.4026384828544584, 0, -1.1913912184457485, -1.732132186658447, -0.4026384828544584, 0, 0, 0, 1, 0, 0, -0.4598555419763902, -0, 1.2088959976921831, -0.7297794575275871, 1.9835614149566971, 0, -1.2088959976921831, 0.7297794575275871, -1.9835614149566971, 0, 0, 0, 0, 1, 0, 0.5986809560819324, 0.19738159369304414, -1.0647198575836367, -0, -0.7264943883762761, -0.19738159369304414, 1.0647198575836367, 0, 0.7264943883762761, 0, 0, 0, 0, 0, 1, 0.36269644970561576}),
			b: []float64{2.3702148909442915, 1.3478112701177931, 0, -0.4598555419763902, 1.5986809560819324, 1.3626964497056158},
			c: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		},
		{
			A:            mat.NewDense(6, 11, []float64{-0.036551083288733854, -0.8967234664797694, 0.036551083288733854, 0.8967234664797694, 1, 0, 0, 0, 0, 0, -0.719908817815329, -1.9043311904524263, -0, 1.9043311904524263, 0, 0, 1, 0, 0, 0, 0, -1.142213296802784, -0, 0.17584914855696687, 0, -0.17584914855696687, 0, 0, 1, 0, 0, 0, -0.5423586338987796, -0.21663357118058713, -0.4815354890024489, 0.21663357118058713, 0.4815354890024489, 0, 0, 0, 1, 0, 0, -0.6864090947259134, -0, -0, 0, 0, 0, 0, 0, 0, 1, 0, 0.4091621839837596, -1.1853040616164046, -0.11374085137543871, 1.1853040616164046, 0.11374085137543871, 0, 0, 0, 0, 0, 1, -1.7416078575675549}),
			b:            []float64{0.28009118218467105, -0.14221329680278405, 0.4576413661012204, 0.3135909052740866, 1.4091621839837596, -1.7416078575675549},
			c:            []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			initialBasic: []int{10, 8, 7, 6, 5, 4},
		},
		{
			A:            mat.NewDense(6, 10, []float64{-0.036551083288733854, -0.8967234664797694, 0.036551083288733854, 0.8967234664797694, 1, 0, 0, 0, 0, 0, -1.9043311904524263, -0, 1.9043311904524263, 0, 0, 1, 0, 0, 0, 0, -0, 0.17584914855696687, 0, -0.17584914855696687, 0, 0, 1, 0, 0, 0, -0.21663357118058713, -0.4815354890024489, 0.21663357118058713, 0.4815354890024489, 0, 0, 0, 1, 0, 0, -0, -0, 0, 0, 0, 0, 0, 0, 1, 0, -1.1853040616164046, -0.11374085137543871, 1.1853040616164046, 0.11374085137543871, 0, 0, 0, 0, 0, 1}),
			b:            []float64{0.28009118218467105, -0.14221329680278405, 0.4576413661012204, 0.3135909052740866, 1.4091621839837596, -1.7416078575675549},
			c:            []float64{-1.1951160054922971, -1.354633418345746, 1.1951160054922971, 1.354633418345746, 0, 0, 0, 0, 0, 0},
			initialBasic: []int{0, 8, 7, 6, 5, 4},
		},
		{
			A:            mat.NewDense(6, 14, []float64{-0.4398035705048233, -0, -1.1190414559968929, -0, 0.4398035705048233, 0, 1.1190414559968929, 0, 1, 0, 0, 0, 0, 0, -0, 0.45892918156139395, -0, -0, 0, -0.45892918156139395, 0, 0, 0, 1, 0, 0, 0, 0, -0, -0, -0.3163051515958635, -0, 0, 0, 0.3163051515958635, 0, 0, 0, 1, 0, 0, 0, -0, -0, -1.8226051692445888, -0.8154477101733032, 0, 0, 1.8226051692445888, 0.8154477101733032, 0, 0, 0, 1, 0, 0, -0, 1.0020104354806922, -2.80863692523519, -0.8493721031516384, 0, -1.0020104354806922, 2.80863692523519, 0.8493721031516384, 0, 0, 0, 0, 1, 0, -0.8292937871394104, -1.4615144665021647, -0, -0, 0.8292937871394104, 1.4615144665021647, 0, 0, 0, 0, 0, 0, 0, 1}),
			b:            []float64{0, -1.0154749704172474, 0, 0, 0, -1.5002324315812783},
			c:            []float64{1.0665389045026794, 0.097366273706136, 0, 2.7928153636989954, -1.0665389045026794, -0.097366273706136, -0, -2.7928153636989954, 0, 0, 0, 0, 0, 0},
			initialBasic: []int{5, 12, 11, 10, 0, 8},
		},
		{
			// Bad Phase I setup.
			A: mat.NewDense(6, 7, []float64{1.4009742075419371, 0, 0.05737255493210325, -2.5954004393412915, 0, 1.561789236911904, 0, 0.17152506517602673, 0, 0, 0, 0, 0, -0.3458126550149948, 1.900744052464951, -0.32773164134097343, -0.9648201331251137, 0, 0, 0, 0, -1.3229549190526497, 0.0692227703722903, 0, 0, -0.1024297720479933, 0.4550740188869777, 0, 0.013599438965679167, 0, 0, 0, 0, 0, -0.1164365105021209, 0, 0, 0.4077091957443405, 1.5682816151954875, 0.8411734682369051, 0.22379142247562167, 1.2206581060250778}),
			b: []float64{0.3293809220378252, 0, -0.5688424847664554, 0, 0, 1.4832526082339592},
			c: []float64{0.5246370956983506, -0.36608819899109946, 1.5854141981237713, 0.5170486527020665, 0, 1.4006819866163691, 0.7733814538809437},
		},
		{
			// The problem is feasible, but the PhaseI problem keeps the last
			// variable in the basis.
			A:            mat.NewDense(2, 3, []float64{0.7171320440380402, 0, 0.22818288617480836, 0, -0.10030202006494793, -0.3282372661549324}),
			b:            []float64{0.8913013436978257, 0},
			c:            []float64{0, 0, 1.16796158316812},
			initialBasic: nil,
		},
		{
			// Case where primal was returned as feasible, but the dual was returned
			// as infeasible. This is the dual.
			// Here, the phase I problem returns the value in the basis but equal
			// to epsilon and not 0.
			A: mat.NewDense(5, 11, []float64{0.48619717875196006, 0.5089083769874058, 1.4064796473022745, -0.48619717875196006, -0.5089083769874058, -1.4064796473022745, 1, 0, 0, 0, 0, 1.5169837857318682, -0, -0, -1.5169837857318682, 0, 0, 0, 1, 0, 0, 0, -1.3096160896447528, 0.12600426735917414, 0.296082394213142, 1.3096160896447528, -0.12600426735917414, -0.296082394213142, 0, 0, 1, 0, 0, -0, -0, 1.9870800277141467, 0, 0, -1.9870800277141467, 0, 0, 0, 1, 0, -0.3822356988571877, -0, -0.1793908926957139, 0.3822356988571877, 0, 0.1793908926957139, 0, 0, 0, 0, 1}),
			b: []float64{0.6015865977347667, 0, -1.5648780993757594, 0, 0},
			c: []float64{-0.642801659201449, -0.5412741400343285, -1.4634460998530177, 0.642801659201449, 0.5412741400343285, 1.4634460998530177, 0, 0, 0, 0, 0},
		},
		{
			// Caused linear solve error. The error is because replacing the minimum
			// index in Bland causes the new basis to be singular. This
			// necessitates the ending loop in bland over possible moves.
			A: mat.NewDense(9, 23, []float64{-0.898219823758102, -0, -0, -0, 1.067555075209233, 1.581598470243863, -1.0656096883610071, 0.898219823758102, 0, 0, 0, -1.067555075209233, -1.581598470243863, 1.0656096883610071, 1, 0, 0, 0, 0, 0, 0, 0, 0, -1.5657353278668433, 0.5798888118401012, -0, 0.14560553520321928, -0, -0, -0, 1.5657353278668433, -0.5798888118401012, 0, -0.14560553520321928, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, -0, -0, -1.5572250142582087, -0, -0, -0, -0, 0, 0, 1.5572250142582087, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, -0, -0, -0, -1.1266215512973428, -0, 1.0661059397023553, -0, 0, 0, 0, 1.1266215512973428, 0, -1.0661059397023553, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, -0, -2.060232129551813, 1.756900609902372, -0, -0, -0, -0, 0, 2.060232129551813, -1.756900609902372, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1.0628806512935949, -0, -0, 0.3306985942820342, -0, 0.5013194822231914, -0, -1.0628806512935949, 0, 0, -0.3306985942820342, 0, -0.5013194822231914, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, -0.02053916418367785, 2.0967009672108627, -0, 1.276296057052031, -0, -0.8396554873675388, -0, 0.02053916418367785, -2.0967009672108627, 0, -1.276296057052031, 0, 0.8396554873675388, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, -1.5173172721095745, -0, -0, -0, -0, -0.7781977786718928, -0.08927683907374018, 1.5173172721095745, 0, 0, 0, 0, 0.7781977786718928, 0.08927683907374018, 0, 0, 0, 0, 0, 0, 0, 1, 0, -0, -0, -0, -0, -0, 0.39773149008355624, -0, 0, 0, 0, 0, 0, -0.39773149008355624, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
			b: []float64{0, 0, 0, 0, 0, 0, 0, 0, 0},
			c: []float64{0.24547850255842107, -0.9373919913433648, 0, 0, 0, 0.2961224049153204, 0, -0.24547850255842107, 0.9373919913433648, -0, -0, -0, -0.2961224049153204, -0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			// Caused error because ALL of the possible replacements in Bland cause.
			// ab to be singular. This necessitates outer loop in bland over possible
			// moves.
			A:            mat.NewDense(9, 23, []float64{0.6595219196440785, -0, -0, -1.8259394918781682, -0, -0, 0.005457361044175046, -0.6595219196440785, 0, 0, 1.8259394918781682, 0, 0, -0.005457361044175046, 1, 0, 0, 0, 0, 0, 0, 0, 0, -0, -0.10352878714214864, -0, -0, -0, -0, 0.5945016966696087, 0, 0.10352878714214864, 0, 0, 0, 0, -0.5945016966696087, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0.31734882842876444, -0, -0, -0, -0, -0, -0.716633126367685, -0.31734882842876444, 0, 0, 0, 0, 0, 0.716633126367685, 0, 0, 1, 0, 0, 0, 0, 0, 0, -0.7769812182932578, -0, -0.17370050158829553, 0.19405062263734607, -0, 1.1472330031002533, -0.6776631768730962, 0.7769812182932578, 0, 0.17370050158829553, -0.19405062263734607, 0, -1.1472330031002533, 0.6776631768730962, 0, 0, 0, 1, 0, 0, 0, 0, 0, -0, 0.8285611486611473, -0, -0, -0, -0, -0, 0, -0.8285611486611473, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, -2.088953453647358, 1.3286488791152795, -0, -0, -0, -0, 0.9147833235021142, 2.088953453647358, -1.3286488791152795, 0, 0, 0, 0, -0.9147833235021142, 0, 0, 0, 0, 0, 1, 0, 0, 0, -0, -0, -0, -0, 0.6560365621262937, -0, -0, 0, 0, 0, 0, -0.6560365621262937, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0.8957188338098074, -0, -0, -0, -0, -0, -0, -0.8957188338098074, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, -0.2761381891117365, -0, -0, -0, 1.1154921426237823, 0.06429872020552618, -0, 0.2761381891117365, 0, 0, 0, -1.1154921426237823, -0.06429872020552618, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
			b:            []float64{0, 0, 0, 0, 0.5046208538522362, 1.0859412982429362, -2.066283584195025, 0, -0.2604305274353169},
			c:            []float64{0, 0, 0, 0, 0, 0, -0.05793762969330718, -0, -0, -0, -0, -0, -0, 0.05793762969330718, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			initialBasic: []int{22, 11, 7, 19, 18, 17, 16, 15, 14},
		},
		{
			// Caused initial supplied basis of Phase I to be singular.
			A: mat.NewDense(7, 11, []float64{0, 0, 0, 0, 0, 0.6667874223914787, -0.04779440888372957, -0.810020924434026, 0, 1.4190243477163373, 0, 0, 1.0452496826112936, 1.1966134226828076, 0, 0, 0, 0, -0.676136041089015, 0, 0, 0, 0, 0, 0, 0, 0, 0, -2.123232807871834, 0.2795467733707712, 0.21997115467272987, 0, -0.1572003980840453, 0, 0, 0, 0, 0.5130196002804861, 0, -0.005957174211761673, 0.3262874931735277, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1.5582052881594286, 0, 0.3544026193217651, 0, -1.0761986709145068, 0, 0.2438593072108347, 0, 0, 0, 0, 1.387509848081664, 0, 0, 0.3958750570508226, 1.6281679612990678, 0, 0, -0.24638311667922103, 0, 0, 0, 0, 0, 0, -0.628850893994423}),
			b: []float64{0.4135281763629115, 0, 0, 0, 0, 0, 0},
			c: []float64{0.5586772876113472, 0, 0.14261332948424457, 0, -0.016394076753000086, -0.506087285562544, 0, 0.37619482505459145, 1.2943822852419233, 0.5887960293578207, 0},
		},
	} {
		testSimplex(t, test.initialBasic, test.c, test.A, test.b, convergenceTol)
	}

	rnd := rand.New(rand.NewSource(1))
	// Randomized tests
	testRandomSimplex(t, 20000, 0.7, 10, rnd)
	testRandomSimplex(t, 20000, 0, 10, rnd)
	testRandomSimplex(t, 200, 0, 100, rnd)
	testRandomSimplex(t, 2, 0, 400, rnd)
}

func testRandomSimplex(t *testing.T, nTest int, pZero float64, maxN int, rnd *rand.Rand) {
	// Try a bunch of random LPs
	for i := 0; i < nTest; i++ {
		n := rnd.Intn(maxN) + 2 // n must be at least two.
		m := rnd.Intn(n-1) + 1  // m must be between 1 and n
		if m == 0 || n == 0 {
			continue
		}
		randValue := func() float64 {
			//var pZero float64
			v := rnd.Float64()
			if v < pZero {
				return 0
			}
			return rnd.NormFloat64()
		}
		a := mat.NewDense(m, n, nil)
		for i := 0; i < m; i++ {
			for j := 0; j < n; j++ {
				a.Set(i, j, randValue())
			}
		}
		b := make([]float64, m)
		for i := range b {
			b[i] = randValue()
		}

		c := make([]float64, n)
		for i := range c {
			c[i] = randValue()
		}

		testSimplex(t, nil, c, a, b, convergenceTol)
	}
}

func testSimplex(t *testing.T, initialBasic []int, c []float64, a mat.Matrix, b []float64, convergenceTol float64) error {
	primalOpt, primalX, _, errPrimal := simplex(initialBasic, c, a, b, convergenceTol)
	if errPrimal == nil {
		// No error solving the simplex, check that the solution is feasible.
		var bCheck mat.VecDense
		bCheck.MulVec(a, mat.NewVecDense(len(primalX), primalX))
		if !mat.EqualApprox(&bCheck, mat.NewVecDense(len(b), b), 1e-10) {
			t.Errorf("No error in primal but solution infeasible")
		}
	}

	primalInfeasible := errPrimal == ErrInfeasible
	primalUnbounded := errPrimal == ErrUnbounded
	primalBounded := errPrimal == nil
	primalASingular := errPrimal == ErrSingular
	primalZeroRow := errPrimal == ErrZeroRow
	primalZeroCol := errPrimal == ErrZeroColumn

	primalBad := !primalInfeasible && !primalUnbounded && !primalBounded && !primalASingular && !primalZeroRow && !primalZeroCol

	// It's an error if it's not one of the known returned errors. If it's
	// singular the problem is undefined and so the result cannot be compared
	// to the dual.
	if errPrimal == ErrSingular || primalBad {
		if primalBad {
			t.Errorf("non-known error returned: %s", errPrimal)
		}
		return errPrimal
	}

	// Compare the result to the answer found from solving the dual LP.

	// Construct and solve the dual LP.
	// Standard Form:
	//  minimize c^T * x
	//    subject to  A * x = b, x >= 0
	// The dual of this problem is
	//  maximize -b^T * nu
	//   subject to A^T * nu + c >= 0
	// Which is
	//   minimize b^T * nu
	//   subject to -A^T * nu <= c

	negAT := &mat.Dense{}
	negAT.Clone(a.T())
	negAT.Scale(-1, negAT)
	cNew, aNew, bNew := Convert(b, negAT, c, nil, nil)

	dualOpt, dualX, _, errDual := simplex(nil, cNew, aNew, bNew, convergenceTol)
	if errDual == nil {
		// Check that the dual is feasible
		var bCheck mat.VecDense
		bCheck.MulVec(aNew, mat.NewVecDense(len(dualX), dualX))
		if !mat.EqualApprox(&bCheck, mat.NewVecDense(len(bNew), bNew), 1e-10) {
			t.Errorf("No error in dual but solution infeasible")
		}
	}

	// Check about the zero status.
	if errPrimal == ErrZeroRow || errPrimal == ErrZeroColumn {
		return errPrimal
	}

	// If the primal problem is feasible, then the primal and the dual should
	// be the same answer. We have flopped the sign in the dual (minimizing
	// b^T *nu instead of maximizing -b^T*nu), so flip it back.
	if errPrimal == nil {
		if errDual != nil {
			t.Errorf("Primal feasible but dual errored: %s", errDual)
		}
		dualOpt *= -1
		if !floats.EqualWithinAbsOrRel(dualOpt, primalOpt, convergenceTol, convergenceTol) {
			t.Errorf("Primal and dual value mismatch. Primal %v, dual %v.", primalOpt, dualOpt)
		}
	}
	// If the primal problem is unbounded, then the dual should be infeasible.
	if errPrimal == ErrUnbounded && errDual != ErrInfeasible {
		t.Errorf("Primal unbounded but dual not infeasible. ErrDual = %s", errDual)
	}

	// If the dual is unbounded, then the primal should be infeasible.
	if errDual == ErrUnbounded && errPrimal != ErrInfeasible {
		t.Errorf("Dual unbounded but primal not infeasible. ErrDual = %s", errPrimal)
	}

	// If the primal is infeasible, then the dual should be either infeasible
	// or unbounded.
	if errPrimal == ErrInfeasible {
		if errDual != ErrUnbounded && errDual != ErrInfeasible && errDual != ErrZeroColumn {
			t.Errorf("Primal infeasible but dual not infeasible or unbounded: %s", errDual)
		}
	}

	return errPrimal
}
