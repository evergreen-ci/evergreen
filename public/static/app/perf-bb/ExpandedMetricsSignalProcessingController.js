mciModule.controller('ExpandedMetricsSignalProcessingController', function(
  $scope, CHANGE_POINTS_GRID, PerformanceAnalysisAndTriageClient, $routeParams, PERFBB_HAZARD_VALUES
) {
  $scope.page = 0;
  $scope.pageSize = 10;
  $scope.totalPages = 1;
  $scope.projectId = $routeParams.projectId;
  $scope.measurementRegex = 'AverageLatency|Latency50thPercentile|Latency95thPercentile';
  $scope.hazardValues = PERFBB_HAZARD_VALUES;

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

  $scope.getAuthenticationUrl = PerformanceAnalysisAndTriageClient.getAuthenticationUrl;
  setupGrid($scope, CHANGE_POINTS_GRID, _.partial(onFilterChanged, $scope, _.partial(getPoints, $scope, PerformanceAnalysisAndTriageClient)));

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
        calculated_on: cp.calculated_on
      })
    }
  }
  $scope.isLoading = false;
}

function getPoints($scope, PerformanceAnalysisAndTriageClient) {
  $scope.gridOptions.data = [];
  $scope.isLoading = true;
  $scope.errorMessage = null;
  PerformanceAnalysisAndTriageClient.getVersionChangePoints($scope.projectId, $scope.page, $scope.pageSize, $scope.variantRegex, $scope.versionRegex, $scope.taskRegex, $scope.testRegex, $scope.measurementRegex, $scope.threadLevels, $scope.calculatedOnWindow, $scope.percentChangeWindows)
      .then(result => handleResponse(result, $scope), err => {
        $scope.isLoading = false;
        $scope.connectionError = false;
        if(err.data) {
          $scope.errorMessage = err.data.message;
        } else {
          $scope.connectionError = true
        }
      });
}

function onFilterChanged($scope, loaderFunction) {
  if($scope.cancelLoading) {
    $scope.cancelLoading()
  }
  this.grid.columns.forEach(function (col) {
    if(col.field === "variant") {
      $scope.variantRegex = col.filters[0].term;
    } else if (col.field === "version") {
      $scope.versionRegex = col.filters[0].term;
    } else if (col.field === "task") {
      $scope.taskRegex = col.filters[0].term;
    } else if (col.field === "test") {
      $scope.testRegex = col.filters[0].term;
    } else if (col.field === "measurement") {
      $scope.measurementRegex = col.filters[0].term;
    } else if (col.field === "thread_level") {
      let tls = col.filters[0].term;
      $scope.threadLevels = tls ? tls.split(',') : undefined;
    } else if (col.field === 'calculated_on') {
      let dates = col.filters[0].term;
      if (dates) {
        let startDate = dates[0];
        let endDate = dates[dates.length - 1];
        startDate = startDate ? startDate.toISOString() : '';
        endDate = endDate ? endDate.toISOString() : '';
        $scope.calculatedOnWindow = `${startDate.replace('Z','')},${endDate.replace('Z','')}`
      }
    } else if (col.field === 'percent_change') {
      let selected = col.filters[0].term;
      $scope.percentChangeWindows = [];
      if (selected) {
        selected.forEach(function (category) {
          if (category === "Major Regression") {
            $scope.percentChangeWindows.push(",-50")
          } else if (category === "Moderate Regression") {
            $scope.percentChangeWindows.push("-50,-20")
          } else if (category === "Minor Regression") {
            $scope.percentChangeWindows.push("-20,0")
          } else if (category === "No Change") {
            $scope.percentChangeWindows.push("0,0")
          } else if (category === "Minor Improvement") {
            $scope.percentChangeWindows.push("0,20")
          } else if (category === "Moderate Improvement") {
            $scope.percentChangeWindows.push("20,50")
          } else if (category === "Major Improvement") {
            $scope.percentChangeWindows.push("50,")
          }
        })
      }
    }
  });
  new Promise((resolve, reject) => {
    $scope.cancelLoading = reject;
    setTimeout(resolve, 1000);
  }).then(loaderFunction).catch(() => {});
}

function setupGrid($scope, CHANGE_POINTS_GRID, onFilterChanged) {
  $scope.gridOptions = {
    enableFiltering: true,
    enableSorting: false,
    enableRowSelection: true,
    enableSelectAll: true,
    selectionRowHeaderWidth: 35,
    useExternalFiltering: true,
    data: [],
    onRegisterApi: function( gridApi ) {
      gridApi.core.on.filterChanged($scope, onFilterChanged);
    },
    columnDefs: [
      {
        name: 'Percent Change',
        field: 'percent_change',
        cellTemplate: '<percent-change-cell row="row" ctx="grid.appScope.spvm.refCtx" />',
        filterHeaderTemplate: `
          <md-input-container style="margin:0">
            <md-select ng-model="col.filters[0].term" multiple>
              <md-option ng-value="hv" ng-repeat="hv in col.filters[0].hazardValues">{{hv}}</md-option>
            </md-select>
          </md-input-container>
        `,
        width: CHANGE_POINTS_GRID.HAZARD_COL_WIDTH,
        filter: {
          term: null,
          hazardValues: $scope.hazardValues
        },
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
        filter: {
          term: $scope.measurementRegex
        }
      },
      {
        name: 'Triage Status',
        field: 'triage_status',
      },
      {
        name: 'Calculated On',
        field: 'calculated_on',
        filterHeaderTemplate: '<md-date-range one-panel="true" auto-confirm="true" ng-model="selectedDate" md-on-select="col.filters[0].term = $dates"></md-date-range>',
        filter: {
          term: null
        }
      }
    ]
  };
}