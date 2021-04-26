mciModule.controller('ExpandedMetricsSignalProcessingController', function (
  $scope, CHANGE_POINTS_GRID, PerformanceAnalysisAndTriageClient, $routeParams, ApiV2, EvgUtil, $q
) {
  $scope.page = 0;
  $scope.pageSize = 10;
  $scope.totalPages = 1;
  $scope.projectId = $routeParams.projectId;
  $scope.measurementRegex = 'Latency50thPercentile|Latency95thPercentile';
  // Look at all tasks except genny_canaries.
  $scope.taskRegex = '^(?!.*(genny_canaries).*)';
  // Don't look at any test-specific canaries (eg. canary_InsertBigDocs.ActorFinished).
  $scope.testRegex = '^(?!.*(canary_|Cleanup).*)';
  $scope.hazardValues = [
    'Major Regression',
    'Moderate Regression',
    'Minor Regression',
    'No Change',
    'Minor Improvement',
    'Moderate Improvement',
    'Major Improvement',
  ];
  $scope.triageStatusRegex = "not_triaged";
  $scope.getVersion = ApiV2.getVersionById;
  $scope.EvgUtil = EvgUtil;
  $scope.q = $q;

  function refreshGridData($scope) {
    $scope.gridApi.selection.clearSelectedRows();
    handleRowSelectionChange($scope, $scope.gridApi);
    getPoints($scope, PerformanceAnalysisAndTriageClient)
  }

  $scope.actions = [{
    title: 'Not Triaged',
    value: "not_triaged"
  }, {
    title: 'True Positive',
    value: "true_positive",
  }, {
    title: 'False Positive',
    value: "false_positive"
  }, {
    title: 'Under Investigation',
    value: "under_investigation"
  }];
  // Holds currently selected items
  $scope.selection = [];
  $scope.selectedAction = null;

  $scope.isNoActionSelected = function () {
    return $scope.selectedAction === null;
  };

  $scope.triagePoints = function () {
    var cpIds = [];
    for (cp of $scope.selection) {
      cpIds.push(cp.id);
    }
    PerformanceAnalysisAndTriageClient.triagePoints(cpIds, $scope.selectedAction.value)
      .then(() => refreshGridData($scope), err => {
        $scope.isLoading = false;
        $scope.connectionError = false;
        if (err.data) {
          $scope.errorMessage = err.data.message;
        } else {
          $scope.connectionError = true
        }
      });
  };

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
    let datas = [];
    let cps = version.change_points;
    for (cp of cps) {
      let data = {
        id: cp._id,
        version: version.version_id,
        variant: cp.time_series_info.variant,
        task: cp.time_series_info.task,
        test: cp.time_series_info.test,
        measurement: cp.time_series_info.measurement,
        percent_change: cp.percent_change.toFixed(2),
        triage_status: cp.triage.triage_status,
        thread_level: cp.time_series_info.thread_level,
        calculated_on: cp.calculated_on
      };
      $scope.gridOptions.data.push(data);
      datas.push(data);
    }
    $scope.getVersion(version.version_id)
      .then(resp => {
        for (data of datas) {
          data['revision'] = resp.data.revision;
          data['revision_time'] = resp.data.create_time;
          data['build_id'] = $scope.EvgUtil.generateBuildId({
            project: $scope.projectId,
            revision: data.revision,
            buildVariant: data.variant,
            createTime: resp.data.create_time,
          });
          data['task_id'] = $scope.EvgUtil.generateTaskId({
            project: $scope.projectId,
            revision: resp.data.revision,
            buildVariant: data.variant,
            task: data.task,
            createTime: resp.data.create_time,
          });
        }
      });
  }
  $scope.isLoading = false;
}

