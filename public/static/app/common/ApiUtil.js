mciModule.factory('ApiUtil', function($http) {
  // TODO Pass CSRF Token (currently, all queries use GET method)
  // TODO Use $httpProvider (tech-debt ticket required)
  return {
    getCallFn: function(base) {
      return function(api, apiParams, httpParams) {
        return $http.get(
          api(_.extend({base: base}, apiParams))
        )
      }
    }
  }
})
