const PROJECT = 'test-project';
// Fake the current date.
const NOW = '2019-04-30';

// Used for 1 week and 7 days test.
const ONE_WEEKS_AGO = '2019-04-23';

// Default 2 weeks.
const TWO_WEEKS_AGO = '2019-04-16';

const TODAY = moment(NOW).toDate();

describe('PerfBBOutliersCtrlTest', () => {
  beforeEach(module('MCI'));

  let controller;
  let $log;
  let $timeout;
  let scope;
  let format;

  let OutliersDataService;

  let OutlierState;
  let state;

  let MuteHandler;
  let mute_handler;

  let Operations;

  let Settings;
  let Lock;
  let vm;

  beforeEach(inject(function ($rootScope, $controller, $injector) {
    scope = $rootScope;
    format = $injector.get('FORMAT');
    OutliersDataService = $injector.get('OutliersDataService');
    $log = $injector.get('$log');
    $timeout = function(fn) {
      return fn();
    };
    Operations = $injector.get('Operations');
    Settings = $injector.get('Settings');
    Lock = $injector.get('Lock');
    $window = $injector.get('$window');
    $window.project = PROJECT;

    state = jasmine.createSpyObj('OutlierState', ['sortRevision']);
    OutlierState = jasmine.createSpy('OutlierState').and.returnValue(state);

    mute_handler = jasmine.createSpyObj('muteHandler', ['muteOutliers', 'unmuteOutliers', 'muteDisabled', 'unmuteDisabled']);
    MuteHandler = jasmine.createSpy('MuteHandler').and.returnValue(mute_handler);

    controller = $controller('PerfBBOutliersCtrl', {
      $scope: scope,
      OutlierState: OutlierState,
      MuteHandler: MuteHandler,
      $window: $window,
      $timeout: $timeout,
      Settings: Settings,
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
    vm = controller;
  }));

  describe('controller', () => {
    it('should setup', () => {
      const mandatory = {
        project: '=' + PROJECT,
        variant: '^((?!wtdevelop).)*$',
        test: '^(canary|fio|NetworkB).*$',
      };
      const transient = ['type', 'create_time', 'project'];
      const sorting = [{
        field: 'order',
        direction: 'desc',
      }];

      expect(OutlierState).toHaveBeenCalledWith(PROJECT, vm, mandatory, transient, sorting, Settings.perf.outlierProcessing, 2000);
      expect(MuteHandler).toHaveBeenCalledWith(state, jasmine.any(Lock));

      expect(vm.state).toBe(state);
      expect(vm.checkboxModel).toEqual({lowConfidence : false});
      expect(vm.mode).toEqual({
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
      });
      expect(_.chain(vm.actions).pluck('title').value()).toEqual(['Mute', 'Unmute', 'Mark' , 'Unmark']);
      expect(vm.reload).toEqual(jasmine.any(Function));
      expect(vm.lowConfidenceChanged).toEqual(jasmine.any(Function));
      expect(vm.modeChanged).toEqual(jasmine.any(Function));

      expect(_.omit(vm.gridOptions, ['onRegisterApi', 'columnDefs'])).toEqual({
        enableFiltering: true,
        enableGridMenu: true,
        enableRowSelection: true,
        enableSelectAll: true,
        selectionRowHeaderWidth: 35,
        useExternalFiltering: true,
        useExternalSorting: false});

      expect(vm.gridOptions.onRegisterApi).toEqual(jasmine.any(Function));
      expect(_.chain(vm.gridOptions.columnDefs).pluck('name').value()).toEqual(['Project', 'Revision', 'Variant' , 'Task', 'Test', 'Thread Level', 'Confidence', 'Muted', 'Create Time']);
    });

  });

  describe('actions', () => {
    let boundMethod = {};

    beforeEach(() => spyOn(_, 'bind').and.returnValue(boundMethod));

    describe('Mute', () => {
      let action;
      beforeEach(() => action = vm.actions[0]);

      it('should handle action', () => expect(action.title).toBe('Mute'));

      it('should handle action', () => {
        action.action();
        expect(mute_handler.muteOutliers).toHaveBeenCalledWith(boundMethod);
        expect(_.bind).toHaveBeenCalledWith(state.onMute, state);
      });

      it('should handle visible', () => expect(action.visible()).toBe(true));

      it('should handle disabled', () => {
        const muteDisabled = {};
        mute_handler.muteDisabled.and.returnValue(muteDisabled);

        expect(action.disabled()).toBe(muteDisabled);

        expect(mute_handler.muteDisabled).toHaveBeenCalled();
      });

    });

    describe('Unmute', () => {
      let action;
      beforeEach(() => action = vm.actions[1]);

      it('should handle action', () => expect(action.title).toBe('Unmute'));

      it('should handle action', () => {
        action.action();
        expect(mute_handler.unmuteOutliers).toHaveBeenCalledWith(boundMethod);
        expect(_.bind).toHaveBeenCalledWith(state.onUnmute, state);
      });

      it('should handle visible', () => expect(action.visible()).toBe(true));

      it('should handle disabled', () => {
        const unmuteDisabled = {};
        mute_handler.unmuteDisabled.and.returnValue(unmuteDisabled);

        expect(action.disabled()).toBe(unmuteDisabled);

        expect(mute_handler.unmuteDisabled).toHaveBeenCalled();
      });

    });
  });

  describe('reload', () => {
    let next_operation = {};
    let promise;
    beforeEach(() => {
      promise = {
        then: () => promise,
        catch: () => promise,
        finally: () => promise,
      };
      vm.state = {
        loadData: () => promise,
        hydrateData: () => promise,
      };
    });

    it('should call load data with next operation', () => {

      spyOn(vm.state, 'loadData').and.callThrough();
      spyOnProperty(vm.promises, 'next', 'get').and.returnValue(next_operation);
      vm.reload();
      expect(vm.state.loadData).toHaveBeenCalledWith(next_operation);
    });

    it('should hydrate data if current', () => {
      const results = {operation: next_operation};
      promise.then = (success) => {
        success(results);
        return promise;
      };
      spyOn(vm.state, 'loadData').and.callThrough();
      spyOn(vm.state, 'hydrateData').and.callThrough();
      spyOn(vm.promises, 'isCurrent').and.returnValue(true);
      spyOnProperty(vm.promises, 'next', 'get').and.returnValue(next_operation);
      vm.reload();

      expect(vm.promises.isCurrent).toHaveBeenCalledWith(next_operation);
      expect(vm.state.hydrateData).toHaveBeenCalledWith(results);

    });

    it('should not hydrate data when not current', () => {
      const results = {operation: next_operation};
      promise.then = (success) => {
        success(results);
        return promise;
      };
      spyOn(vm.state, 'loadData').and.callThrough();
      spyOn(vm.state, 'hydrateData').and.callThrough();
      spyOn(vm.promises, 'isCurrent').and.returnValue(false);
      spyOnProperty(vm.promises, 'next', 'get').and.returnValue(next_operation);
      vm.reload();

      expect(vm.promises.isCurrent).toHaveBeenCalledWith(next_operation);
      expect(vm.state.hydrateData).not.toHaveBeenCalledWith(results);

    });

    it('should catch error', () => {
      spyOn(promise, 'catch').and.callThrough();
      vm.reload();

      expect(promise.catch).toHaveBeenCalledWith($log.error);
    });

    it('should cancel loading when current', () => {
      const next_operation = {};
      spyOn(vm.promises, 'isCurrent').and.returnValue(true);
      spyOnProperty(vm.promises, 'next', 'get').and.returnValue(next_operation);
      promise.finally = (success) => {
        success();
        return promise;
      };
      vm.reload();

      expect(vm.promises.isCurrent).toHaveBeenCalledWith(next_operation);
      expect(vm.isLoading).toBe(false);
    });

    it('should skip cancel loading when not current', () => {
      const next_operation = {};
      spyOn(vm.promises, 'isCurrent').and.returnValue(false);
      spyOnProperty(vm.promises, 'next', 'get').and.returnValue(next_operation);
      promise.finally = (success) => {
        success();
        return promise;
      };
      vm.reload();

      expect(vm.promises.isCurrent).toHaveBeenCalledWith(next_operation);
      expect(vm.isLoading).toBe(true);
    });

  });

  describe('lowConfidenceChanged', () => {

    it('should call reload', () => {

      spyOn(vm, 'reload').and.returnValue(undefined);
      vm.lowConfidenceChanged();
      expect(vm.reload).toHaveBeenCalled();
    });

  });

  describe('modeChanged', () => {

    it('should call reload', () => {

      spyOn(vm, 'reload').and.returnValue(undefined);
      vm.modeChanged();
      expect(vm.reload).toHaveBeenCalled();
    });

  });

  describe('onRegisterApi', () => {

    it('should register callbacks', () => {

      const api = {
        core:{
          on:{
            sortChanged: () => {},
            filterChanged: () => {},
            rowsRendered: () => {},
          }
        }
      };

      spyOn(api.core.on, 'sortChanged').and.callThrough();
      spyOn(api.core.on, 'filterChanged').and.callThrough();
      spyOn(api.core.on, 'rowsRendered').and.callThrough();

      const debounce = () => {};
      const once = () => {};
      const bind = () => {};
      spyOn(_, 'debounce').and.returnValue(debounce);
      spyOn(_, 'once').and.returnValue(once);
      spyOn(_, 'bind').and.returnValue(bind);

      vm.gridOptions.onRegisterApi(api);

      expect(api.core.on.sortChanged).toHaveBeenCalledWith(scope, vm.state.onSortChanged);
      expect(api.core.on.filterChanged).toHaveBeenCalledWith(scope, debounce);
      expect(api.core.on.rowsRendered).toHaveBeenCalledWith(null, once);

      expect(_.debounce).toHaveBeenCalledWith(vm.reload, 500);
      expect(_.bind).toHaveBeenCalledWith(vm.state.onRowsRendered, vm.state);
      expect(_.once).toHaveBeenCalledWith(bind, api);
    });

  });

});

describe('PerfBBOutliersFactoriesTest', () => {

  beforeEach(module('MCI'));

  jasmine.clock().mockDate(TODAY);

  describe('MuteHandler', () => {

    const state = {};
    const lock = {locked: true , lock: () => {}, unlock: () => {}};

    let MuteHandler;
    let MuteDataService;
    let handler;
    let confirmDialogFactory;

    beforeEach(() => {
      inject($injector => {
        MuteHandler = $injector.get('MuteHandler');
        MuteDataService = $injector.get('MuteDataService');
        confirmDialogFactory = $injector.get('confirmDialogFactory');
      });
      handler = new MuteHandler(state, lock);
    });

    it('should create a correct instance', () => {
      expect(handler.state).toBe(state);
      expect(handler.lock).toBe(lock);
      expect(handler.confirmMuteAction).toEqual(jasmine.any(Function));
      expect(handler.confirmUnmuteAction).toEqual(jasmine.any(Function));
    });

    describe('muteSelected', () => {

      it('should return false if no selection', () => {
        handler.state = {selection:[]};
        expect(handler.muteSelected()).toBe(false);
      });

      it('should return true if any selection is muted', () => {
        handler.state = {selection:[{muted: true}, {muted: false}]};
        expect(handler.muteSelected()).toBe(true);
      });

      it('should return false if no selection is muted', () => {
        handler.state = {selection:[{muted: false}, {muted: false}]};
        expect(handler.muteSelected()).toBe(false);
      });

    });

    describe('unmuteSelected', () => {

      it('should return false if no selection', () => {
        handler.state = {selection:[]};
        expect(handler.unmuteSelected()).toBe(false);
      });

      it('should return true if any selection is muted', () => {
        handler.state = {selection:[{muted: true}, {muted: false}]};
        expect(handler.unmuteSelected()).toBe(true);
      });

      it('should return false if no selection is muted', () => {
        handler.state = {selection:[{muted: true}, {muted: true}]};
        expect(handler.unmuteSelected()).toBe(false);
      });

    });

    describe('muteDisabled', () => {

      it('should return true if locked', () => {
        handler.lock.locked = true;
        handler.state ={selection: [1]};

        spyOn(handler, 'unmuteSelected').and.callThrough();
        spyOn(handler, 'muteSelected').and.callThrough();

        expect(handler.muteDisabled()).toBe(true);

        expect(handler.unmuteSelected).not.toHaveBeenCalled();
        expect(handler.muteSelected).not.toHaveBeenCalled();
      });

      it('should return true if no selection', () => {
        handler.lock.locked = false;
        handler.state ={selection: []};

        spyOn(handler, 'unmuteSelected').and.callThrough();
        spyOn(handler, 'muteSelected').and.callThrough();

        expect(handler.muteDisabled()).toBe(true);

        expect(handler.unmuteSelected).not.toHaveBeenCalled();
        expect(handler.muteSelected).not.toHaveBeenCalled();
      });

      it('should return false if unmute selected', () => {
        handler.lock.locked = false;
        handler.state ={selection: [1]};

        spyOn(handler, 'unmuteSelected').and.returnValue(false);
        spyOn(handler, 'muteSelected').and.callThrough();

        expect(handler.muteDisabled()).toBe(true);

        expect(handler.unmuteSelected).toHaveBeenCalled();
        expect(handler.muteSelected).not.toHaveBeenCalled();
      });

      it('should return true if no unmute selected', () => {
        handler.lock.locked = false;
        handler.state ={selection: [1]};

        spyOn(handler, 'unmuteSelected').and.returnValue(true);
        spyOn(handler, 'muteSelected').and.callThrough();

        expect(handler.muteDisabled()).toBe(false);

        expect(handler.unmuteSelected).toHaveBeenCalled();
        expect(handler.muteSelected).not.toHaveBeenCalled();
      });

    });

    describe('unmuteDisabled', () => {

      it('should return true if locked', () => {
        handler.lock.locked = true;
        handler.state ={selection: [1]};

        spyOn(handler, 'unmuteSelected').and.callThrough();
        spyOn(handler, 'muteSelected').and.callThrough();

        expect(handler.unmuteDisabled()).toBe(true);

        expect(handler.unmuteSelected).not.toHaveBeenCalled();
        expect(handler.muteSelected).not.toHaveBeenCalled();
      });

      it('should return true if no selection', () => {
        handler.lock.locked = false;
        handler.state ={selection: []};

        spyOn(handler, 'unmuteSelected').and.callThrough();
        spyOn(handler, 'muteSelected').and.callThrough();

        expect(handler.unmuteDisabled()).toBe(true);

        expect(handler.unmuteSelected).not.toHaveBeenCalled();
        expect(handler.muteSelected).not.toHaveBeenCalled();
      });

      it('should return false if no mute selected', () => {
        handler.lock.locked = false;
        handler.state ={selection: [1]};

        spyOn(handler, 'unmuteSelected').and.callThrough();
        spyOn(handler, 'muteSelected').and.returnValue(true);

        expect(handler.unmuteDisabled()).toBe(false);

        expect(handler.unmuteSelected).not.toHaveBeenCalled();
        expect(handler.muteSelected).toHaveBeenCalled();
      });

      it('should return true if no mute selected', () => {
        handler.lock.locked = false;
        handler.state ={selection: [1]};

        spyOn(handler, 'unmuteSelected').and.callThrough();
        spyOn(handler, 'muteSelected').and.returnValue(false);

        expect(handler.unmuteDisabled()).toBe(true);

        expect(handler.unmuteSelected).not.toHaveBeenCalled();
        expect(handler.muteSelected).toHaveBeenCalled();
      });

    });

    describe('muteOutliers', () => {
      const addMute = {};
      const mute = {};
      const item = {muted:false};

      const success = () => {};
      const error = () => {};

      beforeEach(() => {
        inject(($injector) => {
          $q = $injector.get('$q');
        });
      });

      describe('body', () => {
        beforeEach(() => {
          spyOn(handler.lock, 'lock').and.callThrough();
          spyOn(handler.lock, 'unlock').and.callThrough();
          spyOn(MuteDataService, 'addMute').and.returnValue(addMute);
          spyOn(handler, 'confirmMuteAction').and.returnValue(confirmMuteAction);
          spyOn($q, 'all').and.returnValue(promise);
          spyOn(MuteHandler, 'getMuteIdentifier').and.returnValue(mute);
        });

        const promise = {
          then: () => promise,
          finally: (func) => {
            func();
            return promise;
          },
          catch: () => promise
        };
        const confirmMuteAction = {
          then: (func)=> {
            return func();
          }
        };

        it('should lock and unlock', () => {
          handler.state ={selection: [item]};

          handler.muteOutliers(success, error);
          expect(handler.lock.lock).toHaveBeenCalled();
          expect(handler.lock.unlock).toHaveBeenCalled();
        });

        it('should call addMute if muted', () => {
          item.muted = false;
          handler.state ={selection: [item]};

          handler.muteOutliers(success, error);
          expect(handler.confirmMuteAction).toHaveBeenCalledWith(handler.state.selection);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteDataService.addMute).toHaveBeenCalledWith(mute);
          expect($q.all).toHaveBeenCalledWith([addMute]);
        });

        it('should return identifier if not muted', () => {
          item.muted = true;
          handler.state ={selection: [item]};

          handler.muteOutliers(success, error);
          expect(handler.confirmMuteAction).toHaveBeenCalledWith(handler.state.selection);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteDataService.addMute).not.toHaveBeenCalled();
          expect($q.all).toHaveBeenCalledWith([mute]);
        });

        it('should handle multiple mutes', () => {
          item.muted = false;
          handler.state ={selection: [item, _.clone(item)]};

          handler.muteOutliers(success, error);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[1]);
          expect(MuteDataService.addMute).toHaveBeenCalledTimes(2);
          expect($q.all).toHaveBeenCalledWith([addMute, addMute]);
        });

        it('should handle multiple mutes where set', () => {
          item.muted = false;
          handler.state ={selection: [item, _.chain(item).clone().tap((item) => item.muted = true).value()]};

          handler.muteOutliers(success, error);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[1]);
          expect(MuteDataService.addMute).toHaveBeenCalledTimes(1);
          expect($q.all).toHaveBeenCalledWith([addMute, addMute]);
        });

      });

      describe('callbacks', () => {
        const promise = {
          then: () => promise,
          finally: () => promise,
          catch: () => promise
        };
        const confirmMuteAction = { then: () =>  promise};

        beforeEach(() => {
          spyOn(handler.lock, 'lock').and.callThrough();
          spyOn(handler.lock, 'unlock').and.callThrough();
          spyOn(MuteDataService, 'addMute').and.returnValue(addMute);
          spyOn(handler, 'confirmMuteAction').and.returnValue(confirmMuteAction);
          spyOn(MuteHandler, 'getMuteIdentifier').and.returnValue(mute);
          spyOn($q, 'all').and.returnValue(promise);
          spyOn(promise, 'then').and.callThrough();
        });

        it('should call then with callbacks', () => {
          handler.muteOutliers(success, error);
          expect(promise.then).toHaveBeenCalledWith(success, error);
        });

      });

    });

    describe('unmuteOutliers', () => {
      const unMute = {};
      const mute = {};
      const item = {muted:false};

      const success = () => {};
      const error = () => {};

      beforeEach(() => {
        inject(($injector) => {
          $q = $injector.get('$q');
        });
      });

      describe('body', () => {
        const promise = {
          then: () => promise,
          finally: (func) => {
            func();
            return promise;
          },
          catch: () => promise
        };

        const confirmUnmuteAction = {
          then: (func)=> {
            return func();
          }
        };

        beforeEach(() => {

          spyOn(handler.lock, 'lock').and.callThrough();
          spyOn(handler.lock, 'unlock').and.callThrough();
          spyOn(MuteDataService, 'unMute').and.returnValue(unMute);
          spyOn(handler, 'confirmUnmuteAction').and.returnValue(confirmUnmuteAction);
          spyOn($q, 'all').and.returnValue(promise);
          spyOn(MuteHandler, 'getMuteIdentifier').and.returnValue(mute);
        });

        it('should lock and unlock', () => {
          handler.state = {selection: [item]};

          handler.unmuteOutliers(success, error);
          expect(handler.lock.lock).toHaveBeenCalled();
          expect(handler.lock.unlock).toHaveBeenCalled();
        });

        it('should call unMute if muted', () => {
          item.muted = true;
          handler.state ={selection: [item]};

          handler.unmuteOutliers(success, error);
          expect(handler.confirmUnmuteAction).toHaveBeenCalledWith(handler.state.selection);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteDataService.unMute).toHaveBeenCalledWith(mute);
          expect($q.all).toHaveBeenCalledWith([unMute]);
        });

        it('should return identifier if not muted', () => {
          item.muted = false;
          handler.state ={selection: [item]};

          handler.unmuteOutliers(success, error);
          expect(handler.confirmUnmuteAction).toHaveBeenCalledWith(handler.state.selection);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteDataService.unMute).not.toHaveBeenCalled();
          expect($q.all).toHaveBeenCalledWith([mute]);
        });

        it('should handle multiple unmutes', () => {
          item.muted = true;
          handler.state ={selection: [item, _.clone(item)]};

          handler.unmuteOutliers(success, error);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[1]);
          expect(MuteDataService.unMute).toHaveBeenCalledTimes(2);
          expect($q.all).toHaveBeenCalledWith([unMute, unMute]);
        });

        it('should handle multiple unmutes where set', () => {
          item.muted = true;
          handler.state ={selection: [item, _.chain(item).clone().tap((item) => item.muted = false).value()]};

          handler.unmuteOutliers(success, error);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[0]);
          expect(MuteHandler.getMuteIdentifier).toHaveBeenCalledWith(handler.state.selection[1]);
          expect(MuteDataService.unMute).toHaveBeenCalledTimes(1);
          expect($q.all).toHaveBeenCalledWith([unMute, unMute]);
        });

      });

      describe('callbacks', () => {
        const promise = {
          then: () => promise,
          finally: () => promise,
          catch: () => promise
        };
        const confirmUnmuteAction = { then: () =>  promise};

        beforeEach(() => {
          spyOn(handler.lock, 'lock').and.callThrough();
          spyOn(handler.lock, 'unlock').and.callThrough();
          spyOn(MuteDataService, 'unMute').and.returnValue(unMute);
          spyOn(handler, 'confirmUnmuteAction').and.returnValue(confirmUnmuteAction);
          spyOn(MuteHandler, 'getMuteIdentifier').and.returnValue(mute);
          spyOn($q, 'all').and.returnValue(promise);
          spyOn(promise, 'then').and.callThrough();
        });

        it('should call then with callbacks', () => {
          handler.unmuteOutliers(success, error);
          expect(promise.then).toHaveBeenCalledWith(success, error);
        });

      });

    });

  });

  describe('OutlierState', () => {
    const model = {};
    const mandatory = {};
    const transient = [];
    const sorting = [];
    const settings = {};
    const limit = 10;
    const grid = {};

    let OutlierState;
    let state;
    let format;

    beforeEach(() => {
      inject($injector => {
        OutlierState = $injector.get('OutlierState');
        format = $injector.get('FORMAT');
      });
      state = new OutlierState(PROJECT, model, mandatory, transient, sorting, settings, limit);
    });

    it('should create a correct instance', () => {
      expect(state.project).toEqual(PROJECT);
      expect(state.vm).toBe(model);
      expect(state.mandatory).toBe(mandatory);
      expect(state.transient).toBe(transient);
      expect(state.sorting).toBe(sorting);
      expect(state.settings).toBe(settings);
      expect(state.limit).toBe(limit);
    });

    it('should setup the lookback with defaults', () => {
      expect(state.lookBack.format(format.ISO_DATE)).toEqual(TWO_WEEKS_AGO);
    });

    it('should setup the lookback with values', () => {
      state = new OutlierState(PROJECT, model, mandatory, transient, sorting, settings, limit, 7, 'days');
      expect(state.lookBack.format(format.ISO_DATE)).toEqual(ONE_WEEKS_AGO);
    });

    describe('onSortChanged', () => {

      it('should update sorting empty onSortChanged', () => {
        state.onSortChanged(grid, []);
        expect(state.sorting).toEqual([]);
      });

      it('should update sorting ', () => {
        state.onSortChanged(grid, [{field:'1', sort:{direction:'up'}} , {field:'2', sort:{direction:'down'}}]);
        expect(state.sorting).toEqual([{field:'1', direction: 'up'}, {field:'2', direction: 'down'}]);
      });

    });

    describe('onRowsRendered', () => {
      const api = {core:{notifyDataChange: () => {}}};
      const getCol = jasmine.createSpy("getCol");
      let EvgUiGridUtil;
      let uiGridConstants;

      beforeEach(() => {
        inject($injector => {
          EvgUiGridUtil = $injector.get('EvgUiGridUtil');
          uiGridConstants = $injector.get('uiGridConstants');
        });
        spyOn(EvgUiGridUtil, 'getColAccessor').and.returnValue(getCol);
        state.vm = {reload: () => {}};
        spyOn(state.vm, 'reload').and.callThrough();
      });

      it('should setup the getCol accessor', () => {
        state.onRowsRendered(api);
        expect(state.api).toBe(api);
      });

      it('should setup the api', () => {
        state.onRowsRendered(api);
        expect(EvgUiGridUtil.getColAccessor).toHaveBeenCalledWith(api);
        expect(state.getCol).toBe(getCol);

        // grid filtering
        expect(getCol).toHaveBeenCalledWith('create_time');
      });

      it('should set initial filtering', () => {
        const spy = spyOnProperty(state, 'defaultFilters', 'get').and.returnValue({'1': 1, '2': 2});
        spyOnProperty(state, 'sorting', 'get').and.returnValue([]);

        state.onRowsRendered(api);

        expect(spy).toHaveBeenCalled();
        expect(getCol).toHaveBeenCalledWith('1');
        expect(getCol).toHaveBeenCalledWith('2');
      });

      it('should set initial sorting', () => {
        spyOnProperty(state, 'defaultFilters', 'get').and.returnValue({});
        const spy = spyOnProperty(state, 'sorting', 'get').and.returnValue([{field:'1'}, {field:'2'}]);

        state.onRowsRendered(api);

        expect(spy).toHaveBeenCalled();
        expect(getCol).toHaveBeenCalledWith('1');
        expect(getCol).toHaveBeenCalledWith('2');
      });

      it('should notifyDataChange', () => {
        spyOn(api.core, 'notifyDataChange').and.callThrough();
        state.onRowsRendered(api);
        expect(api.core.notifyDataChange).toHaveBeenCalledWith(uiGridConstants.dataChange.COLUMN);
      });

      it('should reload', () => {
        state.onRowsRendered(api);
        expect(state.vm.reload).toHaveBeenCalled();
      });

    });

    describe('onMute', () => {
      beforeEach(() => {
        model.gridOptions = {data:[{name:0}, {name: 1}, {name: 2}, {name: 3}, {name: 4}]};
      });

      it('should handle null mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onMute(null);
        expect(state.refreshGridData).not.toHaveBeenCalled();
      });

      it('should handle no mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onMute([]);
        expect(state.refreshGridData).not.toHaveBeenCalled();
      });

      it('should handle updating even mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onMute([{name:0}, {name: 2}, {name: 4} ]);
        const even = _.chain(model.gridOptions.data).filter(i => i.name % 2 === 0).all(i => i.muted).value();
        expect(even).toBe(true);

        const odd = _.chain(model.gridOptions.data).filter(i => i.name % 2 !== 0).all(i => _.isUndefined(i.muted)).value();
        expect(odd).toBe(true);

        expect(state.refreshGridData).toHaveBeenCalled();
      });

      it('should handle updating event mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onMute([{name:1}, {name: 3}, {name: 5} ]);
        const even = _.chain(model.gridOptions.data).filter(i => i.name % 2 === 0).all(i => _.isUndefined(i.muted)).value();
        expect(even).toBe(true);

        const odd = _.chain(model.gridOptions.data).filter(i => i.name % 2 !== 0).all(i => i.muted).value();
        expect(odd).toBe(true);
        expect(state.refreshGridData).toHaveBeenCalled();
      });

    });

    describe('onUnmute', () => {

      beforeEach(() => {
        model.gridOptions = {data:[{name:0}, {name: 1}, {name: 2}, {name: 3}, {name: 4}]};
      });

      it('should handle null mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onUnmute(null);
        expect(state.refreshGridData).not.toHaveBeenCalled();
      });

      it('should handle no mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onUnmute([]);
        expect(state.refreshGridData).not.toHaveBeenCalled();
      });

      it('should handle updating even mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onUnmute([{name:0}, {name: 2}, {name: 4} ]);
        const even = _.chain(model.gridOptions.data).filter(i => i.name % 2 === 0).all(i => i.muted).value();
        expect(even).toBe(false);

        const odd = _.chain(model.gridOptions.data).filter(i => i.name % 2 !== 0).all(i => _.isUndefined(i.muted)).value();
        expect(odd).toBe(true);
        expect(state.refreshGridData).toHaveBeenCalled();
      });

      it('should handle updating event mutes', () => {
        spyOn(state, 'refreshGridData').and.callThrough();
        state.onUnmute([{name:1}, {name: 3}, {name: 5} ]);
        const even = _.chain(model.gridOptions.data).filter(i => i.name % 2 === 0).all(i => _.isUndefined(i.muted)).value();
        expect(even).toBe(true);

        const odd = _.chain(model.gridOptions.data).filter(i => i.name % 2 !== 0).all(i => i.muted).value();
        expect(odd).toBe(false);
        expect(state.refreshGridData).toHaveBeenCalled();
      });

    });

    describe('refreshGridData', () => {

      it('should handle null api', () => {
        state.api = null;
        state.refreshGridData();
      });

      it('should clear selected rows', () => {
        state.api = {selection: {clearSelectedRows:() => {}}};
        spyOn(state.api.selection, 'clearSelectedRows').and.callThrough();
        state.refreshGridData();
        expect(state.api.selection.clearSelectedRows).toHaveBeenCalled();
      });

    });

    describe('selection', () => {

      it('should handle null api', () => {
        state.api = null;
        expect(state.selection).toEqual([]);
      });

      it('should clear selected rows', () => {
        state.api = {selection: {getSelectedRows:() => {}}};
        spyOn(state.api.selection, 'getSelectedRows').and.returnValue([1]);
        expect(state.selection).toEqual([1]);
        expect(state.api.selection.getSelectedRows).toHaveBeenCalled();
      });
    });

    describe('sorting', () => {

      it('should return sorting', () => {
        const sorting = [];
        state._sorting = sorting;
        expect(state.sorting).toBe(sorting);
      });

    });

    describe('mode', () => {

      it('should return correct value', () => {
        const mode = [];
        state.vm = {mode: {value: mode}};
        expect(state.mode).toBe(mode);
      });

    });

    describe('lowConfidence', () => {

      it('should return correct value', () => {
        const lowConfidence = [];
        state.vm = {checkboxModel: {lowConfidence: lowConfidence}};
        expect(state.lowConfidence).toBe(lowConfidence);
      });

    });

    describe('columns', () => {

      it('should handle null api', () => {
        state.api = null;
        expect(state.columns).toEqual([]);
      });

      it('should return columns', () => {
        const columns = [];
        state.api = {grid: {columns:columns}};
        expect(state.columns).toBe(columns);
      });
    });

    describe('filters', () => {

      it('should omitTransientFilters', () => {
        const spy = spyOnProperty(state, 'columns', 'get').and.returnValue([]);
        spyOn(state, 'omitTransientFilters').and.callThrough();
        state.filters;
        expect(spy).toHaveBeenCalled();
        expect(state.omitTransientFilters).toHaveBeenCalled();
      });

      it('should no column data', () => {
        const expected = {};
        spyOnProperty(state, 'columns', 'get').and.returnValue([]);

        expect(state.filters).toEqual(expected);
        expect(state.settings.persistentFiltering).toEqual(expected);
      });

      it('should get column data', () => {
        const create_time = {field: 'create_time', filters:[{term:'>2019-04-25'}]};
        const variant = {field: 'variant', filters:[{term:'linux-standalone'}]};
        const expected = {create_time: '>2019-04-25', variant: 'linux-standalone'};

        spyOnProperty(state, 'columns', 'get').and.returnValue([create_time, variant]);

        expect(state.filters).toEqual(expected);
        expect(state.settings.persistentFiltering).toEqual(expected);
      });

    });

    describe('secondaryDefaultFiltering', () => {

      it('should be two weeks ago', () => {
        expect(state.secondaryDefaultFiltering).toEqual({create_time: '>' + TWO_WEEKS_AGO});
      });

    });

    describe('mandatoryDefaultFiltering', () => {

      it('should be mandatory', () => {
        expect(state.mandatoryDefaultFiltering).toBe(mandatory);
      });

    });

    describe('defaultFilters', () => {

      it('should extend filters', () => {
        const expected = {};
        spyOn(_, 'extend').and.returnValue(expected);
        spyOnProperty(state, 'secondaryDefaultFiltering', 'get').and.returnValue('secondaryDefaultFiltering');
        state.settings.persistentFiltering = 'persistentFiltering';
        spyOnProperty(state, 'mandatoryDefaultFiltering', 'get').and.returnValue('mandatoryDefaultFiltering');

        expect(state.defaultFilters).toBe(expected);
        expect(_.extend).toHaveBeenCalledWith({}, 'secondaryDefaultFiltering', 'persistentFiltering', 'mandatoryDefaultFiltering');
      });

    });

    describe('getFilteringContext', () => {
      let EvgUiGridUtil;
      beforeEach(() => {
        inject($injector => EvgUiGridUtil = $injector.get('EvgUiGridUtil'));
      });

      it('should handle low confidence', () => {
        spyOnProperty(state, 'filters', 'get').and.returnValue([]);
        spyOnProperty(state, 'lowConfidence', 'get').and.returnValue(true);

        expect(state.getFilteringContext()).toBe(null);
      });

      it('should handle high confidence', () => {
        spyOnProperty(state, 'filters', 'get').and.returnValue([]);
        spyOnProperty(state, 'lowConfidence', 'get').and.returnValue(false);

        expect(state.getFilteringContext()).toEqual( [{
          field: 'type',
          type: 'string',
          term: '=detected'
        }]);
      });

      it('should skip filters when no accessor', () => {
        spyOnProperty(state, 'filters', 'get').and.returnValue({test: "canary_ping", create_time: ">2019-04-25"});
        spyOnProperty(state, 'lowConfidence', 'get').and.returnValue(false);

        expect(state.getFilteringContext()).toEqual( [{
          field: 'type',
          type: 'string',
          term: '=detected'
        }]);
      });

      it('should process filters', () => {
        const girdApi ={grid: {
          columns: [
              {field: "test", colDef: {type: "string"}},
              {field: "create_time", colDef: {type: "foo"}},
            ]
        }};

        spyOnProperty(state, 'filters', 'get').and.returnValue({test: "canary_ping",
                                                                create_time: ">2019-04-25",
                                                                missing: ">2019-04-25"});
        spyOnProperty(state, 'lowConfidence', 'get').and.returnValue(false);

        state.getCol = EvgUiGridUtil.getColAccessor(girdApi);
        expect(state.getFilteringContext()).toEqual( [{
          field: 'type',
          type: 'string',
          term: '=detected'
        },{
          field: 'test',
          type: 'string',
          term: 'canary_ping'
        },{
          field: 'create_time',
          type: 'foo',
          term: '>2019-04-25'
        }]);
      });

    });

    describe('getAggChain', () => {
      let MDBQueryAdaptor;
      beforeEach(() => {
        inject($injector => MDBQueryAdaptor = $injector.get('MDBQueryAdaptor'));
      });

      it('should compile filtering context', () => {
        spyOn(MDBQueryAdaptor, 'compileFiltering').and.returnValue(null);
        spyOn(state, 'getFilteringContext').and.returnValue('getFilteringContext');
        spyOnProperty(state, 'sorting', 'get').and.returnValue(null);

        state.getAggChain();
        expect(MDBQueryAdaptor.compileFiltering).toHaveBeenCalledWith('getFilteringContext');
      });

      it('should compile sorting', () => {
        spyOn(MDBQueryAdaptor, 'compileFiltering').and.returnValue(null);
        spyOn(MDBQueryAdaptor, 'compileSorting').and.returnValue(null);
        spyOn(state, 'getFilteringContext').and.returnValue('getFilteringContext');
        spyOnProperty(state, 'sorting', 'get').and.returnValue('get sorting');

        state.getAggChain();
        expect(MDBQueryAdaptor.compileSorting).toHaveBeenCalledWith('get sorting');
      });

      it('should only apply limit if no filter or sort', () => {
        spyOn(MDBQueryAdaptor, 'compileFiltering').and.returnValue(null);
        spyOn(state, 'getFilteringContext').and.returnValue('getFilteringContext');
        spyOnProperty(state, 'sorting', 'get').and.returnValue(null);

        expect(state.getAggChain()).toEqual([{$limit: 10}]);
      });

      it('should produce a full chain', () => {
        spyOn(MDBQueryAdaptor, 'compileFiltering').and.returnValue('compileFiltering');
        spyOn(MDBQueryAdaptor, 'compileSorting').and.returnValue('compileSorting');
        spyOn(state, 'getFilteringContext').and.returnValue('getFilteringContext');
        spyOnProperty(state, 'sorting', 'get').and.returnValue('get sorting');

        expect(state.getAggChain()).toEqual(['compileFiltering', 'compileSorting', {$limit: 10}]);
      });

    });

    describe('sortRevision', () => {
      let a, b, rowA, rowB;

      it('should handle no api', () => {
        state.api = null;
        expect(state.sortRevision(a, b, rowA, rowB)).toBe(null);
      });

      it('should return sortHandleNulls', () => {
        const nulls = {};
        state.api = {core: {sortHandleNulls: () => nulls}};

        expect(state.sortRevision(a, b, rowA, rowB)).toBe(nulls);
      });

      it('should return 0 when a is b', () => {
        a = b = 1;
        state.api = {core: {sortHandleNulls: () => null}};

        expect(state.sortRevision(a, b, rowA, rowB)).toBe(0);
      });

      it('should return row delta when available', () => {
        b = 1;
        a = 2;
        rowB = {entity:{order:10}};
        rowA = {entity:{order:20}};
        state.api = {core: {sortHandleNulls: () => null}};

        expect(state.sortRevision(a, b, rowA, rowB)).toBe(10);
      });

      it('should return delta otherwise', () => {
        b = 10;
        a = 21;
        rowB = null;
        rowA = null;
        state.api = {core: {sortHandleNulls: () => null}};

        expect(state.sortRevision(a, b, rowA, rowB)).toBe(11);
      });

    });

    describe('loadData', () => {
      let $q, OutliersDataService, MuteDataService;
      beforeEach(() => {
        inject($injector => {
          $q = $injector.get('$q');
          OutliersDataService = $injector.get('OutliersDataService');
          MuteDataService = $injector.get('MuteDataService');
        });
      });

      it('should handle no api', () => {
        const pipeline = {};
        const aggregateQ = {};
        const queryQ = {};
        const operation = 1;
        const promise = {};

        spyOn(state, 'getAggChain').and.returnValue(pipeline);
        spyOn(OutliersDataService, 'aggregateQ').and.returnValue(aggregateQ);
        spyOn(MuteDataService, 'queryQ').and.returnValue(aggregateQ);
        spyOn($q, 'all').and.returnValue(promise);

        expect(state.loadData(operation)).toBe(promise);

        expect(state.getAggChain).toHaveBeenCalled();
        expect(OutliersDataService.aggregateQ).toHaveBeenCalledWith(pipeline);
        expect(MuteDataService.queryQ).toHaveBeenCalledWith({project: PROJECT});
        expect($q.all).toHaveBeenCalledWith({
            outliers: aggregateQ,
            mutes: queryQ,
            operation: operation
        });
      });

    });

    describe('hydrateData', () => {
      const create_mute = params => {
        const {i, revision, project, variant, task, test, thread_level, enabled} = params;
        return {
          revision: revision || `revision ${i}`,
          project: project|| 'sys-perf',
          variant: variant || 'linux-standalone',
          task: task || 'bestbuy_agg',
          test: test || 'canary_ping',
          thread_level: thread_level || '1',
          enabled: _.isUndefined(enabled) ? false : !!enabled,
        }
      };

      const create_outlier = params => {
        let outlier = create_mute(params);
        outlier['create_time']  = params.create_time || params['i'] || 1;
        delete outlier['enabled'];
        return outlier;
      };

      it('should set data and return', () => {
        const data = [];
        const outliers = [];
        const mutes = [];
        spyOn(_, 'each').and.returnValue(data);

        state.vm = {gridOptions:{data:{}}};
        expect(state.hydrateData({outliers:outliers, mutes:mutes})).toBe(data);

        expect(_.each).toHaveBeenCalledWith(outliers, jasmine.any(Function));
        expect(state.vm.gridOptions.data).toBe(data);
      });

      it('should set matching to unmuted', () => {
        const outliers = [create_outlier({i: 1})];
        const mutes = [create_mute({i: 1})];

        state.vm = {gridOptions:{data:{}}};
        const results = state.hydrateData({outliers:outliers, mutes:mutes});

        expect(results.length).toEqual(1);
        expect(_.all(results, (result) => result.muted)).toEqual(false);
      });

      it('should set matching to muted', () => {
        const outliers = [create_outlier({i: 1})];
        const mutes = [create_mute({i: 1, enabled: true})];

        state.vm = {gridOptions:{data:{}}};
        const results = state.hydrateData({outliers: outliers, mutes: mutes});

        expect(results.length).toEqual(1);

        expect(results.length).toEqual(1);
        expect(_.all(results, (result) => result.muted)).toEqual(true);
      });

      it('should toggle muted', () => {
        const outliers = [create_outlier({i: 1})];
        let mutes = [create_mute({i: 1, enabled: true})];

        state.vm = {gridOptions:{data:{}}};
        state.hydrateData({outliers: outliers, mutes: mutes});

        mutes = [create_mute({i: 1, enabled: false})];
        const results = state.hydrateData({outliers: outliers, mutes: mutes});

        expect(results.length).toEqual(1);
        expect(_.all(results, (result) => result.muted)).toEqual(false);
      });

      it('should handle multiple', () => {
        const outliers = [create_outlier({i: 1}), create_outlier({i: 2})];
        let mutes = [create_mute({i: 1, enabled: true}), create_mute({i: 2, enabled: true})];

        state.vm = {gridOptions:{data:{}}};
        const results = state.hydrateData({outliers: outliers, mutes: mutes});

        expect(results.length).toEqual(2);
        expect(_.all(results, (result) => result.muted)).toEqual(true);
      });

      it('should update correctly', () => {
        const outliers = [
          create_outlier({i: 1}),
          create_outlier({i: 2}),
          create_outlier({i: 3}),
          create_outlier({i: 4}),
          create_outlier({i: 5}),
        ];
        let mutes = [
          create_mute({i: 2, enabled: true}),
          create_mute({i: 4, enabled: true})
        ];

        state.vm = {gridOptions:{data:{}}};
        const results = state.hydrateData({outliers: outliers, mutes: mutes});

        const even = _.chain(results).filter((element, index) => index % 2 !== 0).value();
        expect(even.length).toEqual(2);
        expect(_.all(even, (result) => result.muted)).toEqual(true);

        const odd = _.chain(results).filter( (element, index) => index % 2 === 0).value();
        expect(odd.length).toEqual(3);
        expect(_.all(odd, (result) => result.muted)).toEqual(false);
      });

    });

    describe('omitTransientFilters', () => {

      it('should omit fields', () => {
        const filtering = {1: 1, 2: 2, 3: 3, 4: 4, 5: 5};
        const transient = [1, 3, 5];

        state.transient = transient;
        expect(state.omitTransientFilters(filtering)).toEqual({2: 2, 4: 4});
      });

    });

  });

  describe('Operations', () => {
    let Operations;
    beforeEach(() => {
      inject($injector => Operations = $injector.get('Operations'));
    });

    it('should start at 0', () => {
      expect(new Operations().current).toBe(0);
    });

    it('should get next', () => {
      const operations = new Operations();
      expect(operations.next).toBe(1);
      expect(operations.next).toBe(2);
      expect(operations.next).toBe(3);
    });

    it('should return correct isCurrent status', () => {
      const operations = new Operations();
      expect(operations.isCurrent(0)).toBe(true);
      operations.next;
      expect(operations.isCurrent(0)).toBe(false);
      expect(operations.isCurrent(1)).toBe(true);
    });

  });

  describe('confirmDialogFactory', () => {
    let confirmDialogFactory;
    let $mdDialog;

    beforeEach(() => {
      module($provide => {
        $mdDialog = {
          show: () => $mdDialog,
          confirm: () => $mdDialog,
          ok: () => $mdDialog,
          cancel: () => $mdDialog,
          title: () => $mdDialog,
          textContent: () => $mdDialog
        };
        spyOn($mdDialog, 'textContent').and.callThrough();
        $provide.value('$mdDialog', $mdDialog);
      });

      inject($injector => confirmDialogFactory = $injector.get('confirmDialogFactory'));
    });

    it('should handle a function', () => {
      confirmDialogFactory(function(items) {
        return 'This is a test : ' + items.length;
      })([]);
      expect($mdDialog.textContent).toHaveBeenCalledWith('This is a test : 0');
    });

    it('should handle pass on items ', () => {
      confirmDialogFactory(function(items) {
        return 'This is a test : ' + items.length;
      })([1, 2, 3]);
      expect($mdDialog.textContent).toHaveBeenCalledWith('This is a test : 3');
    });

    it('should handle a text', () => {
      confirmDialogFactory('This is a test.')([]);
      expect($mdDialog.textContent).toHaveBeenCalledWith('This is a test.');
    });
  });

  describe('outlierTypeToConfidence', () => {
    let outlierTypeToConfidence;

    beforeEach(() => inject($filter => outlierTypeToConfidence = $filter('outlierTypeToConfidence')));

    it('should return high for detected', () => expect(outlierTypeToConfidence('detected')).toEqual('high'));

    it('should return low for detected', () => expect(outlierTypeToConfidence('suspicious')).toEqual('low'));

    it('should return null otherwise', () => expect(outlierTypeToConfidence('some other value')).toEqual(undefined));

  });

});