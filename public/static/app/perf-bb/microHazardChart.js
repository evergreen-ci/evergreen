mciModule.directive('microHazardChart', function(
  CHANGE_POINTS_GRID, PERFBB_HAZARD_VALUES
) { return {
  restrict: 'E',
  scope: {
    points: '=',
    ctx: '=',
  },
  link: function ($scope, el) {
    const categories = PERFBB_HAZARD_VALUES
    const colors = [
      'ff0c00',
      'ff8800',
      'ffdb00',
      'b1ff4e',
      '67ff2a',
      '00ce6a',
    ]

    var container = el[0].parentElement

    var cfg = {
      width: CHANGE_POINTS_GRID.HAZARD_CHART_WIDTH,
      height: container.clientHeight || CHANGE_POINTS_GRID.HAZARD_CHART_HEIGHT,
      margin: 2,
    }

    var svg = d3.select(el[0]).append('svg')
      .attr({
        width: cfg.width,
        height: cfg.height,
        'shape-rendering': 'optimizeSpeed',
      })

    var effectiveWidth = cfg.width - cfg.margin * 2
    var effectiveChartWidth = cfg.width - cfg.margin * 4
    var effectiveHeight = cfg.height - cfg.margin * 2

    var xScale = d3.scale.linear()
      .domain([0, categories.length])
      .range([cfg.margin, effectiveWidth])

    var yScale = d3.scale.log()
      .base(Math.E)
      .range([cfg.margin, effectiveHeight])

    var barWidth = effectiveChartWidth / categories.length

    // Zero-level line
    svg.append('line')
      .attr({
        x1: cfg.margin,
        x2: effectiveWidth,
        y1: effectiveHeight + cfg.margin,
        y2: effectiveHeight + cfg.margin,
      }).style({
        stroke: 'gray',
      })

    // Create new bars
    svg.selectAll('rect')
      .data(categories)
      .enter()
      .append('rect')

    // This $watch is required due to known bug with ui-grid
    // https://github.com/angular-ui/ui-grid/issues/4869
    $scope.$watchCollection('[points, ctx]', function(coll) {
      var points = coll[0]
      var ctx = coll[1]
      var data = _.chain(points)
        .sortBy('category')
        .groupBy('category')
        .mapObject((d) => d.length )
        .value()

      yScale.domain([
        1/Math.E,
        d3.max(_.values(data).concat(ctx))
      ])

      // Update existing bars
      svg.selectAll('rect')
        .data(categories)
        .attr({
          x: function(d, i) {
            return xScale(i)
          },
          y: function(d) {
            return  data[d]
              ? effectiveHeight - yScale(data[d]) + cfg.margin
              : 0
          },
          width: barWidth,
          height: function(d) { return data[d] ? yScale(data[d]) - 1 : 0 },
          fill: function(d, i) {
            return '#' + colors[i]
          },
        })
    })
  }
}})
