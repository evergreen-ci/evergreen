mciModule.directive('perfDiscoveryRatio', function() {
  var scale = d3.scale.linear()
    .domain([0, 1, 2])
    .range(['red', 'white', 'green'])
    .clamp(true)

  return {
    restrict: 'E',
    scope: {
      ratio: '=',
    },
    templateUrl: 'perf-discovery-ratio',
    link: function (scope) {
      // This $watch is required due to known bug with ui-grid
      // https://github.com/angular-ui/ui-grid/issues/4869
      scope.$watch('ratio', function(val) {
        scope.color = scale(scope.ratio)
      })
    }
  }
})
