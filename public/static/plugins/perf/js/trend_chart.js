// Helper function
// TODO Make it accept (d, i) and return function (FP)
function d3Translate(x, y) {
  if (y === undefined) y = x
  return 'translate(' + x + ',' + y + ')';
}

mciModule.factory('PerfChartService', function() {
  var cfg = {
    container: {
      width: 960,
      height: 222
    },
    margin: {
      top: 12,
      right: 50,
      bottom: 60,
      left: 120
    },
    effectiveWidth: undefined, // to be calculated
    effectiveHeight: undefined, // to be calculated
    points: {
      focusedR: 4.5,
      changePointSize: 12,
    },
    toolTip: { // For change points
      interlineSpace: 20,
      width: 165,
      margin: 15,
      xOffset: -65,
      yOffset: 0,
      colOffset: [0, 45, 50, 65],
      height: undefined, // to be calculated
      labels: [{
        key: 'nobs',
        display: '#',
      }, {
        key: function(d) { return d.minmax[0] },
        display: 'Min',
      }, {
        key: function(d) { return d.minmax[1] },
        display: 'Max',
      }, {
        key: 'mean',
        display: 'Mean',
      }, {
        key: 'variance',
        display: 'Variance',
      }, {
        key: 'bfs',
        display: 'Tickets',
        nonStat: true,
        formatter: function(val) { return (
          val.length == 0 ? 'None' :
          val.length == 1 ? val[0].key :
          /* else */        val[0].key + ' and ' + val.length - 1 + ' more'
        )},
      }, {
        key: 'skewness',
        display: 'Skewness',
        visible: false,
      }, {
        key: 'kurtosis',
        display: 'Kurtosis',
        visible: false,
      }].filter(function(d) {
        // Filter out non-visible items
        return d.visible !== false
      }).map(function(d, i) {
        // Enumerate all items (required for rendering heterogeneous lists by d3js)
        d.idx = i
        return d
      }),
    },
    valueAttr: 'ops_per_sec',
    yAxis: {
      ticks: 5,
      gap: 10 // White space between axis and chart
    },
    xAxis: {
      maxTicks: 10,
      format: 'MMM DD',
      labelYOffset: 20,
    },
    focus: {
      labelOffset: {
        x: 6,
        y: -5,
        between: 15, // Minimal vertical space between ops labels
      }
    },
    format: {
      date: 'll'
    },
    legend: {
      itemHeight: 20,
      itemWidth: 35,
      gap: 10, // between legen items
      yOffset: 10, // To bootom
      textOverRectOffset: -15, // Aligns legend item with the text label
      yPos: undefined, // To be calculated
      xPos: undefined, // To be calculated
      step: undefined, // to e calculated. Step between legend items
    },
    knownLevels: {
        1: { colorId: 0 },
        2: { colorId: 9 },
        4: { colorId: 1 },
        8: { colorId: 2 },
       16: { colorId: 3 },
       32: { colorId: 4 },
       64: { colorId: 5 },
      128: { colorId: 8 },
      256: { colorId: 6 },
      512: { colorId: 7 },
    },
    formatters: {
      large: d3.format(',.0f'), // grouped thousands, no significant digits
      digits_1: d3.format('.01f'), // floating point 1 digits
      digits_2: d3.format('.02f'), // floating point 2 digits
      digits_3: d3.format('.03f'), // floating point 3 digits
      si: d3.format(',s'), // si notation
    }
  };

  // Non-persistent color id offset for unusual thread level
  cfg.knownLevelsCount = _.keys(cfg.knownLevels).length

  // Chart are real size
  cfg.effectiveWidth = cfg.container.width - cfg.margin.left - cfg.margin.right;
  cfg.effectiveHeight = cfg.container.height - cfg.margin.top - cfg.margin.bottom;

  // Tool Tip
  cfg.toolTip.height = cfg.toolTip.interlineSpace * cfg.toolTip.labels.length

  // Legend y pos
  cfg.legend.yPos = (
    cfg.container.height - cfg.legend.yOffset
    - cfg.legend.itemHeight - cfg.legend.textOverRectOffset
  )

  cfg.legend.step = cfg.legend.itemWidth + cfg.legend.gap

  // Returns list of y-positions of ops labels for given
  // yScaledValues list.
  function getOpsLabelYPosition(vals, cfg) {
    var yScaledValues = _.sortBy(vals, function(d) { return -d })

    // Calculate the most top (the last) label position.
    // Also checks top margin overlap
    var currentVal = _.last(yScaledValues)
    var prevPos = currentVal + cfg.focus.labelOffset.y
    if (prevPos < cfg.margin.top) {
      prevPos = cfg.margin.top + 5
    }
    var textPosList = []
    textPosList[_.indexOf(vals, currentVal)] = prevPos

    // Calculate all other items positions, based on previous item position
    // Loop skips the last item (see code above)
    for (var i = yScaledValues.length - 2; i >= 0; i--) {
      var currentVal = yScaledValues[i]
      var currentPos = currentVal + cfg.focus.labelOffset.y;
      var delta = prevPos - currentPos;
      // If labels overlapping, move the label below previous label
      var newPos = (delta > -cfg.focus.labelOffset.between)
        ? prevPos + cfg.focus.labelOffset.between
        : currentPos
      prevPos = newPos
      textPosList[_.indexOf(vals, currentVal)] = newPos
    }

    return textPosList;
  }

  // Pair for getValueForAllLevels function
  // This curried function returns value for given
  // thread `level` (ignored in this case) and series `item`
  function getValueForMaxOnly() {
    return function(item) {
      return item[cfg.valueAttr]
    }
  }

  // Pair for getValueForMaxOnly function
  // This curried function returns value for given
  // thread `level` and series `item`
  function getValueForAllLevels(level) {
    return function(item) {
      var result = _.findWhere(item.threadResults, {threadLevel: level.name})
      return result
        ? result[cfg.valueAttr]
        : null
    }
  }

  return {
    cfg: cfg,
    getOpsLabelYPosition: getOpsLabelYPosition,
    getValueForMaxOnly: getValueForMaxOnly,
    getValueForAllLevels: getValueForAllLevels,
  }
})

