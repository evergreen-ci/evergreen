describe('AdminSettingsController', function() {
  beforeEach(module('MCI'));

  var controller;
  var scope;

  beforeEach(inject(function($rootScope, $controller, $injector) {
    scope = $rootScope;
    scope.Settings = {};
    controller = $controller('AdminSettingsController', {
      $scope: scope,
      $window: {},
      mciAdminRestService: $injector.get('mciAdminRestService'),
      notificationService: {}
    });
  }));

  describe('chipToUserJSON', function () {
    it('returns a chip object for valid json', function() {
      var input = '{"username": "u", "password": "p"}';
      expect(scope.chipToUserJSON(input)).toEqual(
        {"username": "u", "password": "p"}
      );
    });

    it('returns null for invalid json', function() {
      var input = 'blah';
      expect(scope.chipToUserJSON(input)).toBe(
        null
      );
    });

    it('returns null for missing username', function() {
      var input = '{"password": "p"}';
      expect(scope.chipToUserJSON(input)).toBe(
        null
      );
    });
  });

  describe('clearSection', function () {
    it('clears Settings correctly', function() {
      scope.clearSection("section");

      expect(scope.Settings).toEqual(
        {"section": {}}
      );
    });

    it('clears subsections correctly', function() {
      scope.Settings.section = {};
      scope.clearSection("section", "subsection");

      expect(scope.Settings).toEqual(
        {"section": {"subsection": {}}}
      );
    });
  });
});
