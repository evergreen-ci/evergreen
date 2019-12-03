mciModule.controller('PerformanceDiscoveryCtrl', function (
  $q, $scope, $timeout, $window, ApiTaskdata, ApiV1, ApiV2,
  EVG, EvgUiGridUtil, PERF_DISCOVERY, PerfDiscoveryDataService,
  PerfDiscoveryStateService, uiGridConstants, OutliersDataService,
  CANARY_EXCLUSION_REGEX
) {
  var vm = this;
  var gridUtil = EvgUiGridUtil
  var stateUtil = PerfDiscoveryStateService
  var dataUtil = PerfDiscoveryDataService
  var PD = PERF_DISCOVERY
  var grid
  var gridApi
  vm.refCtx = [0, 0]
  vm.jiraHost = $window.JiraHost

  // Load state from the URL
  var state = stateUtil.readState({
    // Default sorting
    sort: {
      ratio: {
        priority: 0,
        direction: uiGridConstants.DESC,
      }
    }
  })

  vm.fromSelect = {
    options: [],
    selected: null,
  }

  vm.toSelect = {
    options: [],
    selected: null,
  }

  var projectId = $window.project

  // For each argument of `arguments`
  // if argument is a function and return value is truthy
  // or argument is truthy non-function return the value
  // If none arguments are truthy returns undefined
  function cascade() {
    var args = Array.prototype.slice.call(arguments)
    for (var i = 0; i < args.length; i++) {
      var ref = args[i]
      var value = _.isFunction(ref) ? ref() : ref
      if (value) return value
    }
  }

  dataUtil.getComparisionOptions(projectId)
    .then(function (items) {
      vm.fromSelect.options = items

      // Sets 'compare from' version from the state if available
      // Sets the first revision from the list otherwise
      vm.fromSelect.selected = cascade(
        _.bind(dataUtil.findVersionItem, null, items),
        _.bind(dataUtil.getQueryBasedItem, null, state.from),
        _.bind(_.findWhere, null, items, {
          kind: PD.KIND_VERSION
        }),
        _.bind(_.first, null, items)
      )

      vm.toSelect.options = items
      // Sets 'compare to' version from the state if available
      // Sets the first tag from the list otherwise
      vm.toSelect.selected = cascade(
        _.bind(dataUtil.findVersionItem, null, items, state.to),
        _.bind(dataUtil.getQueryBasedItem, null, state.to),
        _.bind(_.findWhere, null, items, {
          kind: PD.KIND_TAG
        }),
        _.bind(_.first, null, items)
      )
    })

  // Handles changes in selectFrom/To drop downs
  // Ignores `null` on start up
  $scope.$watch('$ctrl.fromSelect.selected', function (item) {
    item && vm.updateData()
  })

  $scope.$watch('$ctrl.toSelect.selected', function (item) {
    item && vm.updateData()
  })

  $scope.reload = function () {
    vm.updateData(true);
  }

  let oldFromVersion, oldToVersion;

  // Convert the name to a revision that can be used to query atlas.
  const nameToRevision = (name) => {
    if (name) {
      const parts = name.split(/[-_]/);
      name = parts[parts.length - 1];
    }
    return name;
  };

  // Create an object that can be used with _.where().
  const createMatcher = (outlier) => {
    return {
      buildVariant: outlier.variant,
      task: outlier.task,
      test: outlier.test,
      threads: outlier.thread_level !== 'max' ? parseInt(outlier.thread_level) : outlier.thread_level,
    };
  };

  function loadCompOptions(fromVersion, toVersion, expandedCurrent, expandedBaseline, expandedTrend) {
    // Set loading flag to display spinner
    vm.isLoading = true;

    // Load the marked and detected outliers for this revision. If no revision, don't load anything.
    const revision = nameToRevision(fromVersion.name);
    const outliersPromise = $q.all({
      detected: revision ? OutliersDataService.getOutliersQ(projectId, {
        revision: revision
      }) : [],
      marked: revision ? OutliersDataService.getMarkedOutliersQ({
        project: projectId,
        revision: revision
      }) : [],
    });

    $q.all({
        fromVersionObj: dataUtil.getCompItemVersion(fromVersion),
        toVersionObj: dataUtil.getCompItemVersion(toVersion),
      })
      // Load perf data
      .then(function (promise) {
        return dataUtil.getData(
          promise.fromVersionObj, promise.toVersionObj, expandedCurrent, expandedBaseline, expandedTrend
        );
      })
      // Apply perf data
      .then(function (res) {
        vm.gridOptions.data = res
        // Apply options data to filter drop downs
        gridUtil.applyMultiselectOptions(
          res,
          ['build', 'storageEngine', 'task', 'threads'],
          vm.gridApi,
          true
        );
        return res
      })
      // Stop spinner
      .finally(function () {
        vm.isLoading = false
      })
      // Fetch BF tickets and outliers.
      .then(function () {

        const projectRevisionMatcher = {
          project: projectId,
          revision: revision
        };

        outliersPromise.then(function (outliers) {
          // add 'm' for marked.
          _.chain(outliers.marked).where(projectRevisionMatcher).each(function (outlier) {
            _.chain(vm.gridOptions.data)
              .where(createMatcher(outlier))
              .each(task => task.outlier = 'm');
          });

          _.chain(outliers.detected).where(projectRevisionMatcher).each(function (outlier) {
            // add '✓' for marked.
            _.chain(vm.gridOptions.data)
              .where(createMatcher(outlier))
              .each(task => {
                if (task.outlier) {
                  if (task.outlier.indexOf('✓') == -1) {
                    task.outlier += '✓';
                  }
                } else {
                  task.outlier = '✓';
                }
              });
          })
        });

        dataUtil.getBFTicketsForRows(vm.gridOptions.data).then(function (bfGroups) {

          // Match grouped points (by task, bv, test) w/ rows
          _.each(bfGroups, function (bfGroup) {
            // Dedupe by 'key' (BFs might have different revisions or other fields)
            var bfs = _.uniq(bfGroup, function (d) {
              return d.key
            })

            var bf = bfs[0] // using the first element as characteristic

            var matcher = {
              task: bf.tasks,
              buildVariant: bf.buildvariants,
            }

            // If BF has associated test
            bf.tests && (matcher['test'] = bf.tests)

            _.each(_.where(vm.gridOptions.data, matcher), function (task) {
              task.buildFailures = bfs
            })
          })
        })
      })
  }

  vm.updateData = function (force) {
    var fromVersion = vm.fromSelect.selected
    var toVersion = vm.toSelect.selected

    // If nothing has changed, exit the function
    if (!force && (fromVersion == oldFromVersion && toVersion == oldToVersion)) {
      return
    }

    oldFromVersion = fromVersion
    oldToVersion = toVersion

    // Update permalink
    stateUtil.applyState(state, {
      from: fromVersion.id,
      to: toVersion.id,
    })

    // Display no data while loading is in progress
    vm.gridOptions.data = []
    expandedCurrent = vm.expandedOptions && vm.expandedOptions.includes("current");
    expandedBaseline = vm.expandedOptions && vm.expandedOptions.includes("baseline");
    expandedHistory = vm.expandedOptions && vm.expandedOptions.includes("history");

    loadCompOptions(fromVersion, toVersion, expandedCurrent, expandedBaseline, expandedHistory);
  }

  function updateChartContext(grid) {
    // Update context chart data for given rendered rows
    // sets [min, max] list to the scope for visible rows
    vm.refCtx = d3.extent(
      _.reduce(grid.renderContainers.body.renderedRows, function (m, d) {
        return m.concat([
          Math.log(d.entity.avgVsSelf[0]),
          Math.log(d.entity.avgVsSelf[1])
        ])
      }, [])
    )
  }

  // Returns a predefined URL for given `row` and `col`
  // Works with build abd task columns only
  vm.getCellUrl = function (row, col) {
    return row.entity[{
      build: 'buildURL',
      task: 'taskURL',
    } [col.field]]
  }

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    onRegisterApi: function (gridApi) {
      vm.gridApi = gridApi
      grid = gridApi.grid;

      // Using _.once, because this behavior is required on init only
      gridApi.core.on.rowsRendered($scope, function () {
        // For some reason, calback being called before
        // the changes were applied to grid
        // Timeout forces underlying code to be executed at the end
        $timeout(
          _.bind(updateChartContext, null, grid) // When rendered, update charts context
        )
      })

      gridApi.core.on.rowsRendered($scope, _.once(function () { // Do once
        stateUtil.applyStateToGrid(state, grid)
        // Set handlers after grid initialized
        gridApi.core.on.sortChanged(
          $scope, stateUtil.onSortChanged(state)
        )
        gridApi.core.on.filterChanged(
          $scope, stateUtil.onFilteringChanged(state, grid)
        )
      }))

      // Adding grid to $scope to create a watcher
      $scope.grid = grid
      // This triggers on vertical scroll
      $scope.$watch(
        'grid.renderContainers.body.currentTopRow',
        function () {
          updateChartContext(grid)
        }
      )
    },
    columnDefs: [{
        name: 'Tickets',
        field: 'buildFailures',
        cellTemplate: 'perf-discovery-bfs',
        width: 100,
      },
      gridUtil.multiselectColDefMixin({
        name: 'Build',
        field: 'build',
        cellTemplate: 'perf-discovery-link',
      }),
      gridUtil.multiselectColDefMixin({
        name: 'Storage Engine',
        field: 'storageEngine',
      }),
      gridUtil.multiselectColDefMixin({
        name: 'Task',
        field: 'task',
        cellTemplate: 'perf-discovery-link',
      }),
      {
        name: 'Test',
        field: 'test',
        _link: row => '/task/' + row.entity.taskId + '##' + row.entity.test,
        cellTemplate: 'ui-grid-link',
        filter: {
          term: CANARY_EXCLUSION_REGEX.toString().substring(1, CANARY_EXCLUSION_REGEX.toString().length - 1)
        }
      },
      gridUtil.multiselectColDefMixin({
        name: 'Threads',
        field: 'threads',
        type: 'number',
        width: 130,
      }),
      {
        name: 'Ratio',
        field: 'ratio',
        type: 'number',
        cellTemplate: '<perf-discovery-ratio ratio="COL_FIELD" />',
        enableFiltering: false,
        width: 80,
      },
      {
        name: 'Average Ratio',
        field: 'avgRatio',
        type: 'number',
        cellTemplate: '<perf-discovery-ratio ratio="COL_FIELD"/>',
        enableFiltering: false,
        visible: false,
        width: 80,
      },
      {
        name: 'Trend',
        field: 'trendData',
        cellTemplate: '<micro-trend-chart data="COL_FIELD" ctx="grid.appScope.$ctrl.refCtx"/>',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      },
      {
        name: 'Avg and Self',
        field: 'avgVsSelf',
        cellTemplate: '<micro-trend-chart data="COL_FIELD" ctx="grid.appScope.$ctrl.refCtx"/>',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      },
      {
        name: 'ops/sec',
        field: 'speed',
        type: 'number',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 100,
      },
      {
        name: 'Baseline',
        field: 'baseSpeed',
        type: 'number',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 120,
      },
      {
        name: 'Outlier',
        field: 'outlier',
        enableFiltering: false,
        width: 120,
      },
    ]
  }
});