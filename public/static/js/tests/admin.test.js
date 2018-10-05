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

  describe('addCredential', function () {
    var validChip = "foo:bar";
    var invalidChip = "foobar";

    it('returns a chip object for valid input', function() {
      scope.tempCredentials = {};
      expect(scope.addCredential(validChip)).toEqual(
        {"foo": "bar"}
      );
      // adding the same thing twice is an error
      expect(scope.addCredential(validChip)).toBe(
        null
      );
    });

    it('returns null for invalid chips', function() {
      expect(scope.addCredential(invalidChip)).toBe(
        null
      );
    });
  });

  describe('transformNaiveUser', function () {
    it('returns a chip object for valid json', function() {
      var input = '{"username": "u", "password": "p"}';
      expect(scope.transformNaiveUser(input)).toEqual(
        {"username": "u", "password": "p"}
      );
    });

    it('returns null for invalid json', function() {
      var input = 'blah';
      expect(scope.transformNaiveUser(input)).toBe(
        null
      );
    });

    it('returns null for missing username', function() {
      var input = '{"password": "p"}';
      expect(scope.transformNaiveUser(input)).toBe(
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
