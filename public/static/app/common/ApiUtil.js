mciModule.factory('ApiUtil', function($http) {
  // TODO Pass CSRF Token (currently, all queries use GET method)
  // TODO Use $httpProvider (tech-debt ticket required)
  return {
    httpGetter: function(base) {
      console.assert(base[0] == '/', 'An API BASE should starts with /')
      return function(apiEndpoint, endpointTplParams, httpParams) {
        return $http.get(
          // Interpolate endpoint template with params
          base + '/' + _.template(apiEndpoint)(endpointTplParams),
          {params: httpParams}
        )
      }
    }
  }
})
