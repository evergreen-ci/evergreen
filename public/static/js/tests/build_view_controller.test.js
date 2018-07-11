/**
 *  build_view_controller.test.js
 *
 *  Created on: September 11, 2013
 *      Author: Valeri Karpov
 *
 *  Karma-based unit tests for build.js/BuildViewController (see public/static/js/tests/conf/karma.conf.js)
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
          },
          $window: {
            build: {
              Version: {},
              Build: {},
              Tasks: [{
                Task: {}
              }],
            },
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
      Version: {},
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
      Version: {},
      Tasks : [
        { Task : { start_time : '04 Oct 2018 1:00:00 GMT', finish_time: '04 Oct 2018 1:01:00 GMT'} },
        { Task : { start_time : '04 Oct 2018 1:00:00 GMT', finish_time: '04 Oct 2018 1:20:00 GMT'} },
        { Task : { start_time : '04 Oct 2018 1:00:00 GMT', finish_time: '04 Oct 2018 1:35:00 GMT'} },
        { Task : { start_time : '04 Oct 2018 1:00:00 GMT', finish_time: '04 Oct 2018 1:00:00 GMT'}},
      ],
    };
    scope.setBuild(mockBuild);

    expect(scope.computed.maxTaskTime).toBe(2100000000000); // 35 mins in nanoseconds
  });
});
