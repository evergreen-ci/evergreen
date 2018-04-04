describe('AdminEventsController', function() {
  beforeEach(module('MCI'));

  var controller;
  var scope;

  beforeEach(inject(function($rootScope, $controller, $injector) {
    scope = $rootScope;
    controller = $controller('AdminEventsController', {
      $scope: scope,
      $window: {},
      mciAdminRestService: $injector.get('mciAdminRestService'),
      notificationService: {}
    });
  }));

  describe('getNextTs', function () {
    it('should correctly parse a paginated link', function() {
      var link = '<http://localhost:9090/rest/v2/admin/events?limit=10&ts=2018-03-01T14%3A52%3A43-05%3A00>; rel="next"';
      expect(scope.getNextTs(link)).toEqual(
        '2018-03-01T14:52:43-05:00'
      );
    });
  });

  describe('getDiffText', function () {
    it('should return the correct display text for adding data', function() {
      var diff = {
        kind: "N",
        path: ["foo"],
        rhs: "newVal"
      };
      expect(scope.getDiffText(diff)).toEqual(
        {property: "foo", before: "", after: "newVal"}
      );
    });

    it('should return the correct display text for deleting data', function() {
      var diff = {
        kind: "E",
        path: ["foo", "bar"],
        lhs: "oldVal",
        rhs: null
      };
      expect(scope.getDiffText(diff)).toEqual(
        {property: "foo.bar", before: "oldVal", after: null}
      );
    });

    it('should return the correct display text for modifying an array', function() {
      var diff = {
        kind: "A",
        path: ["foo"],
        index: 1,
        item: {
          kind: "N",
          rhs: "newVal"
        }
      };
      expect(scope.getDiffText(diff)).toEqual(
        {property: "foo[1]", before: "", after: "newVal" }
      );
    });
  });
});
