// Helper function
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
      itemWidth: 30,
      gap: 10, // between legen items
      yOffset: 10, // To bootom
      textOverRectOffset: -15, // Aligns legend item with the text label
      yPos: undefined, // To be calculated
      xPos: undefined, // To be calculated
      step: undefined, // to e calculated. Step between legend items
    }
  };

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

  return {
    cfg: cfg,
    getOpsLabelYPosition: getOpsLabelYPosition,
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
      threadMode = params.threadMode;

  var cfg = PerfChartService.cfg;
  document.getElementById(containerId).innerHTML = '';

  var svg = d3.select('#' + containerId)
    .append('svg')
    .attr({
      class: 'series',
      width: cfg.container.width,
      height: cfg.container.height,
    })

  var ops = _.pluck(series, cfg.valueAttr);
  var opsValues = _.pluck(series, 'ops_per_sec_values');
  var avgOpsPerSec = d3.mean(ops)

  var allLevels = _.keys(series[0].threadResults)
  var levels = threadMode == MAXONLY
    ? [_.max(allLevels, function(d) { return +d })]
    : allLevels

  // Calculate legend x pos based on levels
  cfg.legend.xPos = (cfg.container.width - levels.length * cfg.legend.step) / 2

  // When there are more than one value in opsValues item
  var hasValues = _.all(opsValues, function(d) {
    return d && d.length > 1
  })

  var flatOpsValues = _.flatten(
    _.map(series, function(d) {
      if (threadMode == MAXONLY) {
        // In maxonly mode levels contain single (max) item
        // Extract just one ops item
        return d.threadResults[levels[0]][cfg.valueAttr]
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
  if (compareSamples) {
    compareMax = _.chain(compareSamples)
      .map(function(d) { return d.maxThroughputForTest(key) })
      .max()
      .value()
  }

  // If the upper and lower y-axis values are very close to the average
  // (within 10%) add extra padding to the upper and lower bounds of the graph for display
  var yAxisUpperBound = d3.max([compareMax, multiSeriesMax, multiSeriesAvg * 1.1])
  var yAxisLowerBound = d3.min([multiSeriesMin, multiSeriesAvg * .9])

  // Calculate X Ticks values
  var idxStep = (series.length / cfg.xAxis.maxTicks + 2) | 0
  var xTicksData = _.filter(series, function(d, i) {
    return i % idxStep == 0
  })

  // create extra padding if seriesMax
  var yScale = d3.scale.linear()
    .domain([yAxisLowerBound, yAxisUpperBound])
    .range([cfg.effectiveHeight, 0]);

  var xScale = d3.scale.linear()
    .domain([0, ops.length - 1])
    .range([0, cfg.effectiveWidth]);

  var colors = d3.scale.category10();

  var yAxis = d3.svg.axis()
    .scale(yScale)
    .orient('left')
    .ticks(cfg.yAxis.ticks);

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

  var legendG = svg.append('svg:g')
    .attr({
      class: 'legend',
      transform: d3Translate(cfg.legend.xPos, cfg.legend.yPos)
    })

  var legendIter = legendG.selectAll('g')
    .data(levels)
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
      fill: function(d, i) { return colors(i) }
    })

  legendIter.append('svg:text')
    .text(_.identity)
    .attr({
      x: 15,
      fill: 'white',
      'text-anchor': 'middle',
    })

  // Chart draw area group
  var chartG = svg.append('svg:g')
    .attr('transform', d3Translate(cfg.margin.left, cfg.margin.top))

  var lines = chartG.selectAll('path')
    .data(levels)
    .enter()
    .append('path')
    .attr({
      d: function(level) {
        return mline(_.map(series, function(d) {
          return d.threadResults[level][cfg.valueAttr]
        }))
      },
    })
    .style({
      stroke: function(d, i) { return colors(i) },
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
      var compareSample = compareSamples[j]
      var compareMax = compareSample.maxThroughputForTest(key)
      if (compareMax && !_.isNaN(compareMax)) {

        var meanValue = yScale(compareMax)
        chartG.append('svg:line')
          .attr({
            class: 'mean-line',
            x1: 0,
            x2: cfg.effectiveWidth,
            y1: meanValue,
            y2: meanValue,
            stroke: function(d, i) { return colors(j + 1) },
            'stroke-width': '1',
            'stroke-dasharray': '5,5'
          })
      }
    }
  }

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
    .data(levels)
    .enter()
    .append('svg:circle')
    .attr({
      class: 'focus-point',
      r: cfg.points.focusedR,
      fill: function(d, i) { return d3.rgb(colors(i)).darker() },
    })

  var focusedText = focusG.selectAll('text')
    .data(levels)
    .enter()
    .append('svg:text')
    .attr({
      class: 'focus-text',
      x: cfg.focus.labelOffset.x,
      fill: function(d, i) { return d3.rgb(colors(i)).darker(2) },
      //'text-anchor': 'middle'
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
      ? [item.threadResults[levels[0]][cfg.valueAttr]]
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
      .text(function (d, i) { return d3.format(',.0f')(values[i])})

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
