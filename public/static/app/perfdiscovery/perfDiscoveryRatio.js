mciModule.directive('perfDiscoveryRatio', function() {
  return {
    restrict: 'E',
    scope: {
      ratio: '=',
    },
    templateUrl: 'perf-discovery-ratio',
    link: function (scope) {
      scope.color = null;
    }
  }
});
