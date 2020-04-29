mciModule.directive('percentChangeCell', function() {

  function renderFoldedItem(scope) {
    scope.points = _.map(scope.row.treeNode.children, (d) => d.row.entity)
  }

  function renderUnfoldedItem(scope, row) {
    var change = row.entity.percent_change;
    if (change === undefined) {
      scope.ratio = change;
      scope.color = 'red';
    } else {
      scope.ratio = change;
      scope.color = scope.ratio > 0 ? 'green' : scope.ratio < 0 ? 'red' : 'black';
    }
  }

  return {
    restrict: 'E',
    scope: {
      row: '=',
      ctx: '=',
    },
    templateUrl: 'percent-change-cell',
    link: function(scope) {
      scope.$watch('row', function(row) {
        if (scope.row.groupHeader) {
          renderFoldedItem(scope, row)
        } else {
          renderUnfoldedItem(scope, row)
        }
      })
    }
  }
});
