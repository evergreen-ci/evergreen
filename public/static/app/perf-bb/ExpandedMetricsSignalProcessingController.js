mciModule.controller('ExpandedMetricsSignalProcessingController', function(
  $scope, CHANGE_POINTS_GRID, PerformanceAnalysisAndTriageClient, $routeParams
) {
  $scope.page = 0;
  $scope.pageSize = 10;
  $scope.totalPages = 1;
  $scope.projectId = $routeParams.projectId;

  $scope.prevPage = () => {
    if ($scope.page > 0) {
      $scope.page--;
      getPoints($scope, PerformanceAnalysisAndTriageClient)
    }
  };

  $scope.nextPage = () => {
    if ($scope.page + 1 < $scope.totalPages) {
      $scope.page++;
      getPoints($scope, PerformanceAnalysisAndTriageClient)
    }
  };

  setupGrid($scope, CHANGE_POINTS_GRID);
  getPoints($scope, PerformanceAnalysisAndTriageClient);
});

function handleResponse(result, $scope) {
  $scope.totalPages = result.data.total_pages;
  for (version of result.data.versions) {
    for (cp of version.change_points) {
      $scope.gridOptions.data.push({
        version: version.version_id,
        variant: cp.time_series_info.variant,
        task: cp.time_series_info.task,
        test: cp.time_series_info.test,
        measurement: cp.time_series_info.measurement,
        percent_change: cp.percent_change.toFixed(2),
        triage_status: cp.triage.triage_status,
        thread_level: cp.time_series_info.thread_level,
      })
    }
  }
  $scope.isLoading = false;
}

function getPoints($scope, PerformanceAnalysisAndTriageClient) {
  $scope.gridOptions.data = [];
  $scope.isLoading = true;
  $scope.errorMessage = null;
  PerformanceAnalysisAndTriageClient.getVersionChangePoints($scope.projectId, $scope.page, $scope.pageSize)
      .then(result => handleResponse(result, $scope), err => {
        $scope.isLoading = false;
        $scope.errorMessage = err.data.message;
      });
}

function setupGrid($scope, CHANGE_POINTS_GRID) {
  $scope.gridOptions = {
    enableFiltering: true,
    enableRowSelection: true,
    enableSelectAll: true,
    selectionRowHeaderWidth: 35,
    useExternalFiltering: true,
    useExternalSorting: true,
    data: [],
    columnDefs: [
      {
        name: 'Percent Change',
        field: 'percent_change',
        type: 'number',
        cellTemplate: '<percent-change-cell row="row" ctx="grid.appScope.spvm.refCtx" />',
        width: CHANGE_POINTS_GRID.HAZARD_COL_WIDTH,
      },
      {
        name: 'Variant',
        field: 'variant',
        type: 'string',
      },
      {
        name: 'Task',
        field: 'task',
        type: 'string',
      },
      {
        name: 'Test',
        field: 'test',
        type: 'string',
      },
      {
        name: 'Version',
        field: 'version',
        type: 'string',
        cellTemplate: 'ui-grid-group-name',
        grouping: {
          groupPriority: 0,
        },
      },
      {
        name: 'Thread Level',
        field: 'thread_level',
        type: 'number',
      },
      {
        name: 'Measurement',
        field: 'measurement',
        type: 'string',
      },
      {
        name: 'Triage Status',
        field: 'triage_status',
      },
    ]
  };
}