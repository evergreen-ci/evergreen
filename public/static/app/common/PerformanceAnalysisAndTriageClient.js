mciModule.factory('PerformanceAnalysisAndTriageClient', function ($http, $filter, ApiUtil, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, $window) {
  const client = ApiUtil.httpGetter(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE);

  return {
    getAuthenticationUrl: function() {
      return PERFORMANCE_ANALYSIS_AND_TRIAGE_API.AUTH_URL + "?redirect=" + $window.location.href
    },
    getVersionChangePoints: function (projectId, page, pageSize, variantRegex, versionRegex, taskRegex, testRegex, measurementRegex, threadLevels, triageStatusRegex, calculatedOnWindow, percentChangeWindows) {
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
        calculated_on: calculatedOnWindow,
        percent_change: percentChangeWindows,
        triage_status_regex: triageStatusRegex,
      })
    },
  }
});