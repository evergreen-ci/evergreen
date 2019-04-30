mciModule.controller('PerfBBRejectsCtrl', function (
  $scope, $window, EvgUiGridUtil, EvgUtil, FORMAT, MDBQueryAdaptor, uiGridConstants,
  STITCH_CONFIG, Stitch, Settings, $timeout, $compile, $log, WhitelistService, $q, RejectState
) {
  // Perf Rejects View-Model.
  const vm = this;
  const project = window.project;
  const LIMIT = 2000;

  const updateToWhitelist = function(items, func, mark) {
    const task_revisions = _.chain(items).
                              map((item) => _.pick(item, 'revision', 'project', 'variant', 'task')).
                              uniq((item) => _.values(item).join('-')).
                              value();
    $log.debug(task_revisions);
    const promise = func(task_revisions);
    promise.then(function(ok) {
      if (!ok) return;
      _.each(task_revisions, (task_revision) => {
        _.chain(vm.gridOptions.data).where(task_revision).each((doc)=> {
          doc.whitelisted = mark;
        }).value();
      });
      refreshGridData();
      // Call vm.reload() to do a full reload from the server.
    });

  };

  $scope.addWhitelist = function(items) {
    return updateToWhitelist(items, WhitelistService.addWhitelist, "✔");
  };

  $scope.removeWhitelist = function(items) {
    return updateToWhitelist(items, WhitelistService.removeWhitelist);
  };

  function refreshGridData() {
    vm.gridApi.selection.clearSelectedRows();
    handleRowSelectionChange(vm.gridApi);
  }

  // Holds currently selected items.
  vm.selection = [];
  vm.actions = [
    {
      title: 'Add',
      action: $scope.addWhitelist,
      disabled: () => vm.selection.length == 0 || _.all(vm.selection, (doc)=> doc.whitelisted)
    },
    {
      title: 'Remove',
      action: $scope.removeWhitelist,
      disabled: () => vm.selection.length == 0 || _.all(vm.selection, (doc)=> !doc.whitelisted)
    },
  ];

  vm.state = new RejectState(project, vm, $scope);

  // Creates aggregation expression, which could be used by Stitch
  // for given `state`
  $scope.getAggChain = (state) => {
    let chain = [];

    // Check if the state has filters
    let filteringChain = MDBQueryAdaptor.compileFiltering(
      // filters context enhances state data with important meta data
      state.getFilteringContext()
    );

    if (!filteringChain) {
      filteringChain = {$match:{}};
    }
    if(!filteringChain['$match']) {
      filteringChain['$match'] = {};
    }
    filteringChain['$match']['$or'] = [{"rejected": true}, {"results.rejected": true}];
    chain.push(filteringChain);

    const sortingChain = MDBQueryAdaptor.compileSorting(state.sorting);
    // check if sorting query was compiled into something
    if (sortingChain) {
      chain.push(sortingChain);
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

  function hydrateData(docs, whitelist) {
    return _.each(docs, (doc) => {
      const matcher = _.pick(doc, 'project', 'variant', 'task', 'revision');
      doc._buildId = EvgUtil.generateBuildId({
        project: project,
        revision: doc.revision,
        buildVariant: doc.variant,
        createTime: doc.create_time,
      });
      doc.whitelisted = (whitelist ? _.findWhere(whitelist, matcher) !== undefined : false);
    });
  }

  // Required by loadData.
  let theMostRecentPromise;

  function setInitialGridFiltering(state) {
    _.each(state.filters, function (term, colName) {
      const col = $scope.getCol(colName);
      if (!col) return;  // Error! Associated col does not found
      col.filters = [{term: term}];
    });
  }

  // Sets `state` to grid filters
  function setInitialGridState(state) {
    setInitialGridFiltering(state);

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
    const promises = {
      docs: Stitch.use(STITCH_CONFIG.PERF).query((db) => {
        return db.db(STITCH_CONFIG.PERF.DB_PERF)
          .collection(STITCH_CONFIG.PERF.COLL_POINTS)
          .aggregate($scope.getAggChain(vm.state));
      }),
      whitelist: WhitelistService.getWhitelistQ(project,{})
    };
    theMostRecentPromise = promises.docs;
    $q.all(promises).then((results) => {
      // If there is more than one concurrent promise - we want the most recent.
      if (promises.docs !== theMostRecentPromise) {
        return;
      }

      promises.docs
        .then((docs) => vm.gridOptions.data = hydrateData(docs, results.whitelist), $log.error)
        .finally(() => $timeout(() => vm.isLoading = false));

      // The finally and timeout clear loading on the next event loop iteration so that the Loading indicator
      // disappears when the data load is complete. Otherwise there can be a gap.
    });
  }

  vm.reload = () => {

    // Push state changes to the grid api
    setInitialGridState(vm.state);

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

      api.core.on.renderingComplete($scope, (grid) => $log.debug('rendered' + grid));
      api.core.on.sortChanged($scope, (grid, cols) => {
        vm.state.sorting = _.map(cols, col => ({
          field: col.field,
          direction: col.sort.direction
        }));
        // NOTE do reload() here for server-side sorting
      });

      api.core.on.filterChanged($scope, _.debounce(() => vm.reload(), 200));

      api.selection.on.rowSelectionChanged(null, _.debounce(() => {
        handleRowSelectionChange(api);
        $scope.$apply();
      }));

      // This is required when user selects all items
      // (rowSelectionChanged doesn't work)
      api.selection.on.rowSelectionChangedBatch(null, () => handleRowSelectionChange(api));

      // Load initial set of data once `columns` are populated
      api.core.on.rowsRendered(null, _.once(() => vm.reload()));
    },

    columnDefs: [
      {
        name: 'Revision',
        field: 'revision',
        type: 'string',
        cellFilter: 'limitTo:7',
        cellTemplate: 'ui-grid-group-name',
        width: 200,
        sort: {
          direction: uiGridConstants.DESC,
          priority: 0,
        },
        sortingAlgorithm: $scope.sortRevision,
        grouping: {
          groupPriority: 0,
        },
        headerTooltip: function( col ) {
          return 'Header: ' + col.displayName;
        },
      },
      {
        name: 'Variant',
        field: 'variant',
        type: 'string',
        _link: row => '/build/' + row.entity._buildId,
        cellTemplate: 'ui-grid-link',
        grouping: {
          groupPriority: 1,
        },
      },
      {
        name: 'Task',
        field: 'task',
        type: 'string',
        _link: row => '/task/' + row.entity.task_id,
        cellTemplate: 'ui-grid-link',
        grouping: {
          groupPriority: 2,
        },
      },
      {
        name: 'Test',
        field: 'test',
        type: 'string',
        _link: row => '/task/' + row.entity.task_id + '##' + row.entity.test,
        cellTemplate: 'ui-grid-link',
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
      {
        name: 'Whitelisted',
        field: 'whitelisted',
        type: 'sting',
        enableFiltering: false,
        enableSorting: false,
        cellFilter: 'checkStatus',
      },
    ],
  };
}).filter('checkStatus', function() {
  return function(status) {
    if (status) {
      return  "✔" ;
    }
  }
}).factory('RejectState', function(FORMAT, $window, Settings) {
  class RejectState {
    constructor(project, model, scope, unit=2, value='weeks') {
      this.project = project;
      this.vm = model;
      this.scope = scope;
      this.sorting = [{
        field: 'order',
        direction: 'desc',
      }];
      this.lookBack = moment().subtract(unit , value);
    }

    get columns() {
      if(this.vm.gridApi) {
        return this.vm.gridApi.grid.columns;
      }
      return [];
    }
    get filters() {
      let filtering = _.reduce(this.columns, function (m, d) {
        const term = d.filters[0].term;
        if (term) m[d.field] = term;
        return m;
      }, {});
      if (_.isEmpty(filtering)) {
        filtering = this.default_filters;
      }
      Settings.perf.rejectProcessing.persistentFiltering = RejectState.omitTransientFilters(filtering);
      return filtering;
    };
    secondaryDefaultFiltering() {
      return {
        create_time: '>' + this.lookBack.format(FORMAT.ISO_DATE),
      };
    };
    getMandatoryDefaultFiltering(){
      return {
        project: '=' + this.project,
        variant: '^((?!wtdevelop).)*$',
        test: '^(canary|fio|NetworkB).*$',
      }
    };
    get default_filters() {
      // If we do not want low confidence outliers then set the type to detected.
      return _.extend(
        {},
        this.secondaryDefaultFiltering(),
        Settings.perf.rejectProcessing.persistentFiltering,
        this.getMandatoryDefaultFiltering()
      );
    };
    // Enhances filters state with some contextual meta data
    // This data is required by expression compiler
    getFilteringContext() {
      let defaultFilters = [];
      const filter = _.reduce(this.filters, (accum, filter_value, filter_key) => {
        if (!this.scope.getCol) {
          return accum;
        }
        const col = this.scope.getCol(filter_key);
        if (!col) return accum;  // Error! Associated col does not found

        return accum.concat({
          field: filter_key,
          term: filter_value,
          type: col.colDef.type || 'string',
        })
      }, defaultFilters);
      return filter.length ? filter : null;
    }
    static omitTransientFilters(filtering, ...fieldNames) {
      if( !fieldNames || fieldNames.length === 0) {
        fieldNames = ['type', 'create_time', 'project'];
      }
      return _.omit(filtering, fieldNames);
    };
  }
  return RejectState;
});