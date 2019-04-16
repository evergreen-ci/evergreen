describe('PerfBBOutliersCtrlTest', () => {
  beforeEach(module('MCI'));

  let controller;
  let scope;
  let format;

  const project = 'test-project';

  beforeEach(inject(function ($rootScope, $controller, $injector) {
    // $scope, $window, EvgUiGridUtil, EvgUtil, FORMAT, MDBQueryAdaptor, uiGridConstants,
    //     STITCH_CONFIG, Stitch,
    scope = $rootScope;
    format = $injector.get('FORMAT');
    controller = $controller('PerfBBOutliersCtrl', {
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
  }));

  describe('getDefaultFiltering', () => {
    it('should filter by project, date, and type for detected', () => {
      controller.mode.value = 'detected';
      const filter = scope.getDefaultFiltering();

      expect(filter.project).toEqual('=' + project);
      const expectedDate = controller.state.lookBack.format(format.ISO_DATE);
      expect(filter.create_time).toEqual('>' + expectedDate);
      expect(filter.type).toEqual('=detected');
    });

    it('should filter by project, date, and type for suspicious', () => {
      controller.mode.value = 'suspicious';
      const filter = scope.getDefaultFiltering();

      expect(filter.project).toEqual('=' + project);
      const expectedDate = controller.state.lookBack.format(format.ISO_DATE);
      expect(filter.create_time).toEqual('>' + expectedDate);
      expect(filter.type).toEqual('=suspicious');
    });

    it('should filter by project and date for marked', () => {
      controller.mode.value = 'marked';
      const filter = scope.getDefaultFiltering();

      expect(filter.project).toEqual('=' + project);
      const expectedDate = controller.state.lookBack.format(format.ISO_DATE);
      expect(filter.create_time).toEqual('>' + expectedDate);
      expect(filter).not.toContain('type');
    });

    it('should filter by project and date for muted', () => {
      controller.mode.value = 'muted';
      const filter = scope.getDefaultFiltering();

      expect(filter.project).toEqual('=' + project);
      const expectedDate = controller.state.lookBack.format(format.ISO_DATE);
      expect(filter.create_time).toEqual('>' + expectedDate);
      expect(filter).not.toContain('type');
    });
  });

  describe('getAggChain', () => {
    beforeEach(() => {
      scope.getCol = (key) => {
        return {
          colDef: {
            type: key
          }
        };
      };

    });

    it('should only limit if not filtering or sorting', () => {
      const chain = scope.getAggChain({});
      expect(chain.length).toEqual(1);
      expect(chain[0].$limit).toEqual(2000);
    });

    it('should include filter if specified', () => {
      const chain = scope.getAggChain({
        filtering: {
          filter1: 'value1',
          filter2: 'value2',
        }
      });
      expect(chain.length).toEqual(2);
      expect(chain[0]).toContain({field: 'filter1', term: 'value1', type: 'filter1'});
      expect(chain[0]).toContain({field: 'filter2', term: 'value2', type: 'filter2'});
      expect(chain[1].$limit).toEqual(2000);
    });

    it('should include sort if specified', () => {
      const chain = scope.getAggChain({
        sorting: 'sort 1',
      });
      expect(chain.length).toEqual(2);
      expect(chain[0]).toEqual('sort 1');
      expect(chain[1].$limit).toEqual(500);
    });

    it('should include sort and filter if specified', () => {
      const chain = scope.getAggChain({
        filtering: () => {
          return {
            filter1: 'value1',
            filter2: 'value2',
          }
        },
        sorting: 'sort 1',
      });
      expect(chain.length).toEqual(3);
      expect(chain[0]).toContain({field: 'filter1', term: 'value1', type: 'filter1'});
      expect(chain[0]).toContain({field: 'filter2', term: 'value2', type: 'filter2'});
      expect(chain[1]).toEqual('sort 1');
      expect(chain[2].$limit).toEqual(500);
    });

    it('should using string for filter if not type', () => {
      scope.getCol = () => {
        return {colDef: {}};
      };
      const chain = scope.getAggChain({
        filtering: () => {
          return {
            filter1: 'value1',
            filter2: 'value2',
          }
        }
      });
      expect(chain.length).toEqual(2);
      expect(chain[0]).toContain({field: 'filter1', term: 'value1', type: 'string'});
      expect(chain[0]).toContain({field: 'filter2', term: 'value2', type: 'string'});
      expect(chain[1].$limit).toEqual(500);
    });

    it('should return an empty filter if getCol is not defined', () => {
      scope.getCol = undefined;
      const chain = scope.getAggChain({
        filtering: () => {
          return {
            filter1: 'value1',
            filter2: 'value2',
          }
        }
      });
      expect(chain.length).toEqual(2);
      expect(chain[0]).toEqual([]);
      expect(chain[1].$limit).toEqual(500);
    });

    it('should not use a filter if getCol cannot find the column', () => {
      scope.getCol = (key) => {
        if (key !== 'filter1') {
          return {
            colDef: {
              type: key
            }
          };
        }

        return null;
      };
      const chain = scope.getAggChain({
        filtering: () => {
          return {
            filter1: 'value1',
            filter2: 'value2',
          }
        }
      });
      expect(chain.length).toEqual(2);
      expect(chain[0]).not.toContain({field: 'filter1', term: 'value1', type: 'filter1'});
      expect(chain[0]).toContain({field: 'filter2', term: 'value2', type: 'filter2'});
      expect(chain[1].$limit).toEqual(500);
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
