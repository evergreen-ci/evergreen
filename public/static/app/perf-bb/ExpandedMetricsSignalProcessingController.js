mciModule.controller('ExpandedMetricsSignalProcessingController', function(
  $scope, CHANGE_POINTS_GRID, CedarClient, $routeParams
) {
  $scope.page = 0;
  $scope.pageSize = 10;
  $scope.totalPages = 1;
  $scope.projectId = $routeParams.projectId;

  $scope.prevPage = () => {
    if ($scope.page > 0) {
      $scope.page--;
      getPoints($scope, CedarClient)
    }
  };

  $scope.nextPage = () => {
    if ($scope.page + 1 < $scope.totalPages) {
      $scope.page++;
      getPoints($scope, CedarClient)
    }
  };

  setupGrid($scope, CHANGE_POINTS_GRID);
  getPoints($scope, CedarClient);
});

function handleResponse(result, $scope) {
  $scope.totalPages = result.data.total_pages;
  for (version of result.data.versions) {
    for (cp of version.change_points) {
      $scope.gridOptions.data.push({
        version: version.version_id,
        variant: cp.variant,
        task: cp.task,
        test: cp.test,
        measurement: cp.measurement,
        percent_change: cp.percent_change.toFixed(2),
        triage_status: cp.triage.triage_status,
        thread_level: cp.thread_level,
      })
    }
  }
  $scope.isLoading = false;
}

function getPoints($scope, CedarClient) {
  $scope.gridOptions.data = [];
  $scope.isLoading = true;
  $scope.errorMessage = null;
  CedarClient.getVersionChangePoints($scope.projectId, $scope.page, $scope.pageSize)
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