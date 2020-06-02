mciModule.factory('PerformanceAnalysisAndTriageClient', function ($http, $filter, ApiUtil, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, CEDAR_APP_URL) {
  const client = ApiUtil.httpGetter(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE);

  return {
    getVersionChangePoints: function (projectId, page, pageSize, variantRegex, versionRegex, taskRegex, testRegex, measurementRegex, threadLevels) {
      return client(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.CHANGE_POINTS_BY_VERSION, {
        projectId: projectId,
      }, {
        page: page,
        page_size: pageSize,
        variant_regex: variantRegex,
        version_regex: versionRegex,
        task_regex: taskRegex,
        test_regex: testRegex,
        measurement_regex: measurementRegex,
        thread_levels: threadLevels,
      })
    },
  }
});