function getPoints($scope, PerformanceAnalysisAndTriageClient) {
  $scope.gridOptions.data = [];
  $scope.isLoading = true;
  $scope.errorMessage = null;
  PerformanceAnalysisAndTriageClient.getVersionChangePoints($scope.projectId, $scope.page, $scope.pageSize, $scope.variantRegex, $scope.versionRegex, $scope.taskRegex, $scope.testRegex, $scope.measurementRegex, $scope.threadLevels, $scope.triageStatusRegex, $scope.calculatedOnWindow, $scope.percentChangeWindows, $scope.sortAscending)
    .then(result => handleResponse(result, $scope), err => {
      $scope.isLoading = false;
      $scope.connectionError = false;
      if (err.data) {
        $scope.errorMessage = err.data.message;
      } else {
        $scope.connectionError = true
      }
    });
}

function handleRowSelectionChange($scope, gridApi) {
  $scope.selection = gridApi.selection.getSelectedRows();
}

function onFilterChanged($scope, loaderFunction) {
  if ($scope.cancelLoading) {
    $scope.cancelLoading()
  }
  this.grid.columns.forEach(function (col) {
    if (col.field === "variant") {
      $scope.variantRegex = col.filters[0].term;
    } else if (col.field === "version") {
      $scope.sortAscending = col.sort.direction === "asc";
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
        $scope.calculatedOnWindow = `${startDate.replace('Z', '')},${endDate.replace('Z', '')}`
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
    } else if (col.field === "triage_status") {
      $scope.triageStatusRegex = col.filters[0].term;
    }
  });
  new Promise((resolve, reject) => {
    $scope.cancelLoading = reject;
    setTimeout(resolve, 1000);
  }).then(loaderFunction).catch(() => {
  });
}

function setupGrid($scope, CHANGE_POINTS_GRID, onFilterChanged) {
  $scope.gridOptions = {
    enableFiltering: true,
    enableRowSelection: true,
    enableSorting: true,
    enableSelectAll: true,
    selectionRowHeaderWidth: 35,
    useExternalFiltering: true,
    useExternalSorting: true,
    data: [],
    onRegisterApi: function (gridApi) {
      $scope.gridApi = gridApi;
      gridApi.core.on.filterChanged($scope, onFilterChanged);
      gridApi.selection.on.rowSelectionChanged(null, _.debounce(function () {
        handleRowSelectionChange($scope, gridApi);
        $scope.$apply();
      }));

      gridApi.selection.on.rowSelectionChangedBatch(null, function () {
        handleRowSelectionChange($scope, gridApi);
      });

      gridApi.core.on.sortChanged($scope, onFilterChanged)
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
        name: 'Revision',
        field: 'revision',
        type: 'string',
        enableFiltering: false
      },
      {
        name: 'Date',
        field: 'revision_time',
        type: 'string',
        enableFiltering: false
      },
      {
        name: 'Variant',
        field: 'variant',
        type: 'string',
        enableSorting: false,
        _link: row => '/build/' + row.entity.build_id,
        cellTemplate: 'ui-grid-link',
      },
      {
        name: 'Task',
        field: 'task',
        type: 'string',
        enableSorting: false,
        _link: row => '/task/' + row.entity.task_id,
        cellTemplate: 'ui-grid-link',
        filter: {
          term: $scope.taskRegex
        }
      },
      {
        name: 'Test',
        field: 'test',
        type: 'string',
        enableSorting: false,
        _link: row => '/task/' + row.entity.task_id + '##' + row.entity.test,
        cellTemplate: 'ui-grid-link',
        filter: {
          term: $scope.testRegex
        }
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
        enableSorting: false,
      },
      {
        name: 'Measurement',
        field: 'measurement',
        type: 'string',
        filter: {
          term: $scope.measurementRegex
        },
        enableSorting: false,
      },
      {
        name: 'Triage Status',
        field: 'triage_status',
        type: 'string',
        filter: {
          term: $scope.triageStatusRegex
        },
        enableSorting: false,
      },
      {
        name: 'Calculated On',
        field: 'calculated_on',
        filterHeaderTemplate: '<md-date-range one-panel="true" auto-confirm="true" ng-model="selectedDate" md-on-select="col.filters[0].term = $dates"></md-date-range>',
        filter: {
          term: null
        },
        enableSorting: false,
      }
    ]
  };
}