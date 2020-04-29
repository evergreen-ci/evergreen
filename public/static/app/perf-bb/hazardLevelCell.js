// TODO Jim: should be moved to python layer (PERF-1546)
mciModule.directive('hazardLevelCell', function() {

  function renderFoldedItem(scope, row) {
    scope.points = _.map(scope.row.treeNode.children, (d) => d.row.entity)
  }

  function ratio(a, b) {
    return 100 * ((a/b) - 1);
  }

  function renderUnfoldedItem(scope, row) {
    var a = scope.row.entity.statistics.next.mean;
    var b = scope.row.entity.statistics.previous.mean;
    scope.ratio = ratio(a, b);
    // a < 0 is a latency change, ensure ratio correct.
    if (a < 0) {
      scope.ratio = scope.ratio * -1
    }
    scope.color = (
      scope.ratio > 0 ? 'green' :
      scope.ratio < 0 ? 'red' :
      'black'
    );
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
});
