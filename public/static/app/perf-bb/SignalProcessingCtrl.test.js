describe('SignalProcessingFactoriesTest', () => {

  beforeEach(module('MCI'));
  describe('ModeToItemVisibilityMap', () => {

    let ModeToItemVisibilityMap;
    let PROCESSED_TYPE;

    beforeEach(() => {
      inject($injector => {
        ModeToItemVisibilityMap = $injector.get('ModeToItemVisibilityMap');
        PROCESSED_TYPE = $injector.get('PROCESSED_TYPE');
      });
    });

    describe('processed', () => {
      let processed;
      beforeEach(() => processed = ModeToItemVisibilityMap['processed']);

      it('empty should be false', () => expect(processed({})).toBe(false))
      it('null should be true', () => expect(processed({ processed_type: null })).toBe(true))
      it('NONE should be false', () => expect(processed({ processed_type: PROCESSED_TYPE.NONE })).toBe(false))
      it('HIDDEN should be true', () => expect(processed({ processed_type: PROCESSED_TYPE.HIDDEN })).toBe(true))
      it('ACKNOWLEDGED should be true', () => expect(processed({ processed_type: PROCESSED_TYPE.ACKNOWLEDGED })).toBe(true))
    });

    describe('unprocessed', () => {
      let unprocessed;
      beforeEach(() => unprocessed = ModeToItemVisibilityMap['unprocessed']);

      it('empty should be true', () => expect(unprocessed({})).toBe(true))
      it('null should be true', () => expect(unprocessed({ processed_type: null })).toBe(true))
      it('NONE should be true', () => expect(unprocessed({ processed_type: PROCESSED_TYPE.NONE })).toBe(true))
      it('HIDDEN should be false', () => expect(unprocessed({ processed_type: PROCESSED_TYPE.HIDDEN })).toBe(false))
      it('ACKNOWLEDGED should be false', () => expect(unprocessed({ processed_type: PROCESSED_TYPE.ACKNOWLEDGED })).toBe(false))
    });
  });

  describe('SignalProcessingCtrl', () => {
    let $controller;
    let settings;

    beforeEach(inject(function(_$controller_, _Settings_) {
      $controller = _$controller_;
      settings = _Settings_;
    }));

    describe('changePointFilter', () => {
      let $scope;
      let controller;

      beforeEach(function() {
        $scope = {};
        controller = $controller('SignalProcessingCtrl', {$scope: $scope});
      });

      afterEach(function() {
        settings.perf.signalProcessing.persistentFiltering = {};
      });

      it('should use defaults if no user preference specified', () => {
        expect(controller.changePointFilter()).toEqual(controller.defaultChangePointFilter);
      });

      it('should use user preferences if specified', () => {
        const filtering = {
          create_time: ">now",
          project: "=my-project",
          probability: ">3.14",
        };
        settings.perf.signalProcessing.persistentFiltering = filtering;
        expect(controller.changePointFilter()).toEqual(filtering);
      });
    });
  });
});
