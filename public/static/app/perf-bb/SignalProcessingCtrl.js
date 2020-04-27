mciModule.controller('SignalProcessingCtrl', function(
  $log, $scope, $timeout, $window, ChangePointsService, CHANGE_POINTS_GRID,
  EvgUiGridUtil, EvgUtil, FORMAT, MDBQueryAdaptor, PROCESSED_TYPE,
  Settings, STITCH_CONFIG, Stitch, uiGridConstants, ModeToItemVisibilityMap
) {
  const vm = this;
  // Ui grid col accessor

  // TODO later this might be replaced with some sort of pagination
  const LIMIT = 500;

  const DEFAULT_MAGNITUDE = .05

  vm.mode = {
    options: [{
      id: 'processed',
      name: 'Processed',
    }, {
      id: 'unprocessed',
      name: 'Unprocessed',
    }],
    value: 'unprocessed',
  };

  // Holds currently selected items
  vm.selection = [];

  vm.defaultChangePointFilter = {
    create_time: '>' + moment().subtract(2, 'weeks').format(FORMAT.ISO_DATE),
    project: '=' + $window.project,
    probability: '>0.05',
    magnitude: `>=${DEFAULT_MAGNITUDE}`
  };

  vm.changePointFilter = function() {
    return _.extend(
      {},
      vm.defaultChangePointFilter,
      Settings.perf.signalProcessing.persistentFiltering,
    );
  };

  let state = {
    sorting: [{
      field: 'suspect_revision',
      direction: 'asc',
    }, {
      field: 'magnitude',
      direction: 'asc',
    }],
    filtering: vm.changePointFilter(),
    mode: vm.mode.value,
  };

  const modeToCollMap = {
    unprocessed: STITCH_CONFIG.PERF.COLL_UNPROCESSED_POINTS,
    processed: STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS,
  };

  function refreshGridData(gridOptions) {
    gridOptions.data = _.filter(gridOptions.data, ModeToItemVisibilityMap[state.mode]);
    vm.gridApi.selection.clearSelectedRows();
    handleRowSelectionChange(vm.gridApi);
  }

  const markFn = function(mark, items) {
    ChangePointsService.markPoints(items, mark, state.mode).then(function(ok) {
      if (!ok) return;
      refreshGridData(vm.gridOptions);
      // Update selection
      handleRowSelectionChange(vm.gridApi);
    });
  };

  vm.actions = [{
    title:    'Hide',
    action:   _.partial(markFn, PROCESSED_TYPE.HIDDEN),
    visible:  _.constant(true),
    disabled: _.isEmpty,
  }, {
    title:    'Acknowledge',
    action:   _.partial(markFn, PROCESSED_TYPE.ACKNOWLEDGED),
    visible:  _.constant(true),
    disabled: _.isEmpty,
  }, {
    title:    'Unmark',
    action:   _.partial(markFn, PROCESSED_TYPE.NONE),
    visible:  function() { return state.mode === 'processed' },
    disabled: _.isEmpty,
  }];

  // Required by loadData.
  let theMostRecentPromise;

  vm.loadData = function(state) {
    vm.isLoading = true;
    vm.gridOptions.data = [];
    theMostRecentPromise = Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(modeToCollMap[state.mode])
        .aggregate(getAggChain(state))
    });
    // Storing this promise in closure.
    const thisPromise = theMostRecentPromise;
    thisPromise.then(function(docs) {
      // There more than one concurring promises - we want the most recent one
      if (thisPromise !== theMostRecentPromise) {
        return;
      }
      theMostRecentPromise
        .then(function() {
          // Hydrate data (generate build id and version id)
          hydrateData(docs);
          vm.gridOptions.data = docs;
        }, function(err) {
          $log.error(err);
        }).finally(function() {
          vm.isLoading = false;
        })
    })
  }

  function hydrateData(docs) {
    _.each(docs, function(doc) {
      // '_' is required to distinguish generate data
      doc._versionId = EvgUtil.generateVersionId({
        project: project,
        revision: doc.suspect_revision,
      });
      doc._buildId = EvgUtil.generateBuildId({
        project: project,
        revision: doc.suspect_revision,
        buildVariant: doc.variant,
        createTime: doc.create_time,
      });
    });
  }

  // Enhances filtering state with some contextual meta data
  // This data is required by expression compiler
  function getFilteringContext(state) {
    return _.reduce(state.filtering, function(m, v, k) {
      const col = vm.getCol(k);
      if (!col) return m;  // Error! Associated col does not found
      return m.concat({
        field: k,
        term: v,
        type: col.colDef.type || 'string',
      });
    }, []);
  }

  // Creates aggregation expression, which could be used by Stitch
  // for given `state`
  function getAggChain(state) {
    let chain = [];

    // Check if the state has filtering
    if (!_.isEmpty(state.filtering)) {
      const filteringChain = MDBQueryAdaptor.compileFiltering(
        // filtering context enhances state data with important meta data
        getFilteringContext(state)
      );
      // check if filtering query was compiled into something
      filteringChain && chain.push(filteringChain);
    }

    if (state.sorting) {
      const sortingChain = MDBQueryAdaptor.compileSorting(state.sorting);
      // check if sorting query was compiled into something
      sortingChain && chain.push(sortingChain);
    }

    chain.push({$limit: LIMIT});
    return chain;
  }

  vm.modeChanged = function() {
    state.mode = vm.mode.value;
    // Show/hide column depending on mode
    const col = vm.getCol('processed_type');

    if (state.mode === 'processed') {
      col.showColumn();
      // Add filtering by processed_type
      state.filtering.processed_type = '=' + PROCESSED_TYPE.ACKNOWLEDGED;
    } else {
      col.hideColumn();
      // Remove filter by processed type
      delete state.filtering.processed_type;
    }

    // Push state changes to the grid api
    setInitialGridState(vm.gridApi, state);

    // Raise col visibility change event
    vm.gridApi.core.notifyDataChange(uiGridConstants.dataChange.COLUMN);

    // Clear selection
    vm.selection = [];

    vm.loadData(state);
  };

  vm.setInitialGridFiltering = function (gridApi, state) {
    _.each(state.filtering, function(term, colName) {
      const col = vm.getCol(colName);
      if (!col) return;  // Error! Associated col does not found
      col.filters = [{term: term}];
    });
  }

  // Sets `state` to grid filters
  function setInitialGridState(gridApi, state) {
    vm.setInitialGridFiltering(gridApi, state);

    _.each(state.sorting, function(sortingItem) {
      const col = vm.getCol(sortingItem.field);
      if (!col) return; // Error! Associated col does not found
      col.sort.direction = sortingItem.direction;
    });
  }

  function handleRowSelectionChange(gridApi) {
    vm.selection = gridApi.selection.getSelectedRows();
  }

  vm.refCtx = 0;
  function updateChartContext(grid) {
    // Update context chart data for given rendered rows
    vm.refCtx = d3.max(
      _.map(grid.renderContainers.body.renderedRows, function(d) {
        return d.treeNode.children.length;
      })
    );
  }

  vm.onFilterChanged = function (api) {
    Settings.perf.signalProcessing.persistentFiltering = _.reduce(api.grid.columns, function (m, d) {
      if (d.visible) {
        const term = d.filters[0].term;
        if (term) {
          m[d.field] = term;
        }
      }
      return m;
    }, {});

    state.filtering = vm.changePointFilter();
    vm.setInitialGridFiltering(vm.gridApi, state);

    vm.loadData(state);
  };

  $scope.sortRevision = (a, b, rowA, rowB) => {
    // Sort revision by order instead of revision id.
    const nulls = vm.gridApi.core.sortHandleNulls(a, b);
    if (nulls !== null) {
      return nulls;
    }

    return EvgUiGridUtil.sortByOrder(a, b, rowA, rowB);
  };

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    enableRowSelection: true,
    enableSelectAll: true,
    selectionRowHeaderWidth: 35,
    useExternalFiltering: true,
    useExternalSorting: false,
    onRegisterApi: function(api) {
      vm.gridApi = api;
      vm.getCol = EvgUiGridUtil.getColAccessor(api);
      api.core.on.sortChanged($scope, function(grid, cols) {
        state.sorting = _.map(cols, function(col) {
          return {
            field: col.field,
            direction: col.sort.direction
          };
        });
        // NOTE do loadData(state) here for server-side sorting
      });

      const onFilterChanged = _.debounce(_.partial(vm.onFilterChanged, api), 200);

      api.core.on.filterChanged($scope, onFilterChanged);

      // Load initial set of data once `columns` are populated
      api.core.on.rowsRendered(null, _.once(function() {
        setInitialGridState(api, state);
        vm.loadData(state);
      }));

      // Debounce is neat when selecting multiple items
      api.selection.on.rowSelectionChanged(null, _.debounce(function() {
        handleRowSelectionChange(api);
        // This function executed asynchronously, so we should call $apply manually
        $scope.$apply();
      }));

      // This is required when user selects all items
      // (rowSelecionChanged doesn't work)
      api.selection.on.rowSelectionChangedBatch(null, function() {
        handleRowSelectionChange(api);
      });

      // Using _.once, because this behavior is required on init only
      api.core.on.rowsRendered($scope, function() {
        // Timeout forces underlying code to be executed at the end
        $timeout(
          _.bind(updateChartContext, null, api.grid) // When rendered, update charts context
        );
      });

      $scope.$watch(
        'grid.renderContainers.body.currentTopRow', function() {
          updateChartContext(api.grid);
        }
      );
    },
    columnDefs: [
      {
        // TODO Jim: Should be managed by PERF-1546
        name: 'Hazard Level',
        field: 'magnitude',
        type: 'number',
        cellTemplate: '<hazard-level-cell row="row" ctx="grid.appScope.spvm.refCtx" />',
        width: CHANGE_POINTS_GRID.HAZARD_COL_WIDTH,
      },
      {
        name: 'Variant',
        field: 'variant',
        type: 'string',
        _link: row => '/build/' + row.entity._buildId,
        cellTemplate: 'ui-grid-link',
      },
      {
        name: 'Task',
        field: 'task',
        type: 'string',
        _link: row => '/task/' + row.entity.task_id,
        cellTemplate: 'ui-grid-link',
      },
      {
        name: 'Test',
        field: 'test',
        type: 'string',
        _link: row => '/task/' + row.entity.task_id + '##' + row.entity.test,
        cellTemplate: 'ui-grid-link',
      },
      {
        name: 'Revision',
        field: 'suspect_revision',
        type: 'string',
        cellFilter: 'limitTo:7',
        width: 100,
        sort: {
          direction: uiGridConstants.DESC,
          priority: 0,
        },
        cellTemplate: 'ui-grid-group-name',
        sortingAlgorithm: $scope.sortRevision,
        grouping: {
          groupPriority: 0,
        },
      },
      {
        name: 'Value',
        field: 'value',
        cellFilter: 'number:2',
        type: 'number',
        visible: false,
      },
      {
        name: 'Value to Avg',
        field: 'value_to_avg',
        cellFilter: 'number:2',
        type: 'number',
        visible: false,
      },
      {
        name: 'Probability',
        field: 'probability',
        cellFilter: 'number:2',
        type: 'number',
        visible: false,
      },
      {
        name: 'Average',
        field: 'average',
        cellFilter: 'number:2',
        visible: false,
        type: 'number',
      },
      {
        name: 'Average Diff',
        field: 'average_diff',
        cellFilter: 'number:2',
        visible: false,
        type: 'number',
      },
      {
        name: 'Value to Avg Diff',
        field: 'value_to_avg_diff',
        cellFilter: 'number:2',
        visible: false,
        type: 'number',
      },
      {
        name: 'Processed Type',
        field: 'processed_type',
        visible: false,
      },
      {
        name: 'Thread Level',
        field: 'thread_level',
        type: 'number',
      },
      {
        name: 'Create Time',
        field: 'create_time',
        type: 'date',
      },
      {
        name: 'Project',
        field: 'project',
        type: 'string',
        visible: false,
      },
      {
        name: 'Min. Magnitude',
        field: 'min_magnitude',
        type: 'number',
        visible: false,
      },
      {
        name: 'Magnitude',
        field: 'magnitude',
        type: 'number',
        visible: false,
      },
      {
        name: 'Prev. Mean',
        field: 'statistics.previous.mean',
        type: 'number',
        visible: false,
      },
      {
        name: 'Next Mean',
        field: 'statistics.next.mean',
        type: 'number',
        visible: false,
      },
    ]
  };
}).factory('ModeToItemVisibilityMap', function (PROCESSED_TYPE) {
  const ModeToItemVisibilityMap = {
    unprocessed: (item) => item.processed_type !== PROCESSED_TYPE.HIDDEN && item.processed_type !== PROCESSED_TYPE.ACKNOWLEDGED,
    processed: (item) => item.processed_type !== PROCESSED_TYPE.NONE,
  };

  return ModeToItemVisibilityMap;
});
