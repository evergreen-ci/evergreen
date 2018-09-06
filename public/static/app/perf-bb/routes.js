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
    // This was added for compatibility with server-side routing
    .otherwise({
      redirectTo: function(_, url) {
        // When UI route doesn't exist, redirect user to the URL
        window.location = url
      }
    })
  // Enable client-side routing and history
  $locationProvider.html5Mode({enabled: true, requireBase: false})
})
