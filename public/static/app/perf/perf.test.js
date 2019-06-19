describe("PerfPluginTests", function () {
    beforeEach(module("MCI"));
    var $filter;
    beforeEach(inject(function(_$filter_) {
        $filter = _$filter_;
    }));

    let scope;
    let makeController;
    let ts;

    beforeEach(inject(function ($rootScope, $controller, $window, TestSample) {
      scope = $rootScope;
      $window.plugins = {perf:{}};
      makeController = () => $controller('PerfController', {
        $scope: scope,
        $window: $window
      });
      ts = TestSample;
    }));
  
    describe('controller', () => {
      it('should use canary exclusion regex to filter perfSample data', () => {
        scope.perfSample = new ts({
          data: {
            results: [{
              name:'insert_vector_primary_retry'
            },{
              name:'mixed_total_retry'
            },{
              name:'canary_server-cpuloop-10x'
            },{
              name:'canary_client-cpuloop-10x'
            },{
              name:'fio_iops_test_write_iops'
            },{
              name:'NetworkBandwidth'
            }]
          }
        });
        makeController();
        expect(scope.hideCanaries).toEqual(jasmine.any(Function));
        spyOn(scope, 'isCanary').and.callThrough();
        scope.hideCanaries();
        expect(scope.isCanary.calls.count()).toEqual(6);
        expect(scope.hiddenGraphs).toEqual({
          'canary_server-cpuloop-10x': true,          
          'canary_client-cpuloop-10x': true,          
          'fio_iops_test_write_iops': true,          
          'NetworkBandwidth': true       
        });
      });
  
    });
    it("expanded metrics data should be converted correctly to legacy data", function () {
        var converter = $filter("expandedMetricConverter");

        var testdata = [{
            "name": "test1",
            "info": {
                "project": "sys-perf",
                "version": "abc123",
                "task_name": "industry_benchmarks_secondary_reads",
                "task_id": "task1",
                "execution": 0,
                "test_name": "ycsb_100read_secondary_reads",
                "trial": 0,
                "parent": "",
                "tags": [
                    "tag1"
                ],
                "args": {
                    "thread_level": 32
                },
                "schema": 0
            },
            "create_at": "2019-02-22T22:11:38.823Z",
            "completed_at": "2019-02-22T22:16:24.308Z",
            "version": 0,
            "artifacts": null,
            "rollups": {
                "stats": [{
                        "name": "ops_per_sec",
                        "val": [{
                            "Key": "fl",
                            "Value": 70211.89951272942
                        }],
                        "version": 0,
                        "user": true
                    },
                    {
                        "name": "average_read_latency_us",
                        "val": [{
                            "Key": "fl",
                            "Value": 451.0556252
                        }],
                        "version": 0,
                        "user": true
                    },
                    {
                        "name": "95th_read_latency_us",
                        "val": [{
                            "Key": "fl",
                            "Value": 580
                        }],
                        "version": 0,
                        "user": true
                    },
                    {
                        "name": "99th_read_latency_us",
                        "val": [{
                            "Key": "fl",
                            "Value": 730
                        }],
                        "version": 0,
                        "user": true
                    }
                ],
                "processed_at": "2019-02-22T22:47:48.531Z",
                "count": 4,
                "valid": true
            },
            "total": null
        }];

        var convertedData = {
            "data": {
                "results": [{
                    "name": "ycsb_100read_secondary_reads",
                    "results": {
                        "32": {
                            "ops_per_sec": 70211.89951272942,
                            "ops_per_sec_values": [70211.89951272942],
                            "average_read_latency_us": 451.0556252,
                            "average_read_latency_us_values": [451.0556252],
                            "95th_read_latency_us": 580,
                            "95th_read_latency_us_values": [580],
                            "99th_read_latency_us": 730,
                            "99th_read_latency_us_values": [730]
                        }
                    },
                    "isExpandedMetric": true
                }]
            }
        };

        expect(converter(testdata)).toEqual(convertedData);
    })

    it("perf results should be merged correctly", function() {
        var merge = $filter("mergePerfResults");
        var firstSample = {
            "data": {
              "results": [
                {
                  "end": 1550873331.840443,
                  "name": "testone",
                  "results": {
                    "32": {
                      "ops_per_sec": 8,
                      "ops_per_sec_values": [
                        8
                      ]
                    }
                  },
                  "start": 1550873204.842642,
                },
                {
                  "end": 1550873784.308943,
                  "name": "testtwo",
                  "results": {
                    "32": {
                      "95th_read_latency_us": 8,
                      "95th_read_latency_us_values": [
                        8
                      ],
                      "99th_read_latency_us": 8,
                      "99th_read_latency_us_values": [
                        8
                      ],
                      "average_read_latency_us": 8,
                      "average_read_latency_us_values": [
                        8
                      ],
                      "ops_per_sec": 8,
                      "ops_per_sec_values": [
                        8
                      ]
                    }
                  },
                  "start": 1550873498.82311,
                },
                {
                  "end": 1550874227.721287,
                  "name": "ycsb_95read5update_secondary_reads",
                  "results": {
                    "32": {
                      "95th_read_latency_us": 8,
                      "95th_read_latency_us_values": [
                        8
                      ],
                      "99th_read_latency_us": 8,
                      "99th_read_latency_us_values": [
                        8
                      ],
                      "average_read_latency_us": 8,
                      "average_read_latency_us_values": [
                        8
                      ],
                      "ops_per_sec": 65820.21865476637,
                      "ops_per_sec_values": [
                        8
                      ]
                    }
                  },
                  "start": 1550873923.136368,
                  "workload": "ycsb"
                }
              ]
            },
            "tag": ""
          };
        var secondSample = {
            "name": "perf",
            "task_name": "industry_benchmarks_secondary_reads",
            "project_id": "sys-perf",
            "task_id": "task",
            "build_id": "build",
            "variant": "wtdevelop-3-node-replSet",
            "version_id": "version",
            "create_time": "2019-02-22T21:14:28Z",
            "is_patch": true,
            "order": 220,
            "revision": "abc123",
            "data": {
              "results": [
                {
                  "end": 1550873331.840443,
                  "name": "ycsb_load",
                  "results": {
                    "32": {
                      "ops_per_sec": 39569.79716521973,
                      "ops_per_sec_values": [
                        39569.79716521973
                      ]
                    }
                  },
                  "start": 1550873204.842642,
                  "workload": "ycsb"
                },
                {
                  "end": 1550873784.308943,
                  "name": "ycsb_100read_secondary_reads",
                  "results": {
                    "32": {
                      "95th_read_latency_us": 580,
                      "95th_read_latency_us_values": [
                        580
                      ],
                      "99th_read_latency_us": 730,
                      "99th_read_latency_us_values": [
                        730
                      ],
                      "average_read_latency_us": 451.0556252,
                      "average_read_latency_us_values": [
                        451.0556252
                      ],
                      "ops_per_sec": 70211.89951272942,
                      "ops_per_sec_values": [
                        70211.89951272942
                      ]
                    }
                  },
                  "start": 1550873498.82311,
                  "workload": "ycsb"
                },
                {
                  "end": 1550874227.721287,
                  "name": "ycsb_95read5update_secondary_reads",
                  "results": {
                    "32": {
                      "95th_read_latency_us": 700,
                      "95th_read_latency_us_values": [
                        700
                      ],
                      "99th_read_latency_us": 1150,
                      "99th_read_latency_us_values": [
                        1150
                      ],
                      "average_read_latency_us": 482.35296395318034,
                      "average_read_latency_us_values": [
                        482.35296395318034
                      ],
                      "ops_per_sec": 65820.21865476637,
                      "ops_per_sec_values": [
                        65820.21865476637
                      ]
                    }
                  },
                  "start": 1550873923.136368,
                  "workload": "ycsb"
                }
              ],
              "storageEngine": "wiredTiger"
            },
            "tag": ""
          };

          var merged = merge(firstSample, secondSample);
          expect(merged.task_name).toEqual("industry_benchmarks_secondary_reads");
          expect(merged.data.results).toContain({
            "end": 1550873784.308943,
            "name": "ycsb_100read_secondary_reads",
            "results": {
              "32": {
                "95th_read_latency_us": 580,
                "95th_read_latency_us_values": [
                  580
                ],
                "99th_read_latency_us": 730,
                "99th_read_latency_us_values": [
                  730
                ],
                "average_read_latency_us": 451.0556252,
                "average_read_latency_us_values": [
                  451.0556252
                ],
                "ops_per_sec": 70211.89951272942,
                "ops_per_sec_values": [
                  70211.89951272942
                ]
              }
            },
            "start": 1550873498.82311,
            "workload": "ycsb"
        });
        expect(merged.data.results).toContain({
            "end": 1550874227.721287,
            "name": "ycsb_95read5update_secondary_reads",
            "results": {
                "32": {
                "95th_read_latency_us": 8,
                "95th_read_latency_us_values": [
                    8
                ],
                "99th_read_latency_us": 8,
                "99th_read_latency_us_values": [
                    8
                ],
                "average_read_latency_us": 8,
                "average_read_latency_us_values": [
                    8
                ],
                "ops_per_sec": 65820.21865476637,
                "ops_per_sec_values": [
                    8
                ]
                }
            },
            "start": 1550873923.136368,
            "workload": "ycsb"
        });
        expect(merged.data.results).toContain({
            "end": 1550873331.840443,
            "name": "testone",
            "results": {
              "32": {
                "ops_per_sec": 8,
                "ops_per_sec_values": [
                  8
                ]
              }
            },
            "start": 1550873204.842642,
        });
    })
})
