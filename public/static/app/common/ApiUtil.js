mciModule.factory('ApiUtil', function($http) {
  // TODO Pass CSRF Token (currently, all queries use GET method)
  // TODO Use $httpProvider (tech-debt ticket required)
  return {
    httpGetter: function(base) {
      return function(endpointTpl, endpointTplParams, httpParams) {
        return $http.get(
          // Interpolate endpoint template with params
          endpointTpl(_.extend({base: base}, endpointTplParams)),
          httpParams
        )
      }
    }
  }
})
