mciModule.directive('microTrendChart', function(PERF_DISCOVERY) { return {
  restrict: 'E',
  scope: {
    data: '=',
  },
  link: function ($scope, el) {
    var container = el[0].parentElement

    var cfg = {
      width: container.clientWidth || PERF_DISCOVERY.TREND_COL_WIDTH,
      height: container.clientHeight,
      margin: 2,
    }

    var svg = d3.select(el[0]).append('svg')
      .attr({
        width: cfg.width,
        height: cfg.height,
      })

    var effectiveWidth = cfg.width - cfg.margin * 2
    var effectiveHeight = cfg.height - cfg.margin * 2

    var xScale = d3.scale.linear()
    var yScale = d3.scale.linear()
      .range([effectiveHeight, cfg.margin])

    // Zero-level line
    var line = svg.append('line')
      .attr({
        x1: cfg.margin,
        x2: effectiveWidth,
      })

    // This $watch is required due to known bug with ui-grid
    // https://github.com/angular-ui/ui-grid/issues/4869
    $scope.$watch('data', function(data) {
      // Aplying log fn to ratios, because ratios are asymmetric
      // And log(y/x) = -log(x/y), so it symmetric around 0
      var samples = _.map(data, Math.log);
      var barWidth = effectiveWidth / samples.length
      var len = samples.length

      xScale
        .domain([0, samples.length])
        .range([cfg.margin, effectiveWidth])

      // Concat 0, because zero reference should always be in domain
      yScale.domain([
        d3.min(samples.concat(0)),
        d3.max(samples.concat(0))
      ])

      line.attr({
        y1: yScale(0),
        y2: yScale(0),
      })

      // Create new bars
      svg.selectAll('rect')
        .data(samples)
        .enter()
        .append('rect')

      // Update existing bars
      svg.selectAll('rect')
        .data(samples)
        .attr({
          x: function(d, i) { return xScale(i) },
          y: function(d) { return d < 0 ? yScale(0) : yScale(d) },
          width: barWidth,
          height: function(d) {
            return Math.sign(d) * (yScale(0) - yScale(d))
          },
          fill: function(d, i) {
            // The last item is blue
            return i == len - 1 ? 'blue' :
                   d >= 0 ? 'green' : 'red'
          },
        })

      // Remove unused bars
      svg.selectAll('rect')
        .data(samples)
        .exit()
        .remove()
    })
  }
}})
