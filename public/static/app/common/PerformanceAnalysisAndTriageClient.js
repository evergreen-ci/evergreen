mciModule.factory('PerformanceAnalysisAndTriageClient', function ($http, $filter, ApiUtil, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, CEDAR_APP_URL) {
  const client = ApiUtil.httpGetter(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE);

  return {
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