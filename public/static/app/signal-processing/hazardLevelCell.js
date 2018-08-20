// TODO Jim: should be moved to python layer (PERF-1546)
mciModule.directive('hazardLevelCell', function() {
  var levels = [
    {label: 'Major', color: '#C00',},
    {label: 'Moderate', color: '#F60',},
    {label: 'Minor', color: '#FC0',},
    {label: 'Recovery', color: 'green',},
  ]

  function weightedRatio(point) {
    if (point.statistics.previous.mean == 0) {
      return 0
    }
    return Math.log(
      point.statistics.next.mean / point.statistics.previous.mean
    )
  }

  function renderFoldedItem(scope, row) {
    var children = scope.row.treeNode.children
    var weights = _.chain(children)
      .map(function(d) { return d.row.entity })
      .map(weightedRatio)
      .sortBy()
      .value()

    var multiplier = _.filter(weights, function(d) {
      return d > 0
    }).length || 1

    var value = multiplier * d3.quantile(weights, .9)

    var idx = (
      value < 0   ? 3 :
      value < 0.5 ? 2 :
      value < 3   ? 1 :
      0
    )

    scope.level = levels[idx]
  }

  function renderUnfoldedItem(scope, row) {
    var a = scope.row.entity.statistics.next.mean
    var b = scope.row.entity.statistics.previous.mean
    scope.ratio = 100 * (a > b ? a / b - 1 : -b / a + 1)
    scope.color = (
      scope.ratio > 0 ? 'red' :
      scope.ratio < 0 ? 'green' :
      'black'
    )
  }

  return {
    restrict: 'E',
    scope: {
      row: '=',
    },
    templateUrl: 'hazard-level-cell',
    link: function(scope) {
      scope.$watch('row', function(row) {
        // TODO It could be devided into two separate directives
        //      with a meta component on top of it
        //      not immediately urgent though
        if (scope.row.groupHeader) {
          renderFoldedItem(scope, row)
        } else {
          renderUnfoldedItem(scope, row)
        }
      })
    }
  }
})
