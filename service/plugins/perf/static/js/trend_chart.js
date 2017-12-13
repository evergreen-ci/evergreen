// TODO Add basic encapsulation to the module
// TODO Create AngularJS directive

var drawSingleTrendChart = function(
  // TODO idx should be passed in different manner
  // TODO Use object instead of positional args
  trendSamples, tests, scope, taskId, compareSamples, idx
) {
  var containerId = 'perf-trendchart-' + cleanId(taskId) + '-' + idx;

  document.getElementById(containerId).innerHTML = '';

  var cfg = {
    container: {
      width: 960,
      height: 200
    },
    margin: {
      top: 20,
      right: 50,
      bottom: 30,
      left: 80
    },
    points: {
      defaultR: 2,
      focusedR: 4.5
    },
    chart: {
      yValueAttr: 'ops_per_sec'
    },
    yAxis: {
      ticks: 5,
      gap: 10 // White space between axis and chart
    },
    xAxis: {
      maxTicks: 10,
      format: 'MMM DD'
    },
    focus: {
      labelOffset: {
        x: 0,
        y: -15
      }
    },
    format: {
      date: 'll'
    }
  };

  cfg.effectiveWidth = cfg.container.width - cfg.margin.left - cfg.margin.right;
  cfg.effectiveHeight = cfg.container.height - cfg.margin.top - cfg.margin.bottom;

  // Q: Figure out what does 'key' mean
  var key = tests[idx];

  var svg = d3.select('#' + containerId)
    .append('svg')
    .attr({
      class: 'series',
      width: cfg.container.width,
      height: cfg.container.height,
    });

  // TODO Q: Do we really need `trendSamples` here?
  var series = trendSamples.seriesByName[key];

  var ops = _.pluck(series, 'ops_per_sec');
  var opsValues = _.pluck(series, 'ops_per_sec_values');
  var avgOpsPerSec = d3.mean(ops)

  // TODO Q: What is this?
  var hasValues = !_.contains(opsValues, undefined)

  var seriesMax = _.max(ops)
  var seriesAvg = d3.mean(ops)

  var compareMax = 0
  if (compareSamples) {
    compareMax = _.chain(compareSamples)
      .map(function(d) { return d.maxThroughputForTest(key) })
      .max()
      .value()
  }

  // If the upper and lower y-axis values are very close to the average
  // (within 10%) add extra padding to the upper and lower bounds of the graph for display
  var yAxisUpperBound = d3.max([compareMax, seriesMax, seriesAvg * 1.1])
  var yAxisLowerBound = d3.min([d3.min(ops), seriesAvg * .9])

  // Calculate datetime difference between the first and last item
  var dateMinMax = d3.extent(series, function(d) { return d.startedAt })
  var period = dateMinMax[1] - dateMinMax[0]
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

  // Main chart line
  var line = d3.svg.line()
    .x(function(d, i) {
      return xScale(i);
    })
    .y(function(d) {
      return yScale(d[cfg.chart.yValueAttr]);
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
      transform: 'translate(' +
        (cfg.margin.left - cfg.yAxis.gap) + ',' + cfg.margin.top +
      ')'
    })
    .call(yAxis);

  var getIdx = function(d) { return _.findIndex(series, d) }

  var xTicks = svg.append('svg:g')
    .attr({
      transform: 'translate(' +
        cfg.margin.left + ',' + (cfg.container.height - cfg.margin.bottom / 2) +
      ')'
    })
    .selectAll('g')
    .data(xTicksData)
    .enter()
    .append('svg:g')
    .attr({
      transform: function(d) {
        return 'translate(' + xScale(getIdx(d)) + ',0)'
      }
    })
    .append('svg:text')
    .attr({
      class: 'x-tick',
      'text-anchor': 'middle'
    })
    .style('fill', '#777')
    .text(function(d) { return moment(d.startedAt).format(cfg.xAxis.format) })

  // Chart draw area group
  var chartG = svg.append('svg:g')
    .attr('transform', 'translate(' + cfg.margin.left + ',' + cfg.margin.top + ')')

  chartG.append('path')
    .data([series])
    .attr({
      class: 'line',
      d: line
    });

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

  // Adds little dots over the line
  chartG.selectAll('.point')
    .data(series)
    .enter()
    .append('svg:circle')
    .attr({
      class: function(d) {
        if (d.task_id == scope.task.id) {
          return 'point current';
        // FIXME long line
        } else if(!!scope.comparePerfSample && d.revision == scope.comparePerfSample.sample.revision) {
          return 'point compare';
        }
        return 'point'
      },
      cx: function(d, i) { return xScale(i) },
      cy: function(d) { return yScale(d[cfg.chart.yValueAttr]) },
      r: function(d) {
        return d.task_id == scope.task.id ? 5 : cfg.points.defaultR;
      }
    })

  if (compareSamples) {
    for(var j=0; j < compareSamples.length; j++) {
      var compareSample = compareSamples[j]
      var compareMax = compareSample.maxThroughputForTest(key)
      if (!_.isNaN(compareMax)) {

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
    //TODO Figure out how to sync this across all charts
    //.style('display', 'none')

  var focusedPoint = focusG.append('svg:circle')
    .attr({
      r: cfg.points.focusedR
    })

  var focusedText = focusG.append('svg:text')
    .attr({
      x: cfg.focus.labelOffset.x,
      y: cfg.focus.labelOffset.y,
      'text-anchor': 'middle'
    })
    .style('font-weight', 'bold')

  // Overlay to handle hover action
  chartG.append('svg:rect')
    .attr({
      class: 'overlay',
      width: cfg.effectiveWidth,
      height: cfg.effectiveHeight
    })
    .on('mouseover', function() {
      // TODO figure out how to sync
      //focusG.style('display', null);
      scope.currentHoverSeries = series;
    })
    .on('click', function() {
      scope.locked = !scope.locked
      scope.$digest()
    })
    .on('mouseout', function() {
      // TODO figure out how sync
      //!scope.locked && focusG.style('display', 'none')
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
      scope.$emit('hashChanged', hash)
      // FIXME This slowdowns the chart dramatically
      scope.$digest()
    }
  }

  function focusPoint(hash) {
    // | 0 for fast Math.floor
    // Just for conveniece
    var idx = _.findIndex(series, function(d) {
      return d && d.revision == hash
    })
    if (!idx) return;
    var d = series[idx]
    if (!d) return;
    var x = xScale(idx) | 0
    var y = yScale(d[cfg.chart.yValueAttr]) | 0

    focusG.attr('transform', 'translate(' + x +',' + y + ')')
    focusedText.text(moment(d.startedAt).format(cfg.format.date))
  }

  scope.$on('hashChanged', function(e, hash) {
    focusPoint(hash)
  })
}
