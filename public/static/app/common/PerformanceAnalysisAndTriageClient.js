mciModule.factory('PerformanceAnalysisAndTriageClient', function ($http, $filter, ApiUtil, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, $window) {
  const getClient = ApiUtil.httpGetter(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE);
  const postClient = ApiUtil.httpPoster(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE);

  return {
    getAuthenticationUrl: function() {
      return PERFORMANCE_ANALYSIS_AND_TRIAGE_API.AUTH_URL + "?redirect=" + $window.location.href
    },
    getVersionChangePoints: function (projectId, page, pageSize, variantRegex, versionRegex, taskRegex, testRegex, measurementRegex, threadLevels, triageStatusRegex, calculatedOnWindow, percentChangeWindows, sortAscending) {
      return getClient(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.CHANGE_POINTS_BY_VERSION, {
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
        sort_ascending: sortAscending
      })
    },
    triagePoints: function(changePointIds, status) {
      return postClient(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.TRIAGE_POINTS, {}, {
        change_point_ids: changePointIds,
        status: status,
      });
    }
  }
});