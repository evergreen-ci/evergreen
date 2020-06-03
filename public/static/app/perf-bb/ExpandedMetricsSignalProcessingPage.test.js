describe('ExpandedMetricsSignalProcessingPage', () => {

  beforeEach(module('MCI'));

  describe('SignalProcessingCtrl', () => {
    let controller;
    let $scope;
    let CHANGE_POINTS_GRID;
    let PerformanceAnalysisAndTriageClient;
    let $routeParams;
    let $httpBackend;
    let PERFORMANCE_ANALYSIS_AND_TRIAGE_API;

    beforeEach(function () {
      module('MCI');
      inject(function (_$controller_, _PerformanceAnalysisAndTriageClient_, _$window_, _CHANGE_POINTS_GRID_, _$routeParams_, _$httpBackend_, _PERFORMANCE_ANALYSIS_AND_TRIAGE_API_, _CEDAR_APP_URL_) {
        $scope = {};
        CHANGE_POINTS_GRID = _CHANGE_POINTS_GRID_;
        PerformanceAnalysisAndTriageClient = _PerformanceAnalysisAndTriageClient_;
        $routeParams = _$routeParams_;
        $routeParams.projectId = 'some-project';
        $httpBackend = _$httpBackend_;
        PERFORMANCE_ANALYSIS_AND_TRIAGE_API = _PERFORMANCE_ANALYSIS_AND_TRIAGE_API_;
        controller = _$controller_;
      })
    });

    function makeController() {
      controller('ExpandedMetricsSignalProcessingController', {
        '$scope': $scope,
        '$routeParams': $routeParams,
        'PerformanceAnalysisAndTriageClient': PerformanceAnalysisAndTriageClient,
        'CHANGE_POINTS_GRID': CHANGE_POINTS_GRID
      });
    }

    it('should authenticate on failed requests', () => {
      makeController();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 0, 10, 511, $scope)
        .respond(function(){
          return [null, null, null, null, 'error']
        });
      $httpBackend.flush();
      $httpBackend.verifyNoOutstandingExpectation();
      $httpBackend.verifyNoOutstandingRequest();
      expect($scope.connectionError).toEqual(true);
    });

    it('should be refresh the data when a new page is loaded', () => {
      makeController();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 0, 10, 511, $scope);
      $httpBackend.flush();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 1, 10, 511, $scope, {
        'versions': Array(10).fill(
          {
            'version_id': 'another_version',
            'change_points': [
              {
                "time_series_info": {
                  "project": "sys-perf",
                  "variant": "another_variant",
                  "task": "another_task",
                  "test": "another_test",
                  "measurement": "another_measurement",
                  "thread_level": 5
                },
                "cedar_perf_result_id": "7a8a54244e0bf868bf1d1edd2f388614aecb16bd",
                "version": "another_version",
                "order": 22151,
                "algorithm": {
                  "name": "e_divisive_means",
                  "version": 0,
                  "options": [
                    {
                      "name": "pvalue",
                      "value": 0.05
                    },
                    {
                      "name": "permutations",
                      "value": 100
                    }
                  ]
                },
                "triage": {
                  'triaged_on': '0001-01-01T00:00:00Z',
                  'triage_status': 'triaged'
                },
                "percent_change": 12.3248234827374,
                "calculated_on": "2020-05-04T20:21:12.037000"
              }]
          }),
        'page': 1,
        'page_size': 10,
        'total_pages': 511
      });
      $scope.nextPage();
      $httpBackend.flush();
      expect($scope.page).toEqual(1);
      expect($scope.pageSize).toEqual(10);
      expect($scope.totalPages).toEqual(511);
      expect($scope.gridOptions.data).toEqual(Array(10).fill({
        version: 'another_version',
        variant: 'another_variant',
        task: 'another_task',
        test: 'another_test',
        measurement: 'another_measurement',
        percent_change: '12.32',
        triage_status: 'triaged',
        thread_level: 5,
      }));
      $httpBackend.verifyNoOutstandingExpectation();
      $httpBackend.verifyNoOutstandingRequest();
    });

    it('should be able to go to the previous page', () => {
      makeController();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 0, 10, 511, $scope);
      $httpBackend.flush();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 510, 10, 511, $scope);
      $scope.page = 511;
      $scope.prevPage();
      $httpBackend.flush();
      expect($scope.page).toEqual(510);
      expect($scope.pageSize).toEqual(10);
      expect($scope.totalPages).toEqual(511);
      expect($scope.gridOptions.data).toEqual(Array(10).fill({
        version: 'sys_perf_085ffeb310e8fed49739cf8443fcb13ea795d867',
        variant: 'linux-standalone',
        task: 'large_scale_model',
        test: 'HotCollectionDeleter.Delete.2.2',
        measurement: 'AverageSize',
        percent_change: '50.32',
        triage_status: 'untriaged',
        thread_level: 0,
      }));
      $httpBackend.verifyNoOutstandingExpectation();
      $httpBackend.verifyNoOutstandingRequest();
    });

    it('should be able to go to the next page', () => {
      makeController();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 0, 10, 511, $scope);
      $httpBackend.flush();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 1, 10, 511, $scope);
      $scope.nextPage();
      $httpBackend.flush();
      expect($scope.page).toEqual(1);
      expect($scope.pageSize).toEqual(10);
      expect($scope.totalPages).toEqual(511);
      expect($scope.gridOptions.data).toEqual(Array(10).fill({
        version: 'sys_perf_085ffeb310e8fed49739cf8443fcb13ea795d867',
        variant: 'linux-standalone',
        task: 'large_scale_model',
        test: 'HotCollectionDeleter.Delete.2.2',
        measurement: 'AverageSize',
        percent_change: '50.32',
        triage_status: 'untriaged',
        thread_level: 0,
      }));
      $httpBackend.verifyNoOutstandingExpectation();
      $httpBackend.verifyNoOutstandingRequest();
    });

    it('should get the first page on load', () => {
      makeController();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 0, 10, 511, $scope);
      $httpBackend.flush();
      expect($scope.page).toEqual(0);
      expect($scope.pageSize).toEqual(10);
      expect($scope.totalPages).toEqual(511);
      expect($scope.gridOptions.data).toEqual(Array(10).fill({
        version: 'sys_perf_085ffeb310e8fed49739cf8443fcb13ea795d867',
        variant: 'linux-standalone',
        task: 'large_scale_model',
        test: 'HotCollectionDeleter.Delete.2.2',
        measurement: 'AverageSize',
        percent_change: '50.32',
        triage_status: 'untriaged',
        thread_level: 0,
      }));
      $httpBackend.verifyNoOutstandingExpectation();
      $httpBackend.verifyNoOutstandingRequest();
    });

    it('should set default pagination variables', () => {
      makeController();
      expect($scope.page).toEqual(0);
      expect($scope.pageSize).toEqual(10);
      expect($scope.totalPages).toEqual(1);
    });

    it('should apply filtering to server calls', () => {
      makeController();
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 0, 10, 511, $scope);
      $httpBackend.flush();
      $scope.variantRegex = 'some_variant';
      $scope.versionRegex = 'some_version';
      $scope.taskRegex = 'some_task';
      $scope.testRegex = 'some_test';
      $scope.measurementRegex = 'some_measurement';
      $scope.threadLevels = [1,2,3];
      expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, 1, 10, 511, $scope);
      $scope.nextPage();
      $httpBackend.flush();
      $httpBackend.verifyNoOutstandingExpectation();
      $httpBackend.verifyNoOutstandingRequest();
    });

    it('should set up the grid, with defaults', () => {
      makeController();
      expect($scope.measurementRegex).toEqual('AverageLatency|Latency50thPercentile|Latency95thPercentile');
      delete $scope.gridOptions.onRegisterApi;
      expect($scope.gridOptions).toEqual({
        enableFiltering: true,
        enableRowSelection: true,
        enableSelectAll: true,
        enableSorting: false,
        selectionRowHeaderWidth: 35,
        useExternalFiltering: true,
        data: [],
        columnDefs: [
          {
            name: 'Percent Change',
            field: 'percent_change',
            type: 'number',
            cellTemplate: '<percent-change-cell row="row" ctx="grid.appScope.spvm.refCtx" />',
            width: CHANGE_POINTS_GRID.HAZARD_COL_WIDTH,
          },
          {
            name: 'Variant',
            field: 'variant',
            type: 'string',
          },
          {
            name: 'Task',
            field: 'task',
            type: 'string',
          },
          {
            name: 'Test',
            field: 'test',
            type: 'string',
          },
          {
            name: 'Version',
            field: 'version',
            type: 'string',
            cellTemplate: 'ui-grid-group-name',
            grouping: {
              groupPriority: 0,
            },
          },
          {
            name: 'Thread Level',
            field: 'thread_level',
            type: 'number',
          },
          {
            name: 'Measurement',
            field: 'measurement',
            type: 'string',
            filter: {
              term: 'AverageLatency|Latency50thPercentile|Latency95thPercentile'
            }
          },
          {
            name: 'Triage Status',
            field: 'triage_status',
          },
        ]
      });
    })
  });
});


