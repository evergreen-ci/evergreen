mciModule.factory('ApiUtil', function($http) {
  // TODO Pass CSRF Token (currently, all queries use GET method)
  // TODO Use $httpProvider (tech-debt ticket required)
  return {
    httpGetter: function(base) {
      var realBase = (function() {
        switch(base) {
          case undefined: return '';
          case '/': return '/';
          default: return base + '/';
        }
      })()

      return function(apiEndpoint, endpointTplParams, httpParams) {
        return $http.get(
          // Interpolate endpoint template with params
          realBase + _.template(apiEndpoint)(endpointTplParams),
          {params: httpParams}
        )
      }
    }
  }
})
