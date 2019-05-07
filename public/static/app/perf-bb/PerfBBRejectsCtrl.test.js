describe('PerfBBRejectsCtrlTest', () => {
  beforeEach(module('MCI'));

  let controller;
  let model;
  let scope;
  let format;
  let $filter;
  let RejectState;
  let Settings;

  // Fake the current date.
  const NOW = '2019-04-30';

  // Used for 1 week and 7 days test.
  const ONE_WEEKS_AGO = '2019-04-23';

  // Default 2 weeks.
  const TWO_WEEKS_AGO = '2019-04-16';

  let today = moment(NOW).toDate();
  jasmine.clock().mockDate(today);

  const project = 'test-project';

  beforeEach(inject(function ($rootScope, $controller, $injector, _$filter_,  _RejectState_) {
    scope = $rootScope;
    $filter = _$filter_;
    format = $injector.get('FORMAT');
    controller = $controller('PerfBBRejectsCtrl', {
      $scope: scope,
      $window: {
        project: project,
      },
      EvgUiGridUtil: $injector.get('EvgUiGridUtil'),
      EvgUtil: $injector.get('EvgUtil'),
      FORMAT: format,
      MDBQueryAdaptor: {
        compileFiltering: (data) => data,
        compileSorting: (data) => data,
      },
      uiGridConstants: $injector.get('uiGridConstants'),
      STITCH_CONFIG: $injector.get('STITCH_CONFIG'),
      Stitch: {
        use: () => {
          return {
            query: () => {
              return {
                then: () => {
                }
              }
            }
          }
        },
      }
    });
    RejectState = _RejectState_;

    // clear settings
    Settings = $injector.get('Settings');
    Settings.perf.rejectProcessing.persistentFiltering = {}
  }));

  describe('RejectState', () => {
    describe('ctor', () => {
      it('should handle non defaults fields', () => {

        const state = new RejectState('project', 'model', 'scope');
        expect(state.project).toEqual('project');
        expect(state.vm).toEqual('model');
        expect(state.scope).toEqual('scope');
        expect(state.sorting).toEqual([{
          field: 'order',
          direction: 'desc',
        }]);
        expect(state.lookBack.format('YYYY-MM-DD')).toEqual(TWO_WEEKS_AGO);
      });
      it('should handle allow units', () => {

        const state = new RejectState('project', 'model', 'scope', 1);
        expect(state.project).toEqual('project');
        expect(state.vm).toEqual('model');
        expect(state.scope).toEqual('scope');
        expect(state.sorting).toEqual([{
          field: 'order',
          direction: 'desc',
        }]);
        expect(state.lookBack.format('YYYY-MM-DD')).toEqual(ONE_WEEKS_AGO);
      });
      it('should handle allow units and value', () => {

        const state = new RejectState('project', 'model', 'scope', 7, 'days');
        expect(state.project).toEqual('project');
        expect(state.vm).toEqual('model');
        expect(state.scope).toEqual('scope');
        expect(state.sorting).toEqual([{
          field: 'order',
          direction: 'desc',
        }]);
        expect(state.lookBack.format('YYYY-MM-DD')).toEqual(ONE_WEEKS_AGO);
      });
    });
    describe('omitTransientFilters', () => {
      it('should include non-matching fields', () => {
        const expected = {find:'me'};
        const filtered = RejectState.omitTransientFilters(expected);
        expect(filtered).toEqual(expected);
      });
      it('should omit matching fields', () => {
        const expected = {find:'me'};
        const keys = ['type', 'create_time', 'project'];
        const filtered = RejectState.omitTransientFilters(_.extend(_.object(keys, keys),expected));
        expect(filtered).toEqual(expected);
      });

    });
    describe('columns', () => {
      it('should handle no grid', () => {
        const state = new RejectState('sys-perf', {}, scope);
        const columns = state.columns;
        expect(columns).toEqual([]);
      });
      it('should handle return grid data', () => {
        const expected = ['column1'];
        const state = new RejectState('sys-perf', {gridApi:{grid:{columns:expected}}}, scope);
        const columns = state.columns;
        expect(columns).toEqual(expected);
      });
    });
    describe('filters', () => {
      it('should handle return default filters', () => {
        const state = new RejectState('sys-perf', {}, scope);
        const filters = state.filters;
        expect(filters).toEqual({
          "create_time": ">" + TWO_WEEKS_AGO,
          "variant": "^((?!wtdevelop).)*$",
          "test": "^(canary|fio|NetworkB).*$",
          "project": "=sys-perf"
        });
        expect(Settings.perf.rejectProcessing.persistentFiltering).toEqual({
          "variant": "^((?!wtdevelop).)*$",
          "test": "^(canary|fio|NetworkB).*$",
        });
      });
      it('should handle 1 column', () => {
        const columns = [
          {field:'skipped', filters:[{}]},
          {field:'name', filters:[{term:'value'}]},
        ];
        const state = new RejectState('sys-perf', {gridApi:{grid:{columns:columns }}}, scope);
        const filters = state.filters;
        expect(filters).toEqual({
          name: "value"
        });
        expect(Settings.perf.rejectProcessing.persistentFiltering).toEqual({name: "value"});
      });
      it('should handle multiple columns', () => {
        const expected = {
          name: "value",
          name1: "value1"
        };
        const columns = [
          {field:'name', filters:[{term:'value'}]},
          {field:'skipped', filters:[{}]},
          {field:'name1', filters:[{term:'value1'}]},
        ];
        const state = new RejectState('sys-perf', {gridApi:{grid:{columns:columns }}}, scope);
        const filters = state.filters;
        expect(filters).toEqual(expected);
        expect(Settings.perf.rejectProcessing.persistentFiltering).toEqual(expected);
      });
    });
    describe('secondaryDefaultFiltering', () => {
      it('should handle return default filters', () => {
        const state = new RejectState('sys-perf', {}, scope);
        const filtering = state.secondaryDefaultFiltering();
        expect(filtering).toEqual({"create_time": ">" + TWO_WEEKS_AGO});
      });
    });
    describe('getMandatoryDefaultFiltering', () => {
      it('should include the project', () => {
        const state = new RejectState('sys-perf', model, scope);
        const filtering = state.getMandatoryDefaultFiltering();
        expect(filtering).toEqual({
          project: '=sys-perf',
          variant: '^((?!wtdevelop).)*$',
          test: '^(canary|fio|NetworkB).*$',
        });
      });
    });
    describe('default_filters', () => {
      it('should return default filters', () => {
        const state = new RejectState('sys-perf', {}, scope);
        const default_filters = state.default_filters;
        expect(default_filters).toEqual({
          "create_time": ">" + TWO_WEEKS_AGO,
          "variant": "^((?!wtdevelop).)*$",
          "test": "^(canary|fio|NetworkB).*$",
          "project": "=sys-perf"
        });
      });
      it('should merge persistentFiltering', () => {
        const state = new RejectState('sys-perf', {}, scope);
        Settings.perf.rejectProcessing.persistentFiltering = {
          "create_time": ">" + ONE_WEEKS_AGO,
          project: '=project',
          variant: 'variant',
          test: 'test',
        };
        const default_filters = state.default_filters;
        expect(default_filters).toEqual({
          "create_time": ">" + ONE_WEEKS_AGO,
          "variant": "^((?!wtdevelop).)*$",
          "test": "^(canary|fio|NetworkB).*$",
          "project": "=sys-perf"
        });
      });
    });
    describe('getFilteringContext', () => {
      it('should return default filters', () => {
        scope.getCol = (key) => {
          return {
            colDef: {
              type: key
            }
          };
        };

        const state = new RejectState('sys-perf', {}, scope);
        const filters = state.getFilteringContext();
        expect(filters).toEqual([
          {
            "field": "create_time",
            "term": ">2019-04-16",
            "type": "create_time"
          },
          {
            "field": "project",
            "term": "=sys-perf",
            "type": "project"
          },
          {
            "field": "variant",
            "term": "^((?!wtdevelop).)*$",
            "type": "variant"
          },
          {
            "field": "test",
            "term": "^(canary|fio|NetworkB).*$",
            "type": "test"
          }
        ]);
      });
      it('should return exclude invalid filters', () => {
        scope.getCol = (key) => {
          if (key !== 'variant') {
            return {
              colDef: {
                type: key
              }
            };
          }

          return null;
        };

        const state = new RejectState('sys-perf', {}, scope);
        const filters = state.getFilteringContext();
        expect(filters).toEqual([
          {
            "field": "create_time",
            "term": ">2019-04-16",
            "type": "create_time"
          },
          {
            "field": "project",
            "term": "=sys-perf",
            "type": "project"
          },
          {
            "field": "test",
            "term": "^(canary|fio|NetworkB).*$",
            "type": "test"
          }
        ]);
      });
    });
  });
  describe('checkStatus', () => {
    it('should handle null', () => {
      const checkStatus = $filter('checkStatus');
      expect(checkStatus()).toBeUndefined();
      expect(checkStatus('')).toBeUndefined();
      expect(checkStatus(false)).toBeUndefined();
      expect(checkStatus(null)).toBeUndefined();
      expect(checkStatus(undefined)).toBeUndefined();
    });
    it('should handle null', () => {
      const checkStatus = $filter('checkStatus');
      expect(checkStatus(true)).toEqual("✔" );
    });
  });

  describe('getAggChain', () => {
    let state;
    beforeEach(() => {
      state = new RejectState('project', 'model', scope);
    });

    it('should add to match ', () => {
      const filteringChain = {
        $match:{variant:{$regex:"^((?!wtdevelop).)*$",$options:"i"},
          test:{$regex:"^(canary|fio|NetworkB).*$",$options:"i"},
          project:{$eq:"sys-perf"},
          create_time:{$gt:"2019-04-16T00:00:00+00:00"}}};
      spyOn(state, 'getFilteringContext').and.returnValue(filteringChain);
      const chain = scope.getAggChain(state);
      expect(state.getFilteringContext).toHaveBeenCalled();

      expect(chain.length).toEqual(3);
      const filtering = chain.shift();
      expect(filtering).toEqual({
        "$match": {
          "variant": {
            "$regex": "^((?!wtdevelop).)*$",
            "$options": "i"
          },
          "test": {
            "$regex": "^(canary|fio|NetworkB).*$",
            "$options": "i"
          },
          "project": {
            "$eq": "sys-perf"
          },
          "create_time": {
            "$gt": "2019-04-16T00:00:00+00:00"
          },
          "$or": [
            {
              "rejected": true
            },
            {
              "results.rejected": true
            }
          ]
        }
      });
      const sortingChain = chain.shift();
      expect(sortingChain).toEqual([
        {
          "field": "order",
          "direction": "desc"
        }
      ]);
      const limitChain = chain.shift();
      expect(limitChain).toEqual({$limit: 2000});
    });
    it('should add $match if missing', () => {
      const filteringChain = {};
      spyOn(state, 'getFilteringContext').and.returnValue(filteringChain);
      const chain = scope.getAggChain(state);
      expect(state.getFilteringContext).toHaveBeenCalled();

      expect(chain.length).toEqual(3);
      const filtering = chain.shift();
      expect(filtering).toEqual({
        "$match": {
          "$or": [
            {
              "rejected": true
            },
            {
              "results.rejected": true
            }
          ]
        }
      });
      const sortingChain = chain.shift();
      expect(sortingChain).toEqual([
        {
          "field": "order",
          "direction": "desc"
        }
      ]);
      const limitChain = chain.shift();
      expect(limitChain).toEqual({$limit: 2000});
    });

  });

  describe('sortRevision', () => {
    beforeEach(() => {
      controller.gridApi = {
        core: {
          sortHandleNulls: () => null,
        }
      }
    });

    function createRow(value) {
      return {
        entity: {
          order: value
        }
      };
    }

    it('should return 0 if values are equal', () => {
      const value = scope.sortRevision(5, 5);
      expect(value).toEqual(0);
    });

    it('should return 0 if order values are equal', () => {
      const value = scope.sortRevision(3, 5, createRow(5), createRow(5));
      expect(value).toEqual(0);
    });

    it('should return negative if row a is less than row b', () => {
      const value = scope.sortRevision(3, 5, createRow(3), createRow(5));
      expect(value).toBeLessThan(0);
    });

    it('should return positive if row a is greater than row b', () => {
      const value = scope.sortRevision(3, 5, createRow(5), createRow(3));
      expect(value).toBeGreaterThan(0);
    });

    it('should use sortHandleNulls if available', () => {
      controller.gridApi.core.sortHandleNulls = () => 'nulls value';
      const value = scope.sortRevision(3, 5, createRow(5), createRow(3));
      expect(value).toEqual('nulls value');
    });

    it('should sort by revision if rows are not available', () => {
      const value = scope.sortRevision(3, 5, createRow(5));
      expect(value).toBeLessThan(0);
    });
  });
});
