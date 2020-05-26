mciModule.factory('ApiUtil', function($http) {
  // TODO Pass CSRF Token (currently, all queries use GET method)
  // TODO Use $httpProvider (tech-debt ticket required)
  return {
    httpGetter: function(base) {
      let realBase = base || '';
      realBase = realBase.endsWith('/') ? realBase.substr(0,realBase.length - 1): realBase;

      return function(apiEndpoint, endpointTplParams, httpParams) {
        if (apiEndpoint[0] !== '/') apiEndpoint = '/' + apiEndpoint;
        return $http.get(
          // Interpolate endpoint template with params
          realBase + _.template(apiEndpoint)(endpointTplParams),
          {params: httpParams, withCredentials: true}
        )
      }
    }
  }
});
