// TODO Jim: should be moved to python layer (PERF-1546)
mciModule.directive('hazardLevelCell', function() {

  function renderFoldedItem(scope, row) {
    var points = _.map(scope.row.treeNode.children, (d) => d.row.entity)
    scope.points = points
  }

  function ratio(a, b) {
    return 100 * (a > b ? a / b - 1 : -b / a + 1)
  }

  function renderUnfoldedItem(scope, row) {
    var a = scope.row.entity.statistics.next.mean
    var b = scope.row.entity.statistics.previous.mean
    // Check mode (ops/sec or latency)
    scope.ratio = a > 0 ? ratio(a, b) : ratio(b, a)
    scope.color = (
      scope.ratio > 0 ? 'green' :
      scope.ratio < 0 ? 'red' :
      'black'
    )
  }

  return {
    restrict: 'E',
    scope: {
      row: '=',
      ctx: '=',
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
