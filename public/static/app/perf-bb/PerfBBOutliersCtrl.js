mciModule.controller('PerfBBOutliersCtrl', function (
  $scope, $window, EvgUiGridUtil, EvgUtil, FORMAT, MDBQueryAdaptor, uiGridConstants,
  STITCH_CONFIG, Stitch, Settings, $timeout, $compile, $log
) {
  // Monkey Patch the toolbar as I can't change outliers.html.
  // TODO don't merge into master.
  const element = $('div.toolbar div.control-group:nth-child(2)');
  const checkbox = $compile('\
<span class="legend">Include low confidence Outliers:</span>\
  <div class="control-group">\
    <input type="checkbox" ng-model="outvm.checkboxModel.lowConfidence"\
     ng-true-value="true" ng-false-value="false" ng-change="outvm.lowConfidenceChanged()" name="checkboxModel">\
    <br/>\
  </div>\
</span>\
')($scope);
  $timeout(() => element.after(checkbox));

  // Perf Failures View-Model.
  const vm = this;
  const project = window.project;
  const LIMIT = 2000;

  vm.checkboxModel = {
    lowConfidence : false,
  };

  vm.mode = {
    options: [{
      id: 'outliers',
      name: 'Outliers',
    }, {
      id: 'marked',
      name: 'Marked',
    }, {
      id: 'muted',
      name: 'Muted',
    }],
    value: 'outliers',
  };

  // Holds currently selected items.
  vm.selection = [];
  // Mark and Unmark actions will be enabled in EVG-5980.
  // Mute and Unmute actions will be enabled in EVG-5981.
  vm.actions = [
    {
      title: 'Mute',
      action: console.log,
      visible: () => vm.state.mode !== 'muted',
      disabled: () => true,
    },
    {
      title: 'Mark',
      action: console.log,
      visible: () => vm.state.mode !== 'marked',
      disabled: () => true,
    },
    {
      title: 'Unmute',
      action: console.log,
      visible: () => vm.state.mode === 'muted',
      disabled: () => true,
    },
    {
      title: 'Unmark',
      action: console.log,
      visible: () => vm.state.mode === 'marked',
      disabled: () => true,
    },
  ];

  // Could not be overridden by persistent user settings.
  const mandatoryDefaultFiltering = {
    project: '=' + $window.project,
    variant: '^((?!wtdevelop).)*$',
    test: '^((?!canary|fio|NetworkB).)*$',
  };

  // Might be overridden by persistent user settings
  const secondaryDefaultFiltering = (state) => {
    return {
      create_time: '>' + state.lookBack.format(FORMAT.ISO_DATE),
    };
  };

  $scope.getFiltering = () => {
    let filtering = {};
    if(vm.gridApi) {
      filtering = _.reduce(vm.gridApi.grid.columns, function (m, d) {
        const term = d.filters[0].term;
        if (term) m[d.field] = term;
        return m;
      }, {});
    }
    if (_.isEmpty(filtering)) {
      filtering = $scope.getDefaultFiltering();
    }
    Settings.perf.outlierProcessing.persistentFiltering = omitTransientFilters(filtering);
    return filtering;
  };

  $scope.getDefaultFiltering = () => {
    let typeFilter = {};
    // If we do not want low confidence outliers then set the type to detected.
    return _.extend(
      typeFilter,
      secondaryDefaultFiltering(vm.state),
      Settings.perf.outlierProcessing.persistentFiltering,
      mandatoryDefaultFiltering
    );
  };

  vm.state = {
    sorting: [{
      field: 'order',
      direction: 'desc',
    }],
    lookBack: moment().subtract(2, 'weeks'),
    mode: vm.mode.value,
    lowConfidence: vm.checkboxModel.value,
  };
  vm.state.filtering = $scope.getDefaultFiltering();

  const modeToCollMap = {
    outliers: STITCH_CONFIG.PERF.COLL_OUTLIERS,
    muted: STITCH_CONFIG.PERF.COLL_MUTE_OUTLIERS,
    marked: STITCH_CONFIG.PERF.COLL_MARKED_OUTLIERS,
  };

  // Enhances filtering state with some contextual meta data
  // This data is required by expression compiler
  function getFilteringContext(state) {
    let defaultFilters = [];
    if(!state.lowConfidence) {
      defaultFilters = [{
        field: 'type',
        type: 'string',
        term: '=detected'
      }];
    };

    const filter = _.reduce(state.filtering, (accum, filter_value, filter_key) => {
      if (!$scope.getCol) {
        return accum;
      }
      const col = $scope.getCol(filter_key);
      if (!col) return accum;  // Error! Associated col does not found

      return accum.concat({
        field: filter_key,
        term: filter_value,
        type: col.colDef.type || 'string',
      })
    }, defaultFilters);
    return filter.length ? filter : null;
  }

  // Creates aggregation expression, which could be used by Stitch
  // for given `state`
  $scope.getAggChain = (state) => {
    let chain = [];

    // Check if the state has filtering
    const filteringChain = MDBQueryAdaptor.compileFiltering(
      // filtering context enhances state data with important meta data
      getFilteringContext(state)
    );
    // check if filtering query was compiled into something
    if (filteringChain) {
      chain.push(filteringChain);
    }

    if (state.sorting) {
      const sortingChain = MDBQueryAdaptor.compileSorting(state.sorting);
      // check if sorting query was compiled into something
      if (sortingChain) {
        chain.push(sortingChain);
      }
    }

    chain.push({$limit: LIMIT});
    return chain;
  };

  $scope.sortRevision = (a, b, rowA, rowB) => {
    // Sort revision by order instead of revision id.
    const nulls = vm.gridApi.core.sortHandleNulls(a, b);
    if (nulls !== null) {
      return nulls;
    }

    if (a === b) {
      return 0;
    }

    if (rowA && rowB) {
      return rowA.entity.order - rowB.entity.order;
    }

    return a - b;
  };

  function hydrateData(docs) {
    _.each(docs, (doc) => {
      doc._buildId = EvgUtil.generateBuildId({
        project: project,
        revision: doc.revision,
        buildVariant: doc.variant,
        createTime: doc.create_time,
      });
    });
  }

  // Required by loadData.
  let theMostRecentPromise;

  function setInitialGridFiltering(gridApi, state) {
    _.each(state.filtering, function (term, colName) {
      const col = $scope.getCol(colName);
      if (!col) return;  // Error! Associated col does not found
      col.filters = [{term: term}];
    });
  }
  // Omit filter fields that we do not want to save in persistent state. At the moment the fields to omit are:
  // 'type', 'create_time', 'project'
  const omitTransientFilters = (filtering, ...fieldNames) => {
    if( !fieldNames || fieldNames.length === 0) {
      fieldNames = ['type', 'create_time', 'project'];
    }
    return _.omit(filtering, fieldNames);
  };


  // Sets `state` to grid filters
  function setInitialGridState(gridApi, state) {
    setInitialGridFiltering(gridApi, state);

    _.each(state.sorting, function (sortingItem) {
      const col = $scope.getCol(sortingItem.field);
      if (!col) return; // Error! Associated col does not found
      col.sort.direction = sortingItem.direction;
    });
  }

  // Load data:
  //    starts the loading indicator
  //    get the remote state from atlas
  //    renders the data (if it is the latest data load event)
  //    clears the loading indicator.
  // Setting filters and updating other vm state is done through relaod() and most methods should
  // use reload..
  function loadData() {
    vm.isLoading = true;
    vm.gridOptions.data = [];
    theMostRecentPromise = Stitch.use(STITCH_CONFIG.PERF).query((db) => {
      return db.db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(modeToCollMap[vm.state.mode])
        .aggregate($scope.getAggChain(vm.state));
    });

    const thisPromise = theMostRecentPromise;
    thisPromise.then((docs) => {
      // If there is more than one concurrent promise - we want the most recent.
      if (thisPromise !== theMostRecentPromise) {
        return;
      }
      theMostRecentPromise
        .then(() => {
          hydrateData(docs);
          vm.gridOptions.data = docs;
        }, (err) => {
          $log.error(err);
        }).finally(() => $timeout(() => vm.isLoading = false));
      // The finally and timeout clear loading on the next event loop iteration so that the Loading indicator
      // disappears when the data load is complete. Otherwise there can be a gap.
    });
  }

  vm.lowConfidenceChanged = () => {
    $log.debug('Low Confidence changed : ' + vm.checkboxModel.lowConfidence);
    vm.state.lowConfidence = vm.checkboxModel.lowConfidence;
    vm.reload()
  };

  vm.modeChanged = () => {
    vm.state.mode = vm.mode.value;
    vm.reload()
  };

  vm.reload = () => {

    // Push state changes to the grid api
    setInitialGridState(vm.gridApi, vm.state);

    // Raise col visibility change event
    vm.gridApi.core.notifyDataChange(uiGridConstants.dataChange.COLUMN);

    // Clear selection
    vm.selection = [];

    loadData();
  };

  function handleRowSelectionChange(gridApi) {
    vm.selection = gridApi.selection.getSelectedRows();
  }

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    enableRowSelection: true,
    enableSelectAll: true,
    selectionRowHeaderWidth: 35,
    useExternalFiltering: true,
    useExternalSorting: false,

    onRegisterApi: (api) => {
      vm.gridApi = api;
      $scope.getCol = EvgUiGridUtil.getColAccessor(api);

      api.core.on.sortChanged($scope, function (grid, cols) {
        vm.state.sorting = _.map(cols, function (col) {
          return {
            field: col.field,
            direction: col.sort.direction
          };
        });
        // NOTE do reload() here for server-side sorting
      });

      const onFilterChanged = _.debounce(function () {
        vm.state.filtering = $scope.getFiltering(vm.state);
        vm.reload();
      }, 200);
      api.core.on.filterChanged($scope, onFilterChanged);

      api.selection.on.rowSelectionChanged(null, _.debounce(() => {
        handleRowSelectionChange(api);
        $scope.$apply();
      }));

      // This is required when user selects all items
      // (rowSelecionChanged doesn't work)
      api.selection.on.rowSelectionChangedBatch(null, () => {
        handleRowSelectionChange(api);
      });

      // Load initial set of data once `columns` are populated
      api.core.on.rowsRendered(null, _.once(function () {
        vm.reload();
      }));
    },

    columnDefs: [
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
        name: 'Type',
        field: 'type',
        type: 'string',
        enableFiltering: false,
      },
      {
        name: 'Revision',
        field: 'revision',
        type: 'string',
        cellFilter: 'limitTo:7',
        width: 100,
        sort: {
          direction: uiGridConstants.DESC,
          priority: 0,
        },
        sortingAlgorithm: $scope.sortRevision,
        cellTemplate: 'ui-grid-group-name',
        grouping: {
          groupPriority: 0,
        },
      },
      {
        name: 'Project',
        field: 'project',
        type: 'string',
        visible: false,
      },
      {
        name: 'Create Time',
        field: 'create_time',
        type: 'date',
      },
    ],
  };
});
