// Helper function
// TODO Make it accept (d, i) and return function (FP)
function d3Translate(x, y) {
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
      left: 80
    },
    points: {
      focusedR: 4.5,
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
      digits_2: d3.format('.03f'), // floating point 3 digits
      digits_3: d3.format('.03f'), // floating point 3 digits
      si: d3.format(',s'), // si notation
    }
  };

  // Non-persistent color id offset for unusual thread level
  cfg.knownLevelsCount = _.keys(cfg.knownLevels).length

  // Chart are real size
  cfg.effectiveWidth = cfg.container.width - cfg.margin.left - cfg.margin.right;
  cfg.effectiveHeight = cfg.container.height - cfg.margin.top - cfg.margin.bottom;

  // Legend y pos
  cfg.legend.yPos = (
    cfg.container.height - cfg.legend.yOffset
    - cfg.legend.itemHeight - cfg.legend.textOverRectOffset
  )

  cfg.legend.step = cfg.legend.itemWidth + cfg.legend.gap

  // Returns list of y-positions of ops labels for given
  // yScaledValues list.
  function getOpsLabelYPosition(yScaledValues, cfg) {
    // The function assumes that ops/second for threads is always
    // 16 > 8 > 4 > 1. This assumption should be correct in regular cases
    // Calculate the most top (the last) label position.
    // Also checks top margin overlap
    var prevPos = _.last(yScaledValues) + cfg.focus.labelOffset.y;
    if (prevPos < cfg.margin.top) {
      prevPos = cfg.margin.top + 5
    }
    var textPosList = [prevPos];
    var pos;

    // Calculate all other items positions, based on previous item position
    // Loop skips the last item (see code above)
    for (var i = yScaledValues.length - 2; i >= 0; i--) {
      var currentPos = yScaledValues[i] + cfg.focus.labelOffset.y;
      var delta = prevPos - currentPos;
      // If labels overlapping, move the label below previous label
      var newPos = (delta > -cfg.focus.labelOffset.between)
        ? prevPos + cfg.focus.labelOffset.between
        : currentPos;
      prevPos = newPos;
      textPosList.push(newPos);
    }

    // Resotre original order
    textPosList.reverse()
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
      return item.threadResults[level.idx][cfg.valueAttr]
    }
  }

  return {
    cfg: cfg,
    getOpsLabelYPosition: getOpsLabelYPosition,
    getValueForMaxOnly: getValueForMaxOnly,
    getValueForAllLevels: getValueForAllLevels,
  }
})

