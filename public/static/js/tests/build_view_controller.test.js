/**
 *  build_view_controller.test.js
 *
 *  Created on: September 11, 2013
 *      Author: Valeri Karpov
 *
 *  Karma-based unit tests for build.js/BuildViewController (see $mci_home/ui/static/js/conf/karma.conf.js)
 *
 */

describe("BuildViewController", function() {
  beforeEach(module('MCI'));

  var controller = null;
  var scope = null;
  var $httpBackend = null;
  var date = null;
  var $timeout = null;

  beforeEach(inject(function($rootScope, $controller, $injector) {
    scope = $rootScope;
    $httpBackend = $injector.get('$httpBackend');
    $timeout = $injector.get('$timeout');

    date = new Date(2013, 8, 26);

    controller = $controller("BuildViewController",
        {
          $scope : scope,
          mciTime : {
            now : function() { return date; },
            fromMilliseconds : function(ms) { return new Date(ms); },
            fromNanoseconds : function(ns) { return new Date(ns / (1000 * 1000)) },
            finishConditional : function() { return 0; }
          }
        });
  }));

  afterEach(function() {
    $httpBackend.verifyNoOutstandingExpectation();
    $httpBackend.verifyNoOutstandingRequest();
  });

  it("should make build visible to UI", function() {
    var mockBuild = {
      Build : {
        activated_time : '2013-08-26'
      },
      Tasks : []
    };

    scope.setBuild(mockBuild);

    expect(scope.build).toBe(mockBuild);
    expect(scope.build.Build.activated_time.getTime()).toBeGreaterThan(0);
    expect(scope.lastUpdate.getTime()).toBe(new Date(2013, 8, 26).getTime());
    expect(scope.computed.maxTaskTime).toBeGreaterThan(0);
  });

  it("should compute the maximum task time properly", function() {
    var mockBuild = {
      Build : {
        activated_time : '2013-08-26'
      },
      Tasks : [
        { Task : { TimeTaken : 25 } },
        { Task : { TimeTaken : 2 } },
        { Task : { TimeTaken : 35 } },
        { Task : { TimeTaken : 0 } },
      ]
    };
    scope.setBuild(mockBuild);

    expect(scope.computed.maxTaskTime).toBe(35);
  });

  it("should ping the correct route to refresh its data", function() {
    var mockBuild = {
      Build : {
        activated_time : '2013-08-26',
        _id : "FAKEBUILDID"
      },
      Tasks : [
        { Task : { TimeTaken : 25 } },
        { Task : { TimeTaken : 2 } },
        { Task : { TimeTaken : 35 } },
        { Task : { TimeTaken : 0 } },
      ]
    };
    scope.setBuild(mockBuild);
    expect(scope.lastUpdate.getTime()).toBe(new Date(2013, 8, 26).getTime());

    var reloadedBuild = angular.copy(mockBuild);
    reloadedBuild.Tasks.push({ Task : { TimeTaken : 50 } });

    date = new Date(2013, 8, 27);

    $httpBackend.expectGET('/json/build/FAKEBUILDID').respond(200, reloadedBuild);
    scope.reloadBuild();
    $httpBackend.flush();

    expect(scope.build).toBe(reloadedBuild);
    expect(scope.lastUpdate.getTime()).toBe(new Date(2013, 8, 27).getTime());
    expect(scope.computed.maxTaskTime).toBe(50);
  });

  it("should reload after 10 minutes", function() {
    var mockBuild = {
      Build : {
        activated_time : '2013-08-26',
        _id : "FAKEBUILDID"
      },
      Tasks : [
        { Task : { TimeTaken : 25 } },
        { Task : { TimeTaken : 2 } },
        { Task : { TimeTaken : 35 } },
        { Task : { TimeTaken : 0 } },
      ]
    };
    scope.setBuild(mockBuild);
    expect(scope.lastUpdate.getTime()).toBe(new Date(2013, 8, 26).getTime());

    var reloadedBuild = angular.copy(mockBuild);
    reloadedBuild.Tasks.push({ Task : { TimeTaken : 50 } });

    date = new Date(2013, 8, 28);

    $httpBackend.expectGET('/json/build/FAKEBUILDID').respond(200, reloadedBuild);
    $timeout.flush(10 * 60 * 1000 + 1);

    $httpBackend.flush();
    expect(scope.build).toBe(reloadedBuild);
    expect(scope.lastUpdate.getTime()).toBe(new Date(2013, 8, 28).getTime());
    expect(scope.computed.maxTaskTime).toBe(50);
  });
});