function expectGetPage($httpBackend, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, page, pageSize, totalPages, $scope, newData) {
  let url = `${PERFORMANCE_ANALYSIS_AND_TRIAGE_API.BASE + PERFORMANCE_ANALYSIS_AND_TRIAGE_API.CHANGE_POINTS_BY_VERSION.replace("{projectId}", "some-project")}?page_size=${pageSize}&page=${page}`;
  if($scope.variantRegex) {
    url += `&variant_regex=${encodeURI($scope.variantRegex)}`
  }
  if($scope.versionRegex) {
    url += `&version_regex=${encodeURI($scope.versionRegex)}`
  }
  if($scope.taskRegex) {
    url += `&task_regex=${encodeURI($scope.taskRegex)}`
  }
  if($scope.testRegex) {
    url += `&test_regex=${encodeURI($scope.testRegex)}`
  }
  if($scope.measurementRegex) {
    url += `&measurement_regex=${encodeURI($scope.measurementRegex)}`
  }
  if($scope.threadLevels) {
    $scope.threadLevels.forEach(tl => {
      url += `&thread_levels=${tl}`
    })
  }
  return $httpBackend.expectGET(url).respond(200, JSON.stringify(newData || standardData(page, pageSize, totalPages)));
}

function standardData(page, pageSize, totalPages) {
  return {
    'versions': Array(10).fill(
      {
        'version_id': 'sys_perf_085ffeb310e8fed49739cf8443fcb13ea795d867',
        'change_points': [
          {
            "time_series_info": {
              "project": "sys-perf",
              "variant": "linux-standalone",
              "task": "large_scale_model",
              "test": "HotCollectionDeleter.Delete.2.2",
              "measurement": "AverageSize",
              "thread_level": 0
            },
            "cedar_perf_result_id": "7a8a54244e0bf868bf1d1edd2f388614aecb16bd",
            "version": "sys_perf_085ffeb310e8fed49739cf8443fcb13ea795d867",
            "order": 22151,
            "algorithm": {
              "name": "e_divisive_means",
              "version": 0,
              "options": [
                {
                  "name": "pvalue",
                  "value": 0.05
                },
                {
                  "name": "permutations",
                  "value": 100
                }
              ]
            },
            "triage": {
              'triaged_on': '0001-01-01T00:00:00Z',
              'triage_status': 'untriaged'
            },
            "percent_change": 50.3248234827374,
            "calculated_on": "2020-05-04T20:21:12.037000"
          }],
      }),
    'page': page,
    'page_size': pageSize,
    'total_pages': totalPages
  }
}