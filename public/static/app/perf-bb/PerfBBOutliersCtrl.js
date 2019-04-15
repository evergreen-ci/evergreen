mciModule.controller('PerfBBOutliersCtrl', function(
    $scope, $window, EvgUiGridUtil, EvgUtil, FORMAT, MDBQueryAdaptor, uiGridConstants,
    STITCH_CONFIG, Stitch,
) {
    // Perf Failures View-Model.
    const vm = this;
    const project = window.project;
    const LIMIT = 500;

    const OUTLIERS_TYPE = {
        DETECTED: 'detected',
        SUSPICIOUS: 'suspicious',
    };

    vm.mode = {
        options: [{
            id: 'detected',
            name: 'Detected',
        }, {
            id: 'suspicious',
            name: 'Suspicious',
        }, {
            id: 'marked',
            name: 'Marked',
        }, {
            id: 'muted',
            name: 'Muted',
        }],
        value: 'detected',
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

    const modesRequiringFilters = new Set([
        OUTLIERS_TYPE.DETECTED, OUTLIERS_TYPE.SUSPICIOUS
    ]);

    // Could not be overridden by persistent user settings.
    const mandatoryDefaultFiltering = (state) => {
        return {
            create_time: '>' + state.lookBack.format(FORMAT.ISO_DATE),
            project: '=' + $window.project,
        };
    };

    $scope.getDefaultFiltering = () => {
        let typeFilter = {};
        if (modesRequiringFilters.has(vm.mode.value)) {
            typeFilter = {
                type: '=' + vm.mode.value,
            };
        }
        return _.extend(
            {},
            mandatoryDefaultFiltering(vm.state),
            typeFilter
        );
    };

    vm.state = {
        filtering: $scope.getDefaultFiltering,
        lookBack: moment().subtract(2, 'weeks'),
        mode: vm.mode.value,
    };

    const modeToCollMap = {
        detected: STITCH_CONFIG.PERF.COLL_OUTLIERS,
        suspicious: STITCH_CONFIG.PERF.COLL_OUTLIERS,
        muted: STITCH_CONFIG.PERF.COLL_MUTE_OUTLIERS,
        marked: STITCH_CONFIG.PERF.COLL_MARKED_OUTLIERS,
    };

    // Enhances filtering state with some contextual meta data
    // This data is required by expression compiler
    function getFilteringContext(state) {
        return _.reduce(state.filtering(), (accum, filter_value, filter_key) => {
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
        }, []);
    }

    // Creates aggregation expression, which could be used by Stitch
    // for given `state`
    $scope.getAggChain = (state) => {
        let chain = [];

        // Check if the state has filtering
        if (state.filtering) {
            const filteringChain = MDBQueryAdaptor.compileFiltering(
                // filtering context enhances state data with important meta data
                getFilteringContext(state)
            );
            // check if filtering query was compiled into something
            if (filteringChain) {
                chain.push(filteringChain);
            }
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
                }).finally(() => {
                    vm.isLoading = false;
            });
        });
    }

    vm.modeChanged = () => {
        vm.state.mode = vm.mode.value;

        // Push state changes to the grid api
        // setInitialGridState(vm.gridApi, state);

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

            api.selection.on.rowSelectionChanged(null, _.debounce(() => {
                handleRowSelectionChange(api);
                $scope.$apply();
            }));

            // This is required when user selects all items
            // (rowSelecionChanged doesn't work)
            api.selection.on.rowSelectionChangedBatch(null, () => {
                handleRowSelectionChange(api);
            });
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
        ],
    };

    loadData();
});