// TODO Add basic encapsulation to the module
// TODO Create AngularJS directive
var drawSingleTrendChart = function(params) {
  var MAXONLY = 'maxonly';

  // Extract params
  var PerfChartService = params.PerfChartService,
      series = params.series,
      key = params.key,
      scope = params.scope,
      containerId = params.containerId,
      compareSamples = params.compareSamples,
      threadMode = params.threadMode,
      linearMode = params.linearMode,
      originMode = params.originMode;

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

  var allLevels = _.pluck(series[0].threadResults, 'threadLevel')

  var levelsMeta = threadMode == MAXONLY
    ? [{name: 'MAX', idx: cfg.valueAttr, color: '#484848'}]
    : _.map(allLevels, function(d, i) {
      var data = {
        name: d,
        idx: _.indexOf(allLevels, d),
      };

      var match = cfg.knownLevels[d];

      data.color = colors(match ? match.colorId : cfg.knownLevelsCount + i)
      return data
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

  var flatOpsValues = _.flatten(
    _.map(series, function(d) {
      if (threadMode == MAXONLY) {
        // In maxonly mode levels contain single (max) item
        // Extract just one ops item
        return d[cfg.valueAttr]
      } else {
        return _.map(d.threadResults, function(d) {
          return d[cfg.valueAttr]
        })
      }
    })
  )

  var multiSeriesAvg = d3.mean(flatOpsValues)
  var multiSeriesMin = d3.min(flatOpsValues)
  var multiSeriesMax = d3.max(flatOpsValues)

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
    .domain([0, ops.length - 1])
    .range([0, cfg.effectiveWidth]);


  // Zoomed mode / linear scale is default.
  // If the upper and lower y-axis values are very close to the average (within 10%)
  // add extra padding to the upper and lower bounds of the graph for display.
  var yAxisUpperBound = d3.max([compareMax, multiSeriesMax, multiSeriesAvg * 1.1]);
  var yAxisLowerBound = originMode ? 0 : d3.min([multiSeriesMin, multiSeriesAvg * .9]);
  var yScale = d3.scale.linear();

  // Create a log based scale, remove any 0 values (log(0) is infinity).
  if (!linearMode) {
    yScale = d3.scale.log()
    if (yAxisUpperBound == 0) {
      yAxisUpperBound = 1e-1;
    }
    if (yAxisLowerBound == 0 ) {
      yAxisLowerBound = 1e-1;
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

  yScale = yScale.clamp(true)
    .domain([yAxisLowerBound, yAxisUpperBound])
    .range([cfg.effectiveHeight, 0]).nice(5);
  var yAxis = d3.svg.axis()
      .scale(yScale)
      .orient('left')
      .ticks(cfg.yAxis.ticks, function(d) {
        var val = Math.abs(d)
        if (val == 0) {
          return "0"
        }
        if (val < 1) {
          if ( val >= .1) {
            return cfg.formatters.digits_1(d);
          } else {
            return cfg.formatters.digits_2(d);
          }
        } else{
          return cfg.formatters.si(d);
        }
      })
  // ## CHART STRUCTURE ##

  // multi line
  var mline = d3.svg.line()
    .x(function(d, i) {
      return xScale(i);
    })
    .y(function(d) {
      return yScale(d);
    });

  if (hasValues) {
    var maxline = d3.svg.line()
      .x(function(d, i) { return xScale(i) })
      .y(function(d) { return yScale(d3.max(d.ops_per_sec_values)) })

    var minline = d3.svg.line()
      .x(function(d, i) { return xScale(i) })
      .y(function(d) { return yScale(d3.min(d.ops_per_sec_values)) })
  }

  // Y Axis
  svg.append('svg:g')
    .attr({
      class: 'axis',
      transform: d3Translate(cfg.margin.left - cfg.yAxis.gap, cfg.margin.top)
    })
    .transition()
    .call(yAxis);

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
    .text(function(d) { return moment(d.startedAt).format(cfg.xAxis.format) })

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

    legendIter.append('svg:rect')
      .attr({
        y: cfg.legend.textOverRectOffset,
        width: cfg.legend.itemWidth,
        height: cfg.legend.itemHeight,
        fill: function(d) { return d.color }
      })

    legendIter.append('svg:text')
      .text(function(d) { return d.name })
      .attr({
        x: cfg.legend.itemWidth / 2,
        fill: 'white',
        'text-anchor': 'middle',
      })
    }

  // Chart draw area group
  var chartG = svg.append('svg:g')
    .attr('transform', d3Translate(cfg.margin.left, cfg.margin.top))

  var lines = chartG.selectAll('path')
    .data(levelsMeta)
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

  if (compareSamples) {
    for(var j=0; j < compareSamples.length; j++) {
      var g = chartG.append('g')
      var compareSample = compareSamples[j]
      var compareMax = compareSample.maxThroughputForTest(key)

      var values;
      if (threadMode == MAXONLY) {
        var values = [yScale(compareSample.maxThroughputForTest(key))]
      } else {
        var testResult = compareSample.resultForTest(key)
        if (testResult) {
          var values = _.map(testResult.results, function(d) {
            return yScale(d[cfg.valueAttr])
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

  // Current revision marker
  chartG.append('g')
    .attr('class', 'g-current-commit')
    .selectAll('circle')
    .data(levelsMeta)
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

  // Contains elements for hover behavior
  var focusG = chartG.append('svg:g')
    .style('display', 'none')

  var focusedLine = focusG.append('svg:line')
    .attr({
      class: 'focus-line',
      x1: 0,
      x2: 0,
      y1: cfg.effectiveHeight,
    })

  var focusedPoints = focusG.selectAll('circle')
    .data(levelsMeta)
    .enter()
    .append('svg:circle')
    .attr({
      class: 'focus-point',
      r: cfg.points.focusedR,
      fill: function(d) { return d3.rgb(d.color).darker() },
    })

  var focusedText = focusG.selectAll('text')
    .data(levelsMeta)
    .enter()
    .append('svg:text')
    .attr({
      class: 'focus-text',
      x: cfg.focus.labelOffset.x,
      fill: function(d) { return d3.rgb(d.color).darker(2) },
    })

  // This function could be called just once
  var enableFocusGroup = _.once(
    function() {
      focusG.style('display', null)
    }
  )

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

  function overlayMouseMove() {
    if (scope.locked) return;
    var idx = Math.round(xScale.invert(d3.mouse(this)[0]))
    var d = series[idx]
    var hash = d.revision

    // Reduce number of calls if hash didn't changed
    if (hash != scope.currentHash) {
      scope.currentHash = hash;
      scope.currentHashDate = d.startedAt
      scope.$emit('hashChanged', hash)
      // FIXME This slowdowns the chart dramatically
      scope.$digest()
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
      : _.pluck(item.threadResults, cfg.valueAttr);

    var maxOps = _.max(values);
    // List of dot Y positions
    var yScaledValues = _.map(values, yScale);
    var opsLabelsY = PerfChartService.getOpsLabelYPosition(yScaledValues, cfg);

    focusG.attr('transform', d3Translate(x, 0))

    focusedPoints.attr({
      cy: function(d, i) { return yScaledValues[i] },
    })

    focusedText
      .attr('y', function(d, i) { return opsLabelsY[i] })
      .text(function (d, i) {
        var val = Math.abs(values[i]);
        if ( val == 0) {
          return "0";
        } else if ( Math.abs(val) < 1) {
          if ( val >= .1) {
            return cfg.formatters.digits_1(val);
          }  else if ( val >= .01) {
            return cfg.formatters.digits_2(val);
          } else {
            return cfg.formatters.digits_3(val);
          }
        } else{
          return cfg.formatters.large(val);
        }
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
