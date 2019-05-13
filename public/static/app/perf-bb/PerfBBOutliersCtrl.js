mciModule.controller('PerfBBOutliersCtrl', function (
  $scope, $window, uiGridConstants, OutlierState,
  STITCH_CONFIG, Stitch, Settings, $timeout, $log, OutliersDataService, Lock, $q, MuteHandler, Operations
) {
  // Perf Outliers View-Model.
  const vm = this;
  const project = $window.project;

  // Default values for sorting / searching etc.
  const LIMIT = 2000;
  const mandatory = {
    project: '=' + project,
    variant: '^((?!wtdevelop).)*$',
    test: '^(canary|fio|NetworkB).*$',
  };
  const transient = ['type', 'create_time', 'project'];
  const sorting = [{
    field: 'order',
    direction: 'desc',
  }];

  // Outlier State encapsulates the UIGrid handling data loading and processing the data for the grid.
  const state = new OutlierState(project, vm, mandatory, transient, sorting, Settings.perf.outlierProcessing, LIMIT);

  // Mute handler encapsulates the logic of adding / removing mutes.
  const mute_handler = new MuteHandler(state, new Lock());

  // Required by loadData.
  const promises = new Operations();

  vm.promises = promises;

  // Set up the view model.
  vm.state = state;

  vm.checkboxModel = {
    lowConfidence: false,
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

  vm.actions = [
    {
      title: 'Mute',
      action: () => mute_handler.muteOutliers(_.bind(state.onMute, state)),
      visible: () => true,
      disabled: () => mute_handler.muteDisabled(),
    },
    {
      title: 'Unmute',
      action: () => mute_handler.unmuteOutliers(_.bind(state.onUnmute, state)),
      visible: () => true,
      disabled: () => mute_handler.unmuteDisabled(),
    },
    {
      title: 'Mark',
      action: console.log,
      visible: () => vm.state.mode !== 'marked',
      disabled: () => true,
    },
    {
      title: 'Unmark',
      action: console.log,
      visible: () => vm.state.mode === 'marked',
      disabled: () => true,
    },
  ];

  // Load data:
  //    starts the loading indicator
  //    get the remote state from atlas
  //    renders the data (if it is the latest data load event)
  //    clears the loading indicator.
  vm.reload = () => {
    vm.isLoading = true;
    vm.gridOptions.data = [];

    // get a Unique identifier for the this reload call. If the this is not the current id when we get the
    // results, then we ignore it and await the most recent results. The finally call also ignores out of date
    // calls.
    const operation = vm.promises.next;

    // If there is more than one concurrent promise - we want the most recent.
    // The finally / timeout clear loading on the next event loop iteration so that the Loading indicator
    // disappears when the data load is complete. Otherwise there can be a gap.
    vm.state.loadData(operation)
      .then(results => vm.promises.isCurrent(results.operation) && vm.state.hydrateData(results))
      .catch($log.error)
      .finally(() => promises.isCurrent(operation) && $timeout(() => vm.isLoading = false));
  };

  vm.lowConfidenceChanged = () => vm.reload();
  vm.modeChanged = () => vm.reload();

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    enableRowSelection: true,
    enableSelectAll: true,
    selectionRowHeaderWidth: 35,
    useExternalFiltering: true,
    useExternalSorting: false,

    onRegisterApi: (api) => {
      api.core.on.sortChanged($scope, vm.state.onSortChanged);
      api.core.on.filterChanged($scope, _.debounce(vm.reload, 500));
      api.core.on.rowsRendered(null, _.once(_.bind(vm.state.onRowsRendered, vm.state), api));
    },

    columnDefs: [
      {
        name: 'Project',
        field: 'project',
        type: 'string',
        visible: false,
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
        sortingAlgorithm: _.bind(vm.state.sortRevision, vm.state),
        cellTemplate: 'ui-grid-group-name',
        grouping: {
          groupPriority: 0,
        },
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
        name: 'Thread Level',
        field: 'thread_level',
        type: 'string',
      },
      {
        name: 'Confidence',
        field: 'type',
        type: 'string',
        enableFiltering: false,
        cellFilter: 'outlierTypeToConfidence',
      },
      {
        name: 'Muted',
        field: 'muted',
        type: 'sting',
        enableFiltering: false,
        enableSorting: false,
        cellFilter: 'checkStatus',
      },
      {
        name: 'Create Time',
        field: 'create_time',
        type: 'date',
      },
    ],
  };
}).factory('MuteHandler', function(MuteDataService, confirmDialogFactory, $log, $q) {
  class MuteHandler {
    constructor(state, lock) {
      this.state = state;
      this.lock = lock;
      this.confirmMuteAction = confirmDialogFactory(function (outliers) {
        return `Add mute for ${outliers.length} Outliers?`
      });
      this.confirmUnmuteAction = confirmDialogFactory(function (outliers) {
        return `Remove mute for ${outliers.length} Outliers?`
      });
    }

    muteSelected() {
      return _.any(this.state.selection, {muted: true});
    }

    unmuteSelected() {
      return _.any(this.state.selection, {muted: false});
    }

    muteDisabled() {
      return this.lock.locked || this.state.selection.length === 0 || !this.unmuteSelected();
    }

    unmuteDisabled() {
      return this.lock.locked || this.state.selection.length === 0 || !this.muteSelected();
    }

    muteOutliers(success, error) {
      const items = this.state.selection;
      this.lock.lock();
      this.confirmMuteAction(items)
        .then(() => {
          const promises = _.map(items, function (item) {
            const mute = MuteHandler.getMuteIdentifier(item);
            if (item.muted) {
              return mute;
            } else {
              return MuteDataService.addMute(mute);
            }
          });
          return $q.all(promises)
        })
        .then(success, error)
        .finally(() => this.lock.unlock())
        .catch($log.debug);
    };

    unmuteOutliers(success, error) {
      const items = this.state.selection;
      this.lock.lock();
      this.confirmUnmuteAction(items)
        .then(() => {
          const promises = _.map(items, function (item) {
            const mute = MuteHandler.getMuteIdentifier(item);
            if (item.muted) {
              return MuteDataService.unMute(mute);
            } else {
              return mute;
            }
          });
          return $q.all(promises)
        })
        .then(success, error)
        .finally(() => this.lock.unlock())
        .catch($log.debug);
    };

    static getMuteIdentifier(mute) {
      return _.pick(mute, "project", "revision", "task", "test", "thread_level", "variant", "create_time", "end", "last_updated_at", "order", "task_id", "version_id");
    }

  }

  return MuteHandler;
}).factory('OutlierState', function(FORMAT, MDBQueryAdaptor, EvgUtil, EvgUiGridUtil, uiGridConstants, OutliersDataService, MuteDataService, $q) {
  class OutlierState {
    // Create a new state instance to encapsulate filtering, sorting and generating queries.
    //
    // :param str project: The project name, e.g. 'sys-perf'.
    // :param object model: The view model instance.
    // :param object scope: The $scope instance.
    // :param dict mandatory: The mandatory filter fields / values.
    // :param list transient: The transient filter fields. These fields are never persisted to the settings.
    // :param list(dict) sorting: The default fields to sort on.
    // :param object settings: A reference to the persistent settings instance.
    // :param int units: The look back value. Defaults to 2.
    // :param str units: The look back units. Defaults to 'weeks'.
    constructor(project, model, mandatory, transient, sorting, settings, limit, value = 2, units = 'weeks') {
      this.project = project;
      this.vm = model;
      this.mandatory = mandatory;
      this.transient = transient;
      this._sorting = sorting;
      this.settings = settings;
      this.limit = limit;

      this.lookBack = moment().subtract(value, units);
      this.getCol = null;
      this.api = null;
    }

    // Handle a sort change event.
    // :param object grid: Not used.
    // :param object cols: The column definitions.
    onSortChanged(grid, cols) {
      this._sorting = _.map(cols, function (col) {
        return {
          field: col.field,
          direction: col.sort.direction
        };
      });
    }

    // Handle a rowsRendered event. This is only expected to be called once.
    // :param object api: The grid api reference.
    // :param object cols: The column definitions.
    onRowsRendered(api) {
      this.api = api;
      this.getCol = EvgUiGridUtil.getColAccessor(api);

      // Set initial uiGrid filtering.
      _.each(this.defaultFilters, (term, colName) => {
        const col = this.getCol(colName);
        if (!col) return;  // Error! Associated col does not found
        col.filters = [{term: term}];
      });

      // Set initial uiGrid sorting.
      _.each(this.sorting, (sortingItem) => {
        const col = this.getCol(sortingItem.field);
        if (!col) return; // Error! Associated col does not found
        col.sort.direction = sortingItem.direction;
      });

      // Raise col visibility change event
      this.api.core.notifyDataChange(uiGridConstants.dataChange.COLUMN);

      this.vm.reload()
    };

    onMute(mutes) {
      if (!mutes || mutes.length === 0) return;
      _.each(mutes, mute => _.chain(this.vm.gridOptions.data).where(mute).each(doc => doc.muted = true).value());

      this.refreshGridData();
    }

    onUnmute(mutes) {
      if (!mutes || mutes.length === 0) return;
      _.each(mutes, mute => _.chain(this.vm.gridOptions.data).where(mute).each(doc => doc.muted = false).value());

      this.refreshGridData();
    }

    refreshGridData() {
      if (this.api) {
        this.api.selection.clearSelectedRows();
      }
    }

    get selection() {
      if (this.api) {
        return this.api.selection.getSelectedRows();
      }
      return [];
    }

    get sorting() {
      return this._sorting;
    }

    get mode() {
      return this.vm.mode.value;
    }

    get lowConfidence() {
      return this.vm.checkboxModel.lowConfidence;
    }

    get columns() {
      if (this.api) {
        return this.api.grid.columns;
      }
      return [];
    }

    get filters() {
      let filtering = _.reduce(this.columns, function (m, d) {
        const term = d.filters[0].term;
        if (term) m[d.field] = term;
        return m;
      }, {});
      this.settings.persistentFiltering = this.omitTransientFilters(filtering);
      return filtering;
    };

    get secondaryDefaultFiltering() {
      return {
        create_time: '>' + this.lookBack.format(FORMAT.ISO_DATE),
      };
    };

    get mandatoryDefaultFiltering() {
      return this.mandatory;
    }

    get defaultFilters() {
      // If we do not want low confidence outliers then set the type to detected.
      return _.extend(
        {},
        this.secondaryDefaultFiltering,
        this.settings.persistentFiltering,
        this.mandatoryDefaultFiltering
      );
    }

    // Enhances filters state with some contextual meta data
    // This data is required by expression compiler
    getFilteringContext() {
      let defaultFilters = [];
      if (!this.lowConfidence) {
        defaultFilters = [{
          field: 'type',
          type: 'string',
          term: '=detected'
        }];
      }

      const filter = _.reduce(this.filters, (accum, filter_value, filter_key) => {
        if (!this.getCol) {
          return accum;
        }
        const col = this.getCol(filter_key);
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
    getAggChain() {
      let chain = [];

      // Check if the state has filtering
      const filteringChain = MDBQueryAdaptor.compileFiltering(
        // filtering context enhances state data with important meta data
        this.getFilteringContext()
      );
      // check if filtering query was compiled into something
      if (filteringChain) {
        chain.push(filteringChain);
      }

      if (this.sorting) {
        const sortingChain = MDBQueryAdaptor.compileSorting(this.sorting);
        // check if sorting query was compiled into something
        if (sortingChain) {
          chain.push(sortingChain);
        }
      }

      chain.push({$limit: this.limit});
      return chain;
    }

    sortRevision(a, b, rowA, rowB) {
      // Sort revision by order instead of revision id.
      if (this.api === null) {
        return null;
      }
      const nulls = this.api.core.sortHandleNulls(a, b);
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

    loadData(operation) {
      const pipeline = this.getAggChain();
      return $q.all({
        outliers: OutliersDataService.aggregateQ(pipeline),
        mutes: MuteDataService.queryQ({project: this.project}),
        operation: operation
      });
    }

    hydrateData(results) {
      const {outliers, mutes} = results;
      this.vm.gridOptions.data = _.each(outliers, (doc) => {
        doc._buildId = EvgUtil.generateBuildId({
          project: this.project,
          revision: doc.revision,
          buildVariant: doc.variant,
          createTime: doc.create_time,
        });
        const matcher = _.pick(doc, 'revision', 'project', 'variant', 'task', 'test', 'thread_level');
        const mute = _.findWhere(mutes, matcher);
        if (mute) {
          doc.muted = mute.enabled;
        }
      });
      return this.vm.gridOptions.data;
    }

    omitTransientFilters(filtering) {
      return _.omit(filtering, this.transient);
    }
  }

  return OutlierState;
}).factory('Operations', function() {
  class Operation {
    constructor() {
      this.currentOp = 0;
    }

    get next() {
      this.currentOp += 1;
      return this.currentOp;
    };

    get current() {
      return this.currentOp;
    };

    isCurrent(operation) {
      return operation === this.currentOp;
    };
  }

  return Operation;
}).factory('confirmDialogFactory', function($mdDialog) {
  return function(text_or_function) {
    const formatter = _.isFunction(text_or_function) ? text_or_function : function() { return text_or_function};
    return function(items) {
      return $mdDialog.show(
        $mdDialog.confirm()
          .ok('Ok')
          .cancel('Cancel')
          .title('Confirm')
          .textContent(formatter(items))
      );
    }
  }
}).filter('outlierTypeToConfidence', function() {
  return function(type) {
    if (type === 'detected') {
      return  "high" ;
    } else if (type === 'suspicious') {
      return "low";
    }
  }
});