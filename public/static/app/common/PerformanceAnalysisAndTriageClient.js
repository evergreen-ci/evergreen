mciModule.factory('PerformanceAnalysisAndTriageClient', function ($http, $filter, ApiUtil, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, $window) {
  const client = ApiUtil.httpGetter(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE);

  return {
    getAuthenticationUrl: function() {
      return PERFORMANCE_ANALYSIS_AND_TRIAGE_API.AUTH_URL + "?redirect=" + $window.location.href
    },
    getVersionChangePoints: function (projectId, page, pageSize) {
      return client(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.CHANGE_POINTS_BY_VERSION, {
        projectId: projectId,
      }, {
        page: page,
        pageSize: pageSize
      })
    },
  }
});