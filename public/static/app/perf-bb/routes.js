mciModule.config(function($routeProvider, $locationProvider) {
  $routeProvider
    .when('/perf-bb/change-points/:projectId?', {
      templateUrl: '/static/app/perf-bb/change-points.html',
      controller: 'SignalProcessingCtrl',
      controllerAs: 'spvm',
    })
    .when('/perf-bb/warnings/:projectId?', {
      templateUrl: '/static/app/perf-bb/warnings.html',
      controller: 'SignalProcessingCtrl',
      controllerAs: 'wvm',
    })
    .otherwise({
      redirectTo: function(_, url) {
        window.location = url
      }
    })
  $locationProvider.html5Mode({enabled: true, requireBase: false})
})
