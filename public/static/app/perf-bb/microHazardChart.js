mciModule.directive('microHazardChart', function (
  CHANGE_POINTS_GRID, PERFBB_HAZARD_VALUES
) {
  return {
    restrict: 'E',
    scope: {
      points: '=',
      ctx: '=',
    },
    link: function ($scope, el) {
      const categories = PERFBB_HAZARD_VALUES;

      let colors = {
        "Major Regression": '#ff0c00',
        "Moderate Regression": '#ff8800',
        "Minor Regression": '#ffdb00',
        "No Change": '#7d796d',
        "Minor Improvement": '#b1ff4e',
        "Moderate Improvement": '#67ff2a',
        "Major Improvement": '#00ce6a',
      };
      const container = el[0].parentElement;

      const cfg = {
        width: CHANGE_POINTS_GRID.HAZARD_CHART_WIDTH,
        height: container.clientHeight || CHANGE_POINTS_GRID.HAZARD_CHART_HEIGHT,
        margin: 2,
      };

      const svg = d3.select(el[0]).append('svg')
        .attr({
          width: cfg.width,
          height: cfg.height,
          'shape-rendering': 'optimizeSpeed',
        });

      const effectiveWidth = cfg.width - cfg.margin * 2;
      const effectiveChartWidth = cfg.width - cfg.margin * 4;
      const effectiveHeight = cfg.height - cfg.margin * 2;

      const xScale = d3.scale.linear()
        .domain([0, categories.length])
        .range([cfg.margin, effectiveWidth]);

      const yScale = d3.scale.log()
        .base(Math.E)
        .range([cfg.margin, effectiveHeight]);

      const barWidth = effectiveChartWidth / categories.length;

      // Zero-level line
      svg.append('line')
        .attr({
          x1: cfg.margin,
          x2: effectiveWidth,
          y1: effectiveHeight + cfg.margin,
          y2: effectiveHeight + cfg.margin,
        }).style({
        stroke: 'gray',
      });

      // Create new bars
      svg.selectAll('rect')
        .data(categories)
        .enter()
        .append('rect');

      // This $watch is required due to known bug with ui-grid
      // https://github.com/angular-ui/ui-grid/issues/4869
      $scope.$watchCollection('[points, ctx]', function (coll) {
        const points = coll[0];
        for (point of points) {
          if (point.category === undefined) {
            let magnitude = point.percent_change / 100;
            if (-Infinity <= magnitude && magnitude < -0.5) {
              point.category = "Major Regression"
            } else if (-0.5 <= magnitude && magnitude < -0.2) {
              point.category = "Moderate Regression"
            } else if (-0.2 <= magnitude && magnitude < 0) {
              point.category = "Minor Regression"
            } else if (magnitude === 0) {
              point.category = "No Change"
            } else if (0 < magnitude && magnitude < 0.2) {
              point.category = "Minor Improvement"
            } else if (0.2 <= magnitude && magnitude < 0.5) {
              point.category = "Moderate Improvement"
            } else if (0.5 <= magnitude && magnitude < Infinity) {
              point.category = "Major Improvement"
            }
          }
        }


        const ctx = coll[1];
        const data = _.chain(points)
          .sortBy('category')
          .groupBy('category')
          .mapObject((d) => d.length)
          .value();

        yScale.domain([
          1 / Math.E,
          d3.max(_.values(data).concat(ctx))
        ]);

        // Update existing bars
        svg.selectAll('rect')
          .data(categories)
          .attr({
            x: function (d, i) {
              return xScale(i)
            },
            y: function (d) {
              return data[d]
                ? effectiveHeight - yScale(data[d]) + cfg.margin
                : 0
            },
            width: barWidth,
            height: function (d) {
              return data[d] ? yScale(data[d]) - 1 : 0
            },
            fill: function (d, i) {
              return colors[d]
            },
          })
      })
    }
  }
});