// TODO Create AngularJS directive
mciModule.factory('DrawPerfTrendChart', function (
  PerfChartService
) {
  var MAXONLY = 'maxonly';

  return function(params) {

    // Extract params
    var series = params.series,
        changePoints = params.changePoints,
        buildFailures = params.buildFailures,
        key = params.key,
        scope = params.scope,
        containerId = params.containerId,
        compareSamples = params.compareSamples,
        threadMode = params.threadMode,
        linearMode = params.linearMode,
        originMode = params.originMode;

    function idxByRevision(series, revision) {
      return _.findIndex(series, function(sample) {
        return sample && sample.revision == revision
      })
    }

    function hydrateChangePoint(point) {
      // if idx == 0, shift it right for one point
      var revIdx = idxByRevision(series, point.suspect_revision) || 1

      point._meta = {
        firstRevIdx: revIdx - 1,
        lastRevIdx:  revIdx,
      }
    }

    // Filter out change points which lays outside ot the chart
    var visibleChangePoints = _.chain(changePoints)
      .filter(function(d) {
        return _.findWhere(series, {revision: d.suspect_revision})
      })
      .each(hydrateChangePoint) // Add some useful meta data
      .value()

    var visibleBFs = _.filter(buildFailures, function(d) {
      return _.findWhere(series, {revision: d.first_failing_revision})
    })

    var cfg = PerfChartService.cfg;
    document.getElementById(containerId).innerHTML = '';

    var svg = d3.select('#' + containerId)
      .append('svg')
      .attr({
        class: 'series',
        width: cfg.container.width,
        height: cfg.container.height,
      })

    var colors = d3.scale.category10();
    // FIXME Force color range 'initialization'. Might be a bug in d3 3.5.3
    // For some reason, d3 will not give you, let's say, the third color
    // unless you requested 0, 1 and 2 before.
    for (var i = 0; i < cfg.knownLevelsCount; i++) colors(i);

    var ops = _.pluck(series, cfg.valueAttr);
    var opsValues = _.pluck(series, 'ops_per_sec_values');
    var avgOpsPerSec = d3.mean(ops)

    // Currently selcted revision item index
    var currentItemIdx = _.findIndex(series, function(d) {
      return d.task_id == scope.task.id
    })

    function extractThreadLevels(series) {
      return _.reduce(series, function(m, d) {
        return _.union(
          m, _.pluck(d.threadResults, 'threadLevel')
        )
      }, [])
    }

    var allLevels = extractThreadLevels(series)

    var levelsMeta = threadMode == MAXONLY
      ? [{name: 'MAX', idx: cfg.valueAttr, color: '#484848', isActive: true}]
      : _.map(allLevels, function(d, i) {
        var data = {
          name: d,
          idx: _.indexOf(allLevels, d),
          isActive: true,
        }

        var match = cfg.knownLevels[d];

        data.color = colors(match ? match.colorId : cfg.knownLevelsCount + i)
        return data
      })

    var activeLevels
    var activeLevelNames
    function updateActiveLevels() {
      activeLevels = _.where(levelsMeta, {isActive: true})
      activeLevelNames = _.map(activeLevels, function(d) {
        return '' + d.name
      })
    }

    updateActiveLevels()

    // For given `activeLevels` returns those which exists in the `sample`
    function threadLevelsForSample(sample, activeLevels) {
      return _.filter(activeLevels, function(d) {
        return _.includes(_.pluck(sample.threadResults, 'threadLevel'), d.name)
      })
    }

    var bfsForLevel = []
    _.each(visibleBFs, function(bf) {
      _.each(levelsMeta, function(level) {
        if (_.find(bfsForLevel, function(d) {
          d.level == level && d.bf.first_failing_revision == bf.first_failing_revision
        })) {
          return
        }

        bfsForLevel.push({
          level: level,
          bf: bf,
        })
      })
    })

    // Array with combinations combinations of {level, changePoint}
    var changePointForLevel = []
    _.each(visibleChangePoints, function(point) {
      point.bfs = _.where(
        buildFailures, {first_failing_revision: point.suspect_revision}
      ) || []
      var level = _.findWhere(levelsMeta, {name: point.thread_level})
      var levels = level ? [level] : levelsMeta

      _.each(levels, function(level) {
        // Check if there is existing point for this revision/level
        // Mostly for MAXONLY mode
        var existing = _.find(changePointForLevel, function(d) {
          return d.level == level && d.changePoint.suspect_revision == point.suspect_revision
        })
        // If the point already exists, increase count meta property
        if (existing) {
          existing.count++
        } else {
          changePointForLevel.push({
            level: level,
            changePoint: point,
            count: 1,
          })
        }
      })
    })

    // Calculate legend x pos based on levels
    cfg.legend.xPos = (cfg.container.width - levelsMeta.length * cfg.legend.step) / 2

    // Obtains value extractor fn for given `level` and series `item`
    // The obtained function is curried, so you should call it as fn(level)(item)
    var getValueFor = threadMode == MAXONLY
      ? PerfChartService.getValueForMaxOnly
      : PerfChartService.getValueForAllLevels

    // When there are more than one value in opsValues item
    var hasValues = _.all(opsValues, function(d) {
      return d && d.length > 1
    })

    var compareMax = 0
    if (compareSamples && compareSamples.length) {
      compareMax = _.chain(compareSamples)
        .map(function(d) { return d.maxThroughputForTest(key) })
        .max()
        .value()
    }

    // Calculate X Ticks values
    var idxStep = (series.length / cfg.xAxis.maxTicks + 2) | 0
    var xTicksData = _.filter(series, function(d, i) {
      return i % idxStep == 0
    })

    var xScale = d3.scale.linear()
      .domain([0, series.length - 1])
      .range([0, cfg.effectiveWidth])

    var yScale = linearMode ? d3.scale.linear() : d3.scale.log()

    function getOpsValues(sample) {
      // For each active thread level extract values
      // including undefined values for missing data
      return _.map(activeLevelNames, function(d) {
        var result = _.findWhere(sample.threadResults, {threadLevel: d})
        return result && result[cfg.valueAttr]
      })
    }

    function calculateYScaleDomain() {
      var flatOpsValues = _.flatten(
        _.map(series, function(d) {
          if (threadMode == MAXONLY) {
            // In maxonly mode levels contain single (max) item
            // Extract just one ops item
            return d[cfg.valueAttr]
          } else {
            return getOpsValues(d)
          }
        })
      )

      var multiSeriesAvg = d3.mean(flatOpsValues)
      var multiSeriesMin = d3.min(flatOpsValues)
      var multiSeriesMax = d3.max(flatOpsValues)

      // Zoomed mode / linear scale is default.
      // If the upper and lower y-axis values are very close to the average (within 10%)
      // add extra padding to the upper and lower bounds of the graph for display.
      var yAxisUpperBound = d3.max([multiSeriesMax, multiSeriesAvg * 1.1]);
      var yAxisLowerBound = originMode ? 0 : d3.min([multiSeriesMin, multiSeriesAvg * .9]);

      // Create a log based scale, remove any 0 values (log(0) is infinity).
      if (!linearMode) {
        if (yAxisUpperBound == 0) {
          yAxisUpperBound = 1e-1;
        }
        if (yAxisLowerBound == 0 ) {
          yAxisLowerBound = multiSeriesMin;
          if (yAxisLowerBound == 0) {
            yAxisLowerBound = 1e-1;
          }
        }
      }

      // We assume values are either all negative or all positive (around 0).
      // If the average is less than 0 then swap values and negate the
      // upper bound.
      if (multiSeriesAvg < 0) {
        if (!linearMode) {
          yAxisUpperBound = -yAxisUpperBound;
        }
        yAxisLowerBound = d3.min([multiSeriesMin, multiSeriesAvg * 1.1]);
      }

      return [yAxisLowerBound, yAxisUpperBound]
    }

    yScale = yScale.clamp(true)
      .range([cfg.effectiveHeight, 0])
      .nice(5)

    function updateYScaleDomain() {
      yScale.domain(calculateYScaleDomain())
    }

    updateYScaleDomain()

    var yAxis = d3.svg.axis()
        .scale(yScale)
        .orient('left')
        .ticks(cfg.yAxis.ticks, function(value) {
          var absolute = Math.abs(value)
          if (absolute == 0) {
            return "0"
          }
          if (absolute < 1) {
            if ( absolute >= .1) {
              return cfg.formatters.digits_1(value);
            } else {
              return cfg.formatters.digits_2(value);
            }
          } else{
            return cfg.formatters.si(value);
          }
        })

    // ## CHART STRUCTURE ##

    // multi line
    var mline = d3.svg.line()
      .defined(_.identity)
      .x(function(d, i) {
        return xScale(i);
      })
      .y(function(d) {
        return yScale(d);
      });

    if (hasValues) {
      var maxline = d3.svg.line()
        .defined(_.identity)
        .x(function(d, i) { return xScale(i) })
        .y(function(d) { return yScale(d3.max(d.ops_per_sec_values)) })

      var minline = d3.svg.line()
        .defined(_.identity)
        .x(function(d, i) { return xScale(i) })
        .y(function(d) { return yScale(d3.min(d.ops_per_sec_values)) })
    }

    // Y Axis
    svg.append('g')
      .attr({
        class: 'y-axis',
        transform: d3Translate(cfg.margin.left - cfg.yAxis.gap, cfg.margin.top)
      })

    function updateYAxis() {
      svg.select('g.y-axis')
        .transition()
        .call(yAxis)
    }

    updateYAxis()

    var getIdx = function(d) { return _.findIndex(series, d) }

    var xTicks = svg.append('svg:g')
      .attr({
        transform: d3Translate(cfg.margin.left, cfg.margin.top)
      })
      .selectAll('g')
      .data(xTicksData)
      .enter()
      .append('svg:g')
      .attr({
        transform: function(d) { return d3Translate(xScale(getIdx(d)), 0) }
      })

    // X Tick date text
    xTicks
      .append('svg:text')
      .attr({
        y: cfg.effectiveHeight + cfg.xAxis.labelYOffset,
        class: 'x-tick-label',
        'text-anchor': 'middle'
      })
      .text(function(d) { return moment(d.createTime).format(cfg.xAxis.format) })

    // X Tick vertical line
    xTicks
      .append('svg:line')
      .attr({
        class: 'x-tick-line',
        x0: 0,
        x1: 0,
        y1: 0,
        y2: cfg.effectiveHeight
      })

    // Show legend for 'all levels' mode only
    if (threadMode != MAXONLY) {
      var legendG = svg.append('svg:g')
        .attr({
          class: 'legend',
          transform: d3Translate(cfg.legend.xPos, cfg.legend.yPos)
        })

      var legendIter = legendG.selectAll('g')
        .data(levelsMeta)
        .enter()
        .append('svg:g')
        .attr({
          transform: function(d, i) {
            return d3Translate(i * cfg.legend.step, 0)
          }
        })
        .style('cursor', 'pointer')
        .on('click', function(d) {
          d.isActive = !d.isActive
          updateActiveLevels()
          updateYScaleDomain()
          updateYAxis()
          redrawLines()
          redrawLegend()
          redrawRefLines()
          redrawTooltip()
          redrawChangePoints()
          redrawBFs()
        })

      redrawLegend()

      function redrawLegend() {
        legendIter.append('svg:rect')
          .attr({
            y: cfg.legend.textOverRectOffset,
            width: cfg.legend.itemWidth,
            height: cfg.legend.itemHeight,
            fill: function(d) {
              return d.isActive ? d.color : '#666'
            }
          })

        legendIter.append('svg:text')
          .text(function(d) { return d.name })
          .attr({
            x: cfg.legend.itemWidth / 2,
            fill: function(d) { return d.isActive ? 'white' : '#DDD' },
            'text-anchor': 'middle',
          })
      }
    }

    // Chart draw area group
    var chartG = svg.append('svg:g')
      .attr('transform', d3Translate(cfg.margin.left, cfg.margin.top))

    var linesG = chartG.append('g').attr({class: 'lines-g'})

    function redrawLines() {
      var lines = linesG.selectAll('path')
        .data(activeLevels)

      lines
        .transition()
        .attr({
          d: function(level) {
            return mline(_.map(series, getValueFor(level)))
          }
        })
        .style({
          stroke: function(d) { return d.color },
        })

      lines
        .enter()
        .append('path')
        .attr({
          d: function(level) {
            return mline(_.map(series, getValueFor(level)))
          },
        })
        .style({
          stroke: function(d) { return d.color },
        })

      lines.exit().remove()

      // Current revision marker
      var commitCircle = chartG
        .selectAll('circle.current')
        .data(threadLevelsForSample(series[currentItemIdx], activeLevels))

      commitCircle
        .enter()
        .append('circle')
          .attr({
            class: 'point current',
            cx: xScale(currentItemIdx),
            cy: function(level) {
              return yScale(getValueFor(level)(series[currentItemIdx]))
            },
            r: cfg.points.focusedR + 0.5,
            stroke: function(d) { return d.color },
          })

      commitCircle
        .transition()
        .attr({
          cy: function(level) {
            return yScale(getValueFor(level)(series[currentItemIdx]))
          }
        })

      commitCircle.exit().remove()
    }

    redrawLines()

    if (hasValues) {
      chartG.append('path')
        .data([series])
        .attr({
          class: 'error-line',
          d: maxline
        })

      chartG.append('path')
        .data([series])
        .attr({
          class: 'error-line',
          d: minline
        })
    }

    // TODO This lines should be rewritten from scratch
    function redrawRefLines() {
      if (compareSamples) {
        // Remove group if exists
        chartG.select('g.cmp-lines').remove()
        // Recreate group
        var cmpLinesG = chartG.append('g')
          .attr({class: 'cmp-lines'})

        for(var j=0; j < compareSamples.length; j++) {
          var g = cmpLinesG.append('g')
            .attr('class', 'cmp-' + j)
          var compareSample = compareSamples[j]
          var compareMax = compareSample.maxThroughputForTest(key)

          var values;
          if (threadMode == MAXONLY) {
            var values = [yScale(compareSample.maxThroughputForTest(key))]
          } else {
            var testResult = compareSample.resultForTest(key)
            if (testResult) {
              var values = _.map(activeLevelNames, function(lvl) {
                return yScale(testResult.results[lvl][cfg.valueAttr])
              })
            }
          }

          values = _.filter(values)

          if (values && values.length) {
            g.selectAll('.mean-line')
              .data(values)
              .enter()
              .append('svg:line')
              .attr({
                class: 'mean-line',
                x1: 0,
                x2: cfg.effectiveWidth,
                y1: _.identity,
                y2: _.identity,
                stroke: function() { return d3.rgb(colors(j + 1)).brighter() },
                'stroke-width': '2',
                'stroke-dasharray': '5,5'
              })
          }
        }
      }
    }

    redrawRefLines()

    function redrawBFs() {
      var bfs = bfsG.selectAll('g.bf')
        .data(_.filter(bfsForLevel, function(d) {
          return d.level.isActive
        }))

      bfs
        .enter()
        .append('g')
        .attr({
          class: 'bf',
        })
        .append('path')
        .attr({
          d: 'M-5,0L0,8L5,0L0,-8Z', // Rhomb figure, marker for a BF
        })
        .style({
          stroke: 'green',
          fill: '#00FF00',
          'stroke-width': 2,
        })

      bfs.attr({
        transform: function(d) {
          var idx = idxByRevision(series, d.bf.first_failing_revision)
          return d3Translate(xScale(idx), yScale(getValueFor(d.level)(series[idx])))
        },
      })

      bfs.exit().remove()
    }

    function redrawChangePoints() {
      // Render change points
      var pointsAndSegments = _.chain(changePointForLevel)
        .filter(function(d) { return d.level.isActive })
        .partition(function(d) { // Separate points and segments
          return d.changePoint._meta.firstRevIdx == d.changePoint._meta.lastRevIdx
        })
        .value()

      var changePointSegments = changePointsG.selectAll('g.change-point-segment')
        .data(pointsAndSegments[1])

      changePointSegments
        .enter()
        .append('g')
          .attr({
            class: 'point change-point-segment',
          })
        .append('line')
        .attr({
          x1: function(d) { return xScale(d.changePoint._meta.firstRevIdx) },
          x2: function(d) { return xScale(d.changePoint._meta.lastRevIdx) },
          y1: function(d) {
            return yScale(getValueFor(d.level)(series[d.changePoint._meta.firstRevIdx]))
          },
          y2: function(d) {
            return yScale(getValueFor(d.level)(series[d.changePoint._meta.lastRevIdx]))
          },
        })
        .style({
          stroke: 'red',
          'stroke-width': '4',
        })

      changePointSegments.exit().remove()

      var changePointPoints = changePointsG.selectAll('g.change-point')
        .data(pointsAndSegments[0])

      changePointPoints
        .enter()
        .append('g')
          .attr({
            class: 'point change-point',
          })
        .append('path') // Plus sign image for change points
          .attr({
            // plus-sign stroke
            class: 'change-point',
            d: 'M-7,3.5h4v4h6v-4h4v-6h-4v-4h-6v4h-4z',
          }).style({
            fill: function(d) {
              return d.count == 1 ? 'yellow' :
                     d.count == 2 ? 'orange' :
                     'red'
            },
          })

      changePointPoints
        .attr({
          transform: function(d) {
            var idx = _.findIndex(series, function(sample) {
              return sample && sample.revision == d.changePoint.suspect_revision
            })

            return idx > -1 ? d3Translate(
              xScale(idx),
              yScale(getValueFor(d.level)(series[idx]))
            ) : undefined
          },
        })

      changePointPoints.exit().remove()
    }

    var bfsG = chartG.append('g')
      .attr('class', 'g-bfs')

    redrawBFs()

    var changePointsG = chartG.append('g')
      .attr('class', 'g-change-points')

    redrawChangePoints()

    var changePointPanelG = chartG.append('g')
      .attr({class: 'change-point-info'})

    // -- REGULAR POINT HOVER --
    // Contains elements for hover behavior
    var focusG = chartG.append('g')
      .style('display', 'none')

    var focusedLine = focusG.append('line')
      .attr({
        class: 'focus-line',
        x1: 0,
        x2: 0,
        y1: cfg.effectiveHeight,
      })

    var enableFocusGroup
    var focusedPointsRef
    var focusedTextRef

    function redrawTooltip() {
      focusG.style('display', 'none')

      // This function could be called just once
      enableFocusGroup = _.once(
        function() {
          focusG.style('display', null)
        }
      )
    }

    redrawTooltip()

    function updateToolitp(levels) {
      var focusedPoints = focusG.selectAll('circle')
        .data(levels)

      focusedPointsRef = focusedPoints
        .attr({
          fill: function(d) { return d3.rgb(d.color).darker() },
        })

      focusedPoints
        .enter()
        .append('svg:circle')
        .attr({
          class: 'focus-point',
          r: cfg.points.focusedR,
          fill: function(d) { return d3.rgb(d.color).darker() },
        })

      focusedPoints.exit().remove()

      var focusedText = focusG.selectAll('text')
        .data(levels)

      focusedTextRef = focusedText
        .attr({
          fill: function(d) { return d3.rgb(d.color).darker(2) },
        })

      focusedText
        .enter()
        .append('svg:text')
        .attr({
          class: 'focus-text',
          x: cfg.focus.labelOffset.x,
          fill: function(d) { return d3.rgb(d.color).darker(2) },
        })

      focusedText.exit().remove()
    }

    // -- CHANGE POINT HOVER --
    var toolTipG = chartG.append('g')
      .attr('class', 'g-tool-tip')
      .style('opacity', 0)

    toolTipG.append('rect')
      .attr({
        x: cfg.toolTip.xOffset - cfg.toolTip.margin,
        y: cfg.toolTip.yOffset - cfg.toolTip.margin,
        width: cfg.toolTip.width + cfg.toolTip.margin * 2,
        height: cfg.toolTip.height + cfg.toolTip.margin * 2,
        rx: 5,
        ry: 5,
      })
      .style({
        fill: '#FFE',
        stroke: '#EED',
        'stroke-width': 2,
      })

    toolTipG.append('line')
      .attr({
        x1: 0,
        y1: 0,
        x2: 0,
        y2: cfg.toolTip.height,
      })
      .style({
        stroke: 'black',
        'stroke-width': 1,
      })

    function translateToolTipItem(d) {
      return d3Translate(0, cfg.toolTip.interlineSpace * (d.idx + .67))
    }

    var toolTipItem = toolTipG
      .selectAll('g.stat')
      // Display all statistical items
      // Non-statistical items are handled separately
      .data(
        _.filter(cfg.toolTip.labels, function(d) {
            return d.nonStat !== true
          }
        )
      )
      .enter()
      .append('g')
      .attr({
        class: 'stat',
        transform: translateToolTipItem,
      })

    var nonStatToolTipItem = toolTipG
      .selectAll('g.non-stat')
      .data(_.where(cfg.toolTip.labels, {nonStat: true}))
      .enter()
      .append('g')
      .attr({
        class: 'non-stat',
        transform: translateToolTipItem,
      })

    function placeTitle(d) {
      return d
        .append('text')
        .attr({
          x: -5,
          y: 0,
          'text-anchor': 'end',
        })
        .text(_.property('display'))
        .style({
          'font-weight': 'bold',
        })
    }

    toolTipItem.call(placeTitle)
    nonStatToolTipItem.call(placeTitle)

    toolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[1],
        y: 0,
        'text-anchor': 'end',
        class: 'prev',
      })

    toolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[2],
        y: 0,
      })
      .text('➞')

    toolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[3],
        y: 0,
        class: 'next',
      })

    nonStatToolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[0] + 5,
        y: 0,
        class: 'value',
      })

    // For given 1d directional segment with length `len` and offset `pos`
    // calculate new position, which fits `domain`
    // :param domain: domain we want to fit in
    // :ptype domain: [min, max]
    // :param pos: position of directional segment (actually 1d vector)
    // :param len: length of directional segment, non-negative
    // :returns: new position of directional segment
    // Example:
    // A. domain=[0, 100], pos=50, len=10
    //    The segment perfectly fits domain, pos = 50
    // B. domain=[0, 100], pos=-20, len=10
    //    The segment position is outside of domain, pos = 0
    // C. domain=[0, 100], pos=99, len=10
    //    The segment end position is outside of domain, pos = 90
    function fit1d(domain, pos, len) {
      return pos < domain[0]       ? 0 :
             pos + len > domain[1] ? domain[1] - len :
             pos
    }

    function fit(domain2d, pos2d, len2d) {
      return [
        fit1d(domain2d[0], pos2d[0], len2d[0]),
        fit1d(domain2d[1], pos2d[1], len2d[1]),
      ]
    }

    function changePointMouseMove() {
      // Fit tool tip panel into chart container
      toolTipG
        .attr({
          'transform': function() {
            return d3Translate.apply(
              null,
              fit(
                [[0, cfg.effectiveWidth], [0, cfg.effectiveHeight]],
                d3.mouse(chartG[0][0]), // [0][0] is required to get bare <g> element
                [cfg.toolTip.width, cfg.toolTip.height]
              )
            )
          }
        })
    }

    function changePointMouseOver(point) {
      function extractToolTipValue(item, obj) {
        // Get raw value of the stat item
        var rawValue = _.isFunction(item.key)
          ? item.key(obj)
          : obj[item.key]
        // Apply formatting if defined
        return item.formatter ? item.formatter(rawValue) : rawValue
      }

      function statTextFor(statItemName) {
        return function(d) {
          var cpStat = point.statistics[statItemName]
          return extractToolTipValue(d, cpStat)
        }
      }

      toolTipItem.select('text.prev')
        .text(_.compose(formatNumber, statTextFor('previous')))

      toolTipItem.select('text.next')
        .text(_.compose(formatNumber, statTextFor('next')))

      nonStatToolTipItem.select('text.value')
        .text(function(d) { return extractToolTipValue(d, point) })

      toolTipG
        .transition()
        .style('opacity', .9)
    }

    function changePointMouseOut() {
      toolTipG
        .transition()
        .style('opacity', 0)
    }

    function formatNumber(value) {
      var absolute = Math.abs(value);
      if (absolute == 0) {
        return "0";
      } else if (absolute < 1) {
        if (absolute >= .1) {
          return cfg.formatters.digits_1(value);
        } else if (absolute >= .01) {
          return cfg.formatters.digits_2(value);
        } else {
          return cfg.formatters.digits_3(value);
        }
      } else{
        return cfg.formatters.large(value);
      }
    }

    // Overlay to handle hover action
    chartG.append('svg:rect')
      .attr({
        class: 'overlay',
        width: cfg.effectiveWidth,
        height: cfg.effectiveHeight
      })
      .on('mouseover', function() {
        scope.currentHoverSeries = series;
      })
      .on('click', function() {
        scope.locked = !scope.locked
        scope.$digest()
      })
      .on('mousemove', overlayMouseMove)

    var cpHover = false

    function overlayMouseMove() {
      if (scope.locked) return;
      var idx = Math.round(xScale.invert(d3.mouse(this)[0]))
      var d = series[idx]
      var hash = d.revision

      var hoveredChangePoint = _.filter(visibleChangePoints, function(d) {
        return d._meta.firstRevIdx <= idx && idx <= d._meta.lastRevIdx
      })[0]

      if (hoveredChangePoint) {
        if (!cpHover) {
          changePointMouseOver(hoveredChangePoint)
        }
        changePointMouseMove()
        cpHover = true
      } else if (cpHover) {
        changePointMouseOut()
        cpHover = false
      }

      // Reduce number of calls if hash didn't changed
      if (hash != scope.$parent.currentHash) {
        scope.$parent.currentHash = hash;
        scope.$parent.currentHashDate = d.createTime
        scope.$parent.bfs = _.pluck(
          _.where(visibleBFs, {first_failing_revision: hash}),
          'key'
        )
        scope.$emit('hashChanged', hash)
        scope.$parent.$digest()
      }
    }

    function focusPoint(hash) {
      var idx = _.findIndex(series, function(d) {
        return d && d.revision == hash
      })
      if (idx == undefined) return;
      var item = series[idx]
      if (!item) return;

      var x = xScale(idx)

      // List of per thread level values for selected item
      var values = threadMode == MAXONLY
        ? [item[cfg.valueAttr]]
        : _.filter(getOpsValues(item))

      var maxOps = _.max(values);

      // List of dot Y positions
      var yScaledValues = _.map(values, yScale)
      var opsLabelsY = PerfChartService.getOpsLabelYPosition(yScaledValues, cfg);

      focusG.attr('transform', d3Translate(x, 0))

      // Add/Remove tooltip items (some data samples may not contain all thread levels)
      updateToolitp(threadMode == MAXONLY
        ? activeLevels
        : threadLevelsForSample(item, activeLevels)
      )

      focusedPointsRef.attr({
        cy: function(d, i) { return yScaledValues[i] },
      })

      focusedTextRef
        .attr({
          y: function(d, i) { return opsLabelsY[i] },
          transform: function(d, i) {
            // transform the hover text location based on the list index
            var x = 0

            // Catch NS_LOOKUP_ERROR. Checking `this` and `this.getBBox` don't work
            // FIXME Should be a better way. Probably, complete refactoring required
            try {
              this.getBBox()
            } catch (e) {
              return
            }

            // this.getBBox might not exist when the sample doesn't exist
            if (series && this.getBBox) {
              x = (cfg.focus.labelOffset.x + this.getBBox().width) * idx / series.length
            }
            return d3Translate(-x, 0)
          }
        })
        .text(function(d, i) {
          return formatNumber(values[i])
        });

      focusedLine.attr({
        y2: yScale(maxOps),
      })
    }

    scope.$on('hashChanged', function(e, hash) {
      // Make tool tip visible
      enableFocusGroup();
      // Apply new position to tool tip
      focusPoint(hash)
    })
  }
})
