describe("PerfPluginTests", function () {
  beforeEach(module("MCI"));
  var $filter;
  beforeEach(inject(function (_$filter_) {
    $filter = _$filter_;
  }));

  let scope;
  let makeController;
  let ts;

  beforeEach(inject(function ($rootScope, $controller, $window, TestSample) {
    scope = $rootScope;
    $window.plugins = { perf: {} };
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
            name: 'insert_vector_primary_retry'
          }, {
            name: 'mixed_total_retry'
          }, {
            name: 'canary_server-cpuloop-10x'
          }, {
            name: 'canary_client-cpuloop-10x'
          }, {
            name: 'fio_iops_test_write_iops'
          }, {
            name: 'NetworkBandwidth'
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

  it("perf results should be merged correctly", function () {
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
  });



});

describe('trendDataComplete', () => {
  beforeEach(module('MCI'));

  let $location;
  let TrendSamples;
  let trendDataComplete;
  let scope;
  let data = {};
  const legacy = [
    {
      "name" : "perf",
      "task_name" : "out_of_cache_scanner",
      "project_id" : "sys-perf",
      "task_id" : "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_780afd77baf061879b05e07b4022a81ab2470af7_19_08_12_19_52_46",
      "build_id" : "sys_perf_linux_1_node_15gbwtcache_780afd77baf061879b05e07b4022a81ab2470af7_19_08_12_19_52_46",
      "variant" : "linux-1-node-15gbwtcache",
      "version_id" : "sys_perf_780afd77baf061879b05e07b4022a81ab2470af7",
      "create_time" : "2019-08-12T19:52:46Z",
      "is_patch" : false,
      "order" : 18029,
      "revision" : "780afd77baf061879b05e07b4022a81ab2470af7",
      "data" : {
        "results" : [
          {
            "end" : 1565650340.602242,
            "name" : "ColdScanner6-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004483765342098279,
                "ops_per_sec_values" : [
                  0.004483765342098279
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "ColdScanner4-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004484294025079462,
                "ops_per_sec_values" : [
                  0.004484294025079462
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "Loader-TotalBulkInsert",
            "results" : {
              "10" : {
                "ops_per_sec" : 0.16710019873136994,
                "ops_per_sec_values" : [
                  0.16710019873136994
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "Loader-IndividualBulkInsert",
            "results" : {
              "10" : {
                "ops_per_sec" : 3.3479642481286067,
                "ops_per_sec_values" : [
                  3.3479642481286067
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "ColdScanner3-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004483542081737452,
                "ops_per_sec_values" : [
                  0.004483542081737452
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "OutOfCacheScanner-ActorFinished",
            "results" : {
              "1" : {
                "ops_per_sec" : 45549.59730015114,
                "ops_per_sec_values" : [
                  45549.59730015114
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "OutOfCacheScanner-Setup",
            "results" : {
              "1" : {
                "ops_per_sec" : 13.406166174175512,
                "ops_per_sec_values" : [
                  13.406166174175512
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "OutOfCacheScanner-ActorStarted",
            "results" : {
              "1" : {
                "ops_per_sec" : 39462.567661860805,
                "ops_per_sec_values" : [
                  39462.567661860805
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "ColdScanner5-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004485068249925737,
                "ops_per_sec_values" : [
                  0.004485068249925737
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "HotSampler-ReadWithScan.2",
            "results" : {
              "50" : {
                "ops_per_sec" : 66.60896680148633,
                "ops_per_sec_values" : [
                  66.60896680148633
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "HotSampler-Read.2",
            "results" : {
              "50" : {
                "ops_per_sec" : 42.03320015900397,
                "ops_per_sec_values" : [
                  42.03320015900397
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "ColdScanner-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.00448394902883802,
                "ops_per_sec_values" : [
                  0.00448394902883802
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565650340.602242,
            "name" : "ColdScanner2-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.00448526028878075,
                "ops_per_sec_values" : [
                  0.00448526028878075
                ]
              }
            },
            "start" : 1565647855.102672,
            "workload" : "genny"
          },
          {
            "end" : 1565652727.083737,
            "name" : "canary_server-cpuloop-10x",
            "results" : {
              "1" : {
                "ops_per_sec" : 5629.114041006716,
                "ops_per_sec_values" : [
                  5629.114041006716
                ]
              },
              "4" : {
                "ops_per_sec" : 21232.564057822907,
                "ops_per_sec_values" : [
                  21232.564057822907
                ]
              },
              "8" : {
                "ops_per_sec" : 39879.05112388881,
                "ops_per_sec_values" : [
                  39879.05112388881
                ]
              },
              "16" : {
                "ops_per_sec" : 74766.43363859564,
                "ops_per_sec_values" : [
                  74766.43363859564
                ]
              }
            },
            "start" : 1565652526.417266,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565652727.083737,
            "name" : "canary_client-cpuloop-1x",
            "results" : {
              "1" : {
                "ops_per_sec" : 120875.40892298323,
                "ops_per_sec_values" : [
                  120875.40892298323
                ]
              },
              "4" : {
                "ops_per_sec" : 479552.69968121406,
                "ops_per_sec_values" : [
                  479552.69968121406
                ]
              },
              "8" : {
                "ops_per_sec" : 942329.8722030352,
                "ops_per_sec_values" : [
                  942329.8722030352
                ]
              },
              "16" : {
                "ops_per_sec" : 1421187.542971923,
                "ops_per_sec_values" : [
                  1421187.542971923
                ]
              }
            },
            "start" : 1565652526.417266,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565652727.083737,
            "name" : "canary_client-cpuloop-10x",
            "results" : {
              "1" : {
                "ops_per_sec" : 15029.139525540008,
                "ops_per_sec_values" : [
                  15029.139525540008
                ]
              },
              "4" : {
                "ops_per_sec" : 60036.75237922007,
                "ops_per_sec_values" : [
                  60036.75237922007
                ]
              },
              "8" : {
                "ops_per_sec" : 119432.25692392104,
                "ops_per_sec_values" : [
                  119432.25692392104
                ]
              },
              "16" : {
                "ops_per_sec" : 223896.18550091176,
                "ops_per_sec_values" : [
                  223896.18550091176
                ]
              }
            },
            "start" : 1565652526.417266,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565652727.083737,
            "name" : "canary_server-sleep-10ms",
            "results" : {
              "1" : {
                "ops_per_sec" : 96.80194930554842,
                "ops_per_sec_values" : [
                  96.80194930554842
                ]
              },
              "4" : {
                "ops_per_sec" : 386.421693162394,
                "ops_per_sec_values" : [
                  386.421693162394
                ]
              },
              "8" : {
                "ops_per_sec" : 772.7110620056922,
                "ops_per_sec_values" : [
                  772.7110620056922
                ]
              },
              "16" : {
                "ops_per_sec" : 1545.7184209117231,
                "ops_per_sec_values" : [
                  1545.7184209117231
                ]
              }
            },
            "start" : 1565652526.417266,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565652727.083737,
            "name" : "canary_ping",
            "results" : {
              "1" : {
                "ops_per_sec" : 5830.245801283064,
                "ops_per_sec_values" : [
                  5830.245801283064
                ]
              },
              "4" : {
                "ops_per_sec" : 21687.33081459084,
                "ops_per_sec_values" : [
                  21687.33081459084
                ]
              },
              "8" : {
                "ops_per_sec" : 42217.91178918103,
                "ops_per_sec_values" : [
                  42217.91178918103
                ]
              },
              "16" : {
                "ops_per_sec" : 76947.70574170526,
                "ops_per_sec_values" : [
                  76947.70574170526
                ]
              }
            },
            "start" : 1565652526.417266,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565653224.028126,
            "name" : "fio_latency_test_write_clat_mean",
            "results" : {
              "1" : {
                "ops_per_sec" : 632.199646,
                "ops_per_sec_values" : [
                  632.199646
                ]
              }
            },
            "start" : 1565652763.558473,
            "workload" : "fio"
          },
          {
            "end" : 1565653224.028126,
            "name" : "fio_latency_test_read_clat_mean",
            "results" : {
              "1" : {
                "ops_per_sec" : 632.231445,
                "ops_per_sec_values" : [
                  632.231445
                ]
              }
            },
            "start" : 1565652763.558473,
            "workload" : "fio"
          },
          {
            "end" : 1565653224.028126,
            "name" : "fio_iops_test_write_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 2774.445557,
                "ops_per_sec_values" : [
                  2774.445557
                ]
              }
            },
            "start" : 1565652763.558473,
            "workload" : "fio"
          },
          {
            "end" : 1565653224.028126,
            "name" : "fio_iops_test_read_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 2770.447998,
                "ops_per_sec_values" : [
                  2770.447998
                ]
              }
            },
            "start" : 1565652763.558473,
            "workload" : "fio"
          },
          {
            "end" : 1565653224.028126,
            "name" : "fio_streaming_bandwidth_test_write_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 3779.682373,
                "ops_per_sec_values" : [
                  3779.682373
                ]
              }
            },
            "start" : 1565652763.558473,
            "workload" : "fio"
          },
          {
            "end" : 1565653224.028126,
            "name" : "fio_streaming_bandwidth_test_read_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 3788.19873,
                "ops_per_sec_values" : [
                  3788.19873
                ]
              }
            },
            "start" : 1565652763.558473,
            "workload" : "fio"
          },
          {
            "end" : 1565653333.900172,
            "name" : "NetworkBandwidth",
            "results" : {
              "1" : {
                "ops_per_sec" : 9328563378.187166,
                "ops_per_sec_values" : [
                  9328563378.187166
                ]
              }
            },
            "start" : 1565653270.871972,
            "workload" : "iperf"
          }
        ],
        "storageEngine" : "wiredTiger"
      },
      "tag" : ""
    },
    {
      "name" : "perf",
      "task_name" : "out_of_cache_scanner",
      "project_id" : "sys-perf",
      "task_id" : "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_20ba91db04c0b7b3d10fe2527b6938b1a14fcaa6_19_08_12_20_24_04",
      "build_id" : "sys_perf_linux_1_node_15gbwtcache_20ba91db04c0b7b3d10fe2527b6938b1a14fcaa6_19_08_12_20_24_04",
      "variant" : "linux-1-node-15gbwtcache",
      "version_id" : "sys_perf_20ba91db04c0b7b3d10fe2527b6938b1a14fcaa6",
      "create_time" : "2019-08-12T20:24:04Z",
      "is_patch" : false,
      "order" : 18031,
      "revision" : "20ba91db04c0b7b3d10fe2527b6938b1a14fcaa6",
      "data" : {
        "results" : [
          {
            "end" : 1565654517.78871,
            "name" : "ColdScanner6-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004481365033009911,
                "ops_per_sec_values" : [
                  0.004481365033009911
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "ColdScanner4-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004482858010896038,
                "ops_per_sec_values" : [
                  0.004482858010896038
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "Loader-TotalBulkInsert",
            "results" : {
              "10" : {
                "ops_per_sec" : 0.1639478562837463,
                "ops_per_sec_values" : [
                  0.1639478562837463
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "Loader-IndividualBulkInsert",
            "results" : {
              "10" : {
                "ops_per_sec" : 3.2846815790131756,
                "ops_per_sec_values" : [
                  3.2846815790131756
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "ColdScanner3-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004481116297310688,
                "ops_per_sec_values" : [
                  0.004481116297310688
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "OutOfCacheScanner-ActorFinished",
            "results" : {
              "1" : {
                "ops_per_sec" : 45335.74208084187,
                "ops_per_sec_values" : [
                  45335.74208084187
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "OutOfCacheScanner-Setup",
            "results" : {
              "1" : {
                "ops_per_sec" : 13.413118136842778,
                "ops_per_sec_values" : [
                  13.413118136842778
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "OutOfCacheScanner-ActorStarted",
            "results" : {
              "1" : {
                "ops_per_sec" : 39489.91002882165,
                "ops_per_sec_values" : [
                  39489.91002882165
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "ColdScanner5-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.0044818374976988784,
                "ops_per_sec_values" : [
                  0.0044818374976988784
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "HotSampler-ReadWithScan.2",
            "results" : {
              "50" : {
                "ops_per_sec" : 56.87437485733593,
                "ops_per_sec_values" : [
                  56.87437485733593
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "HotSampler-Read.2",
            "results" : {
              "50" : {
                "ops_per_sec" : 34.604349641276,
                "ops_per_sec_values" : [
                  34.604349641276
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "ColdScanner-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004483085068481749,
                "ops_per_sec_values" : [
                  0.004483085068481749
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565654517.78871,
            "name" : "ColdScanner2-Scan.2",
            "results" : {
              "1" : {
                "ops_per_sec" : 0.004483784856749016,
                "ops_per_sec_values" : [
                  0.004483784856749016
                ]
              }
            },
            "start" : 1565652022.424066,
            "workload" : "genny"
          },
          {
            "end" : 1565656957.911434,
            "name" : "canary_server-cpuloop-10x",
            "results" : {
              "1" : {
                "ops_per_sec" : 5592.443306852216,
                "ops_per_sec_values" : [
                  5592.443306852216
                ]
              },
              "4" : {
                "ops_per_sec" : 21439.76017163168,
                "ops_per_sec_values" : [
                  21439.76017163168
                ]
              },
              "8" : {
                "ops_per_sec" : 40929.163497665424,
                "ops_per_sec_values" : [
                  40929.163497665424
                ]
              },
              "16" : {
                "ops_per_sec" : 74585.58536308071,
                "ops_per_sec_values" : [
                  74585.58536308071
                ]
              }
            },
            "start" : 1565656757.21953,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565656957.911434,
            "name" : "canary_client-cpuloop-1x",
            "results" : {
              "1" : {
                "ops_per_sec" : 120824.9153333528,
                "ops_per_sec_values" : [
                  120824.9153333528
                ]
              },
              "4" : {
                "ops_per_sec" : 476049.31639829144,
                "ops_per_sec_values" : [
                  476049.31639829144
                ]
              },
              "8" : {
                "ops_per_sec" : 938054.7733708782,
                "ops_per_sec_values" : [
                  938054.7733708782
                ]
              },
              "16" : {
                "ops_per_sec" : 1486506.279644098,
                "ops_per_sec_values" : [
                  1486506.279644098
                ]
              }
            },
            "start" : 1565656757.21953,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565656957.911434,
            "name" : "canary_client-cpuloop-10x",
            "results" : {
              "1" : {
                "ops_per_sec" : 15002.111917212846,
                "ops_per_sec_values" : [
                  15002.111917212846
                ]
              },
              "4" : {
                "ops_per_sec" : 59861.745286023426,
                "ops_per_sec_values" : [
                  59861.745286023426
                ]
              },
              "8" : {
                "ops_per_sec" : 118922.6101122872,
                "ops_per_sec_values" : [
                  118922.6101122872
                ]
              },
              "16" : {
                "ops_per_sec" : 221566.22861728215,
                "ops_per_sec_values" : [
                  221566.22861728215
                ]
              }
            },
            "start" : 1565656757.21953,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565656957.911434,
            "name" : "canary_server-sleep-10ms",
            "results" : {
              "1" : {
                "ops_per_sec" : 96.72005120567339,
                "ops_per_sec_values" : [
                  96.72005120567339
                ]
              },
              "4" : {
                "ops_per_sec" : 386.373475775622,
                "ops_per_sec_values" : [
                  386.373475775622
                ]
              },
              "8" : {
                "ops_per_sec" : 772.7776100673663,
                "ops_per_sec_values" : [
                  772.7776100673663
                ]
              },
              "16" : {
                "ops_per_sec" : 1545.0676360425211,
                "ops_per_sec_values" : [
                  1545.0676360425211
                ]
              }
            },
            "start" : 1565656757.21953,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565656957.911434,
            "name" : "canary_ping",
            "results" : {
              "1" : {
                "ops_per_sec" : 5849.106374859983,
                "ops_per_sec_values" : [
                  5849.106374859983
                ]
              },
              "4" : {
                "ops_per_sec" : 22310.466628295282,
                "ops_per_sec_values" : [
                  22310.466628295282
                ]
              },
              "8" : {
                "ops_per_sec" : 42125.12357700831,
                "ops_per_sec_values" : [
                  42125.12357700831
                ]
              },
              "16" : {
                "ops_per_sec" : 77318.17383250255,
                "ops_per_sec_values" : [
                  77318.17383250255
                ]
              }
            },
            "start" : 1565656757.21953,
            "workload" : "mongoshell"
          },
          {
            "end" : 1565657462.026578,
            "name" : "fio_latency_test_write_clat_mean",
            "results" : {
              "1" : {
                "ops_per_sec" : 695.134949,
                "ops_per_sec_values" : [
                  695.134949
                ]
              }
            },
            "start" : 1565657006.207802,
            "workload" : "fio"
          },
          {
            "end" : 1565657462.026578,
            "name" : "fio_latency_test_read_clat_mean",
            "results" : {
              "1" : {
                "ops_per_sec" : 684.882446,
                "ops_per_sec_values" : [
                  684.882446
                ]
              }
            },
            "start" : 1565657006.207802,
            "workload" : "fio"
          },
          {
            "end" : 1565657462.026578,
            "name" : "fio_iops_test_write_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 2765.7146,
                "ops_per_sec_values" : [
                  2765.7146
                ]
              }
            },
            "start" : 1565657006.207802,
            "workload" : "fio"
          },
          {
            "end" : 1565657462.026578,
            "name" : "fio_iops_test_read_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 2762.099854,
                "ops_per_sec_values" : [
                  2762.099854
                ]
              }
            },
            "start" : 1565657006.207802,
            "workload" : "fio"
          },
          {
            "end" : 1565657462.026578,
            "name" : "fio_streaming_bandwidth_test_write_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 3615.132812,
                "ops_per_sec_values" : [
                  3615.132812
                ]
              }
            },
            "start" : 1565657006.207802,
            "workload" : "fio"
          },
          {
            "end" : 1565657462.026578,
            "name" : "fio_streaming_bandwidth_test_read_iops",
            "results" : {
              "1" : {
                "ops_per_sec" : 3622.999023,
                "ops_per_sec_values" : [
                  3622.999023
                ]
              }
            },
            "start" : 1565657006.207802,
            "workload" : "fio"
          },
          {
            "end" : 1565657571.186898,
            "name" : "NetworkBandwidth",
            "results" : {
              "1" : {
                "ops_per_sec" : 9404023292.44925,
                "ops_per_sec_values" : [
                  9404023292.44925
                ]
              }
            },
            "start" : 1565657508.170817,
            "workload" : "iperf"
          }
        ],
        "storageEngine" : "wiredTiger"
      },
      "tag" : ""
    }
  ];
  const cedar = [
    {
      "data" : {
        "results" : [
          {
            "name" : "ColdScanner-Scan.2",
            "isExpandedMetric" : true,
            "results" : {
              "6" : {
                "AverageLatency" : 5059.062329521759,
                "AverageLatency_values" : [
                  5059.062329521759
                ],
                "AverageSize" : 130,
                "AverageSize_values" : [
                  130
                ],
                "OperationThroughput" : 2279221.519227766,
                "OperationThroughput_values" : [
                  2279221.519227766
                ],
                "SizeThroughput" : 296298797.4996096,
                "SizeThroughput_values" : [
                  296298797.4996096
                ],
                "ErrorRate" : 0,
                "ErrorRate_values" : [
                  0
                ],
                "Latency50thPercentile" : 641941806225.5,
                "Latency50thPercentile_values" : [
                  641941806225.5
                ],
                "Latency80thPercentile" : 755805805593,
                "Latency80thPercentile_values" : [
                  755805805593
                ],
                "Latency90thPercentile" : 995912516158.3334,
                "Latency90thPercentile_values" : [
                  995912516158.3334
                ],
                "Latency95thPercentile" : 995925590308.5,
                "Latency95thPercentile_values" : [
                  995925590308.5
                ],
                "Latency99thPercentile" : 995929220065,
                "Latency99thPercentile_values" : [
                  995929220065
                ],
                "WorkersMin" : 6,
                "WorkersMin_values" : [
                  6
                ],
                "WorkersMax" : 6,
                "WorkersMax_values" : [
                  6
                ],
                "LatencyMin" : 303709401504,
                "LatencyMin_values" : [
                  303709401504
                ],
                "LatencyMax" : 995929220065,
                "LatencyMax_values" : [
                  995929220065
                ],
                "DurationTotal" : 947692000000,
                "DurationTotal_values" : [
                  947692000000
                ],
                "ErrorsTotal" : 0,
                "ErrorsTotal_values" : [
                  0
                ],
                "OperationsTotal" : 2160000000,
                "OperationsTotal_values" : [
                  2160000000
                ],
                "SizeTotal" : 280800000000,
                "SizeTotal_values" : [
                  280800000000
                ],
                "OverheadTotal" : 12046884880,
                "OverheadTotal_values" : [
                  12046884880
                ]
              }
            }
          }
        ]
      },
      "create_time" : "2019-09-10T12:03:28.000Z",
      "order" : 18426,
      "version_id" : "sys_perf_af71c266dae464807509962e6f12d5f2d9a61f73",
      "project_id" : "sys-perf",
      "task_name" : "out_of_cache_scanner",
      "task_id" : "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_af71c266dae464807509962e6f12d5f2d9a61f73_19_09_10_01_08_02",
      "revision" : "af71c266dae464807509962e6f12d5f2d9a61f73"
    },
    {
      "data" : {
        "results" : [
          {
            "name" : "ColdScanner-Scan.2",
            "isExpandedMetric" : true,
            "results" : {
              "6" : {
                "AverageLatency" : 7126.839776655555,
                "AverageLatency_values" : [
                  7126.839776655555
                ],
                "AverageSize" : 130,
                "AverageSize_values" : [
                  130
                ],
                "OperationThroughput" : 2324515.2417173,
                "OperationThroughput_values" : [
                  2324515.2417173
                ],
                "SizeThroughput" : 302186981.423249,
                "SizeThroughput_values" : [
                  302186981.423249
                ],
                "ErrorRate" : 0,
                "ErrorRate_values" : [
                  0
                ],
                "Latency50thPercentile" : 794905100227.5,
                "Latency50thPercentile_values" : [
                  794905100227.5
                ],
                "Latency80thPercentile" : 1212269512858.6,
                "Latency80thPercentile_values" : [
                  1212269512858.6
                ],
                "Latency90thPercentile" : 1212291855794.8,
                "Latency90thPercentile_values" : [
                  1212291855794.8
                ],
                "Latency95thPercentile" : 1212316984899,
                "Latency95thPercentile_values" : [
                  1212316984899
                ],
                "Latency99thPercentile" : 1212316984899,
                "Latency99thPercentile_values" : [
                  1212316984899
                ],
                "WorkersMin" : 6,
                "WorkersMin_values" : [
                  6
                ],
                "WorkersMax" : 6,
                "WorkersMax_values" : [
                  6
                ],
                "LatencyMin" : 618443157929,
                "LatencyMin_values" : [
                  618443157929
                ],
                "LatencyMax" : 1212316984899,
                "LatencyMax_values" : [
                  1212316984899
                ],
                "DurationTotal" : 619484000000,
                "DurationTotal_values" : [
                  619484000000
                ],
                "ErrorsTotal" : 0,
                "ErrorsTotal_values" : [
                  0
                ],
                "OperationsTotal" : 1440000000,
                "OperationsTotal_values" : [
                  1440000000
                ],
                "SizeTotal" : 187200000000,
                "SizeTotal_values" : [
                  187200000000
                ],
                "OverheadTotal" : 6006525996,
                "OverheadTotal_values" : [
                  6006525996
                ]
              }
            }
          }
        ]
      },
      "create_time" : "2019-09-09T11:31:11.000Z",
      "order" : 18411,
      "version_id" : "sys_perf_96a1a78772038fa29b94dbe92b51b9e9957d3f6f",
      "project_id" : "sys-perf",
      "task_name" : "out_of_cache_scanner",
      "variant" : "linux-1-node-15gbwtcache",
      "task_id" : "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_96a1a78772038fa29b94dbe92b51b9e9957d3f6f_19_09_08_15_12_44",
      "revision" : "96a1a78772038fa29b94dbe92b51b9e9957d3f6f"
    }
  ];

  beforeEach(() => {
    inject(( $compile, $injector) => {
      $location = $injector.get('$location');
      trendDataComplete =  $injector.get('trendDataComplete');
    });
  });

  beforeEach(() => {
    scope = {
      trendResults:[],
      outliers: {rejects:[]},
      metricSelect:{options:[],
        default:{key: 'ops_per_sec'}},
      conf:{enabled: false},
      perfSample:{sample:{
          "name" : "perf",
          "task_name" : "latency-benchmark",
          "project_id" : "mongohouse",
          "task_id" : "mongohouse_archlinux_latency_benchmark_patch_63a5a59830a1e0a8614b12cbff62cca065bfcad5_5d6ff50257e85a69060aa156_19_09_04_17_31_51",
          "build_id" : "mongohouse_archlinux_patch_63a5a59830a1e0a8614b12cbff62cca065bfcad5_5d6ff50257e85a69060aa156_19_09_04_17_31_51",
          "variant" : "archlinux",
          "version_id" : "5d6ff50257e85a69060aa156",
          "create_time" : "2019-08-21T17:01:19Z",
          "is_patch" : true,
          "order" : 595,
          "revision" : "63a5a59830a1e0a8614b12cbff62cca065bfcad5",
          "data" : {
            "results" : [
              {
                "name" : "simple_starter_benchmark_2",
                "results" : {
                  "1" : {
                    "ops_per_sec" : 797,
                    "ops_per_sec_values" : [
                      797
                    ]
                  }
                }
              },
              {
                "name" : "simple_starter_benchmark_1",
                "results" : {
                  "1" : {
                    "ops_per_sec" : 799,
                    "ops_per_sec_values" : [
                      799
                    ]
                  }
                }
              }
            ]
          },
          "tag" : ""
        }
      }
    };
  });

  it('should handle no data', () => {
    expect(trendDataComplete(scope, data)).toBe(data);

    expect(scope.trendResults).toEqual([scope.perfSample.sample]);
    expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
    expect(scope.allTrendSamples).toBe(scope.filteredTrendSamples);
    expect(scope.metricSelect.options).toEqual([]);
    expect(scope.metricSelect.value).toBeUndefined();
  });

  describe('legacy', () => {
    it('should not filter unmatched rejects', () => {
      data = {legacy};
      scope.outliers.rejects =['task_id 2'];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).toBe(scope.filteredTrendSamples);
      expect(scope.metricSelect.options).toEqual([]);
      expect(scope.metricSelect.value).toBeUndefined();
    });

    it('should not filter whitelisted rejects', () => {
      data = {legacy};
      scope.whitelist= [{revision:'780afd77baf061879b05e07b4022a81ab2470af7', project:'sys-perf', variant:'linux-1-node-15gbwtcache', task:'out_of_cache_scanner'}];
      scope.outliers.rejects =['sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_780afd77baf061879b05e07b4022a81ab2470af7_19_08_12_19_52_46'];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).toBe(scope.filteredTrendSamples);
      expect(scope.metricSelect.options).toEqual([
        {
          "key": "threadLevel",
          "name": "threadLevel"
        }
      ]);
      expect(scope.metricSelect.value).toBeUndefined();
    });

    it('should filter rejects', () => {
      data = {legacy};
      // scope.outliers.rejects =[{task_id: 'task_id 1', revision:'revision 1', project:'project', variant:'variant', task:'task'}];
      scope.outliers.rejects =['sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_780afd77baf061879b05e07b4022a81ab2470af7_19_08_12_19_52_46'];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).not.toBe(scope.filteredTrendSamples);
      expect(scope.filteredTrendSamples.samples.length).toBe(2);
      expect(scope.filteredTrendSamples.samples[0].task_id).toBe('mongohouse_archlinux_latency_benchmark_patch_63a5a59830a1e0a8614b12cbff62cca065bfcad5_5d6ff50257e85a69060aa156_19_09_04_17_31_51');
      expect(scope.filteredTrendSamples.samples[1].task_id).toBe('sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_20ba91db04c0b7b3d10fe2527b6938b1a14fcaa6_19_08_12_20_24_04');
      expect(scope.metricSelect.options).toEqual([
        {
          "key": "threadLevel",
          "name": "threadLevel"
        }
      ]);
      expect(scope.metricSelect.value).toBeUndefined();
    });
  });

  describe('cedar', () => {
    it('should not filter unmatched rejects', () => {
      data = {cedar};
      scope.outliers.rejects =['task_id 2'];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).toBe(scope.filteredTrendSamples);
      expect(scope.metricSelect.options).toEqual([
        {
          "key": "AverageLatency",
          "name": "AverageLatency"
        },
        {
          "key": "AverageSize",
          "name": "AverageSize"
        },
        {
          "key": "OperationThroughput",
          "name": "OperationThroughput"
        },
        {
          "key": "SizeThroughput",
          "name": "SizeThroughput"
        },
        {
          "key": "ErrorRate",
          "name": "ErrorRate"
        },
        {
          "key": "Latency50thPercentile",
          "name": "Latency50thPercentile"
        },
        {
          "key": "Latency80thPercentile",
          "name": "Latency80thPercentile"
        },
        {
          "key": "Latency90thPercentile",
          "name": "Latency90thPercentile"
        },
        {
          "key": "Latency95thPercentile",
          "name": "Latency95thPercentile"
        },
        {
          "key": "Latency99thPercentile",
          "name": "Latency99thPercentile"
        },
        {
          "key": "WorkersMin",
          "name": "WorkersMin"
        },
        {
          "key": "WorkersMax",
          "name": "WorkersMax"
        },
        {
          "key": "LatencyMin",
          "name": "LatencyMin"
        },
        {
          "key": "LatencyMax",
          "name": "LatencyMax"
        },
        {
          "key": "DurationTotal",
          "name": "DurationTotal"
        },
        {
          "key": "ErrorsTotal",
          "name": "ErrorsTotal"
        },
        {
          "key": "OperationsTotal",
          "name": "OperationsTotal"
        },
        {
          "key": "SizeTotal",
          "name": "SizeTotal"
        },
        {
          "key": "OverheadTotal",
          "name": "OverheadTotal"
        }
      ]);
      expect(scope.metricSelect.value).toBeUndefined();
    });

    it('should not filter whitelisted rejects', () => {
      data = {cedar};
      scope.whitelist= [{revision:'af71c266dae464807509962e6f12d5f2d9a61f73', project:'sys-perf', variant:'linux-1-node-15gbwtcache',task:'out_of_cache_scanner'}];
      // scope.outliers.rejects =[{task_id: 'task_id 1', revision:'revision 1', project:'project', variant:'variant', task:'task'}];
      scope.outliers.rejects =['sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_af71c266dae464807509962e6f12d5f2d9a61f73_19_09_10_01_08_02'];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).toBe(scope.filteredTrendSamples);
      const expected = [
        {
          "key": "AverageLatency",
          "name": "AverageLatency"
        },
        {
          "key": "AverageSize",
          "name": "AverageSize"
        },
        {
          "key": "OperationThroughput",
          "name": "OperationThroughput"
        },
        {
          "key": "SizeThroughput",
          "name": "SizeThroughput"
        },
        {
          "key": "ErrorRate",
          "name": "ErrorRate"
        },
        {
          "key": "Latency50thPercentile",
          "name": "Latency50thPercentile"
        },
        {
          "key": "Latency80thPercentile",
          "name": "Latency80thPercentile"
        },
        {
          "key": "Latency90thPercentile",
          "name": "Latency90thPercentile"
        },
        {
          "key": "Latency95thPercentile",
          "name": "Latency95thPercentile"
        },
        {
          "key": "Latency99thPercentile",
          "name": "Latency99thPercentile"
        },
        {
          "key": "WorkersMin",
          "name": "WorkersMin"
        },
        {
          "key": "WorkersMax",
          "name": "WorkersMax"
        },
        {
          "key": "LatencyMin",
          "name": "LatencyMin"
        },
        {
          "key": "LatencyMax",
          "name": "LatencyMax"
        },
        {
          "key": "DurationTotal",
          "name": "DurationTotal"
        },
        {
          "key": "ErrorsTotal",
          "name": "ErrorsTotal"
        },
        {
          "key": "OperationsTotal",
          "name": "OperationsTotal"
        },
        {
          "key": "SizeTotal",
          "name": "SizeTotal"
        },
        {
          "key": "OverheadTotal",
          "name": "OverheadTotal"
        },
        {
          "key": "threadLevel",
          "name": "threadLevel"
        }
      ];
      expect(scope.metricSelect.options).toEqual(expected);
      expect(scope.metricSelect.value).toBeUndefined();
    });

    it('should filter rejects', () => {
      data = {cedar};
      // scope.outliers.rejects =[{task_id: 'task_id 1', revision:'revision 1', project:'project', variant:'variant', task:'task'}];
      scope.outliers.rejects =['sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_af71c266dae464807509962e6f12d5f2d9a61f73_19_09_10_01_08_02'];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).not.toBe(scope.filteredTrendSamples);
      expect(scope.filteredTrendSamples.samples.length).toBe(2);
      expect(scope.filteredTrendSamples.samples[0].task_id).toBe('mongohouse_archlinux_latency_benchmark_patch_63a5a59830a1e0a8614b12cbff62cca065bfcad5_5d6ff50257e85a69060aa156_19_09_04_17_31_51');
      expect(scope.filteredTrendSamples.samples[1].task_id).toBe('sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_96a1a78772038fa29b94dbe92b51b9e9957d3f6f_19_09_08_15_12_44');
      const expected = [
        {
          "key": "AverageLatency",
          "name": "AverageLatency"
        },
        {
          "key": "AverageSize",
          "name": "AverageSize"
        },
        {
          "key": "OperationThroughput",
          "name": "OperationThroughput"
        },
        {
          "key": "SizeThroughput",
          "name": "SizeThroughput"
        },
        {
          "key": "ErrorRate",
          "name": "ErrorRate"
        },
        {
          "key": "Latency50thPercentile",
          "name": "Latency50thPercentile"
        },
        {
          "key": "Latency80thPercentile",
          "name": "Latency80thPercentile"
        },
        {
          "key": "Latency90thPercentile",
          "name": "Latency90thPercentile"
        },
        {
          "key": "Latency95thPercentile",
          "name": "Latency95thPercentile"
        },
        {
          "key": "Latency99thPercentile",
          "name": "Latency99thPercentile"
        },
        {
          "key": "WorkersMin",
          "name": "WorkersMin"
        },
        {
          "key": "WorkersMax",
          "name": "WorkersMax"
        },
        {
          "key": "LatencyMin",
          "name": "LatencyMin"
        },
        {
          "key": "LatencyMax",
          "name": "LatencyMax"
        },
        {
          "key": "DurationTotal",
          "name": "DurationTotal"
        },
        {
          "key": "ErrorsTotal",
          "name": "ErrorsTotal"
        },
        {
          "key": "OperationsTotal",
          "name": "OperationsTotal"
        },
        {
          "key": "SizeTotal",
          "name": "SizeTotal"
        },
        {
          "key": "OverheadTotal",
          "name": "OverheadTotal"
        },
        {
          "key": "threadLevel",
          "name": "threadLevel"
        }
      ];
      expect(scope.metricSelect.options).toEqual(expected);
      expect(scope.metricSelect.value).toBeUndefined();
    });
  });

  describe('both', () => {
    it('should not filter unmatched rejects', () => {
      data = {legacy, cedar};
      scope.outliers.rejects =[];
      expect(trendDataComplete(scope, data)).toBe(data);

      expect(scope.allTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.filteredTrendSamples).toEqual(jasmine.anything(TrendSamples));
      expect(scope.allTrendSamples).toBe(scope.filteredTrendSamples);
      const expected = [
        {
          "key": "threadLevel",
          "name": "threadLevel"
        },
        {
          "key": "AverageLatency",
          "name": "AverageLatency"
        },
        {
          "key": "AverageSize",
          "name": "AverageSize"
        },
        {
          "key": "OperationThroughput",
          "name": "OperationThroughput"
        },
        {
          "key": "SizeThroughput",
          "name": "SizeThroughput"
        },
        {
          "key": "ErrorRate",
          "name": "ErrorRate"
        },
        {
          "key": "Latency50thPercentile",
          "name": "Latency50thPercentile"
        },
        {
          "key": "Latency80thPercentile",
          "name": "Latency80thPercentile"
        },
        {
          "key": "Latency90thPercentile",
          "name": "Latency90thPercentile"
        },
        {
          "key": "Latency95thPercentile",
          "name": "Latency95thPercentile"
        },
        {
          "key": "Latency99thPercentile",
          "name": "Latency99thPercentile"
        },
        {
          "key": "WorkersMin",
          "name": "WorkersMin"
        },
        {
          "key": "WorkersMax",
          "name": "WorkersMax"
        },
        {
          "key": "LatencyMin",
          "name": "LatencyMin"
        },
        {
          "key": "LatencyMax",
          "name": "LatencyMax"
        },
        {
          "key": "DurationTotal",
          "name": "DurationTotal"
        },
        {
          "key": "ErrorsTotal",
          "name": "ErrorsTotal"
        },
        {
          "key": "OperationsTotal",
          "name": "OperationsTotal"
        },
        {
          "key": "SizeTotal",
          "name": "SizeTotal"
        },
        {
          "key": "OverheadTotal",
          "name": "OverheadTotal"
        }
      ];
      expect(scope.metricSelect.options).toEqual(expected);
      expect(scope.metricSelect.value).toBeUndefined();
    });
  });
});

describe('loadWhitelist', () => {
  beforeEach(module('MCI'));

  let PointsDataService;
  let WhitelistDataService;
  let loadWhitelist;
  const points = {};
  const whitelist = {};
  const resp = {};

  let getOutlierPointsQSpy;
  let getWhitelistQSpy;
  let allSpy;

  beforeEach(() => {
    inject(($injector, $q) => {
      PointsDataService = $injector.get('PointsDataService');
      getOutlierPointsQSpy = spyOn(PointsDataService, 'getOutlierPointsQ').and.returnValue(points);
      WhitelistDataService =  $injector.get('WhitelistDataService');
      getWhitelistQSpy = spyOn(WhitelistDataService, 'getWhitelistQ').and.returnValue(whitelist);
      allSpy = spyOn($q, 'all').and.returnValue(resp);
      loadWhitelist =  $injector.get('loadWhitelist');
    });
  });

  it('should get points and whitelist ', () => {
    expect(loadWhitelist('project', 'variant', 'task')).toBe(resp);
    expect(getOutlierPointsQSpy).toHaveBeenCalledWith('project', 'variant', 'task');
    expect(getWhitelistQSpy).toHaveBeenCalledWith({project:'project', variant:'variant', task:'task'});
    expect(allSpy).toHaveBeenCalledWith({points, whitelist});
  });
});

describe('loadTrendData', () => {
  beforeEach(module('MCI'));

  let loadTrendData;
  let $http;
  let getSpy;
  let cedarSpy;
  let project = 'sys-perf';
  let variant = 'standalone-linux';
  let task_name = 'bestbuy_agg';
  let taskId = 'sys_perf_standalone_linux_bestbuy_agg_9129d4db52acea797f866a68d28de9f60c1206f6_18_07_28_03_10_45';
  let legacyUrl = '/plugin/json/history/' + taskId+ '/perf';
  let cedarUrl = '/rest/v1/perf/task_name/bestbuy_agg?variant=standalone-linux&project=sys-perf';
  const scope= {task:{id:taskId}};
  const cedarPromise= {
    then: () => cedarPromise,
    catch: () => cedarPromise
  };
  const legacyPromise = {
    then: () => legacyPromise,
    catch: () => legacyPromise
  };
  const ApiUtil = {
    cedarAPI: () => cedarPromise
  };

  beforeEach(() => {
    module($provide => {
      $provide.value('ApiUtil', ApiUtil);
    });

    inject(($injector, _$http_) => {
      loadTrendData = $injector.get('loadTrendData');
      $http = _$http_;
      getSpy = spyOn($http, 'get').and.returnValue(legacyPromise);
      cedarSpy = spyOn(ApiUtil, 'cedarAPI').and.returnValue(cedarPromise);
    });
  });

  it('should get both urls', () => {
    loadTrendData(scope, project, variant, task_name);
    expect(getSpy).toHaveBeenCalledWith(legacyUrl);
    expect(cedarSpy).toHaveBeenCalledWith(cedarUrl);
  });
});

describe('loadChangePoints', function () {
  beforeEach(module('MCI'));

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';
  const scope = {};

  let loadChangePoints;
  let $q;
  let loadUnprocessedSpy;
  let loadProcessedSpy;
  let allSpy;

  let processed;
  let unprocessed;
  let resp;

  describe('no processed or unprocessed', () => {
    beforeEach(() => {
      module($provide => {
        loadProcessedSpy = jasmine.createSpy('loadProcessed').and.returnValue([]);
        loadUnprocessedSpy = jasmine.createSpy('loadUnprocessed').and.returnValue([]);

        $provide.value('loadProcessed', loadProcessedSpy);
        $provide.value('loadUnprocessed', loadUnprocessedSpy);
      });

      inject($injector => {
        $q = $injector.get('$q');
        loadChangePoints = $injector.get('loadChangePoints');
      });
      processed = [];
      unprocessed = [];
      resp  = {processed, unprocessed};
      allSpy = spyOn($q, 'all').and.returnValue({
        then: (func) => func({processed:[], unprocessed:[]})
      });
    });

    it('should be handled correctly', function () {
      const value = loadChangePoints(scope, project, variant, task);

      expect(value).toEqual({});
      expect(scope.changePoints).toEqual({});

      expect(loadUnprocessedSpy).toHaveBeenCalledWith(project, variant, task);
      expect(loadProcessedSpy).toHaveBeenCalledWith(project, variant, task);
      expect(allSpy).toHaveBeenCalledWith(resp);
    });

  });

  describe('some processed or unprocessed', () => {
    beforeEach(() => {
      processed = [
        {
          "test" : "InsertRemove.InsertRemoveTest.remove",
          "suspect_revision" : "8029241ffa616d27502f9396c50a85b95115beaf",
        },
        {
          "test" : "InsertRemove.dummy_inserts",
          "suspect_revision" : "b6147f664cc22c360611b05f79c0d8febf3c7692",
        }
      ];
      unprocessed = [
        {
          "test" : "InsertRemove.Genny.Setup",
          "suspect_revision" : "03e13f90426a82a97cbb0f926385e09904519259",
        },
        {
          "test" : "InsertRemove.Genny.Setup",
          "suspect_revision" : "6efa4ed0820b6f6e3a2615dc5f42e13ce3415ad8",
        },
        {
          "test" : "InsertRemove.Genny.Setup",
          "suspect_revision" : "2c3845a00763f8e6b2ccae76ba4ea7c1434450df",
        }
      ];
      resp  = {processed, unprocessed};
      module($provide => {
        loadProcessedSpy = jasmine.createSpy('loadProcessed').and.returnValue(processed);
        loadUnprocessedSpy = jasmine.createSpy('loadUnprocessed').and.returnValue(unprocessed);

        $provide.value('loadProcessed', loadProcessedSpy);
        $provide.value('loadUnprocessed', loadUnprocessedSpy);
      });

      inject($injector => {
        $q = $injector.get('$q');
        loadChangePoints = $injector.get('loadChangePoints');
      });

      allSpy = spyOn($q, 'all').and.returnValue({
        then: (func) => func(resp)
      });
    });

    it('should be handled correctly', function () {
      const value = loadChangePoints(scope, project, variant, task);

      expect(value).toBe(scope.changePoints);
      expect(Object.keys(scope.changePoints).length).toBe(3);
      expect(Object.keys(scope.changePoints)).toEqual([
        "InsertRemove.InsertRemoveTest.remove",
        "InsertRemove.dummy_inserts",
        "InsertRemove.Genny.Setup"
      ]);

      expect(loadUnprocessedSpy).toHaveBeenCalledWith(project, variant, task);
      expect(loadProcessedSpy).toHaveBeenCalledWith(project, variant, task);

      expect(allSpy.calls.allArgs()[0]).toEqual([resp]);
    });

  });

  describe('some overlap', () => {
    beforeEach(() => {
      processed = [
        {
          "test" : "InsertRemove.InsertRemoveTest.remove",
          "suspect_revision" : "8029241ffa616d27502f9396c50a85b95115beaf",
        },
        {
          "test" : "InsertRemove.dummy_inserts",
          "suspect_revision" : "b6147f664cc22c360611b05f79c0d8febf3c7692",
        }
      ];
      unprocessed = [
        {
          "test" : "InsertRemove.Genny.NotOverlapping",
          "suspect_revision" : "03e13f90426a82a97cbb0f926385e09904519259",
        },
        {
          "test" : "InsertRemove.Genny.Setup",
          "suspect_revision" : "b6147f664cc22c360611b05f79c0d8febf3c7692",
        },
        {
          "test" : "InsertRemove.dummy_inserts",
          "suspect_revision" : "b6147f664cc22c360611b05f79c0d8febf3c7692",
        }
      ];
      resp  = {processed, unprocessed};
      module($provide => {
        loadProcessedSpy = jasmine.createSpy('loadProcessed').and.returnValue(processed);
        loadUnprocessedSpy = jasmine.createSpy('loadUnprocessed').and.returnValue(unprocessed);

        $provide.value('loadProcessed', loadProcessedSpy);
        $provide.value('loadUnprocessed', loadUnprocessedSpy);
      });

      inject($injector => {
        $q = $injector.get('$q');
        loadChangePoints = $injector.get('loadChangePoints');
      });

      allSpy = spyOn($q, 'all').and.returnValue({
        then: (func) => func(resp)
      });
    });

    it('should be handled correctly', function () {
      const value = loadChangePoints(scope, project, variant, task);

      expect(value).toBe(scope.changePoints);
      expect(Object.keys(scope.changePoints).length).toBe(4);
      expect(Object.keys(scope.changePoints)).toEqual([
        "InsertRemove.InsertRemoveTest.remove",
        "InsertRemove.dummy_inserts",
        "InsertRemove.Genny.NotOverlapping",
        "InsertRemove.Genny.Setup"
      ]);

      expect(loadUnprocessedSpy).toHaveBeenCalledWith(project, variant, task);
      expect(loadProcessedSpy).toHaveBeenCalledWith(project, variant, task);

      expect(allSpy.calls.allArgs()[0]).toEqual([resp]);
    });

  });

});

describe('loadProcessed', function () {
  beforeEach(module('MCI'));

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';

  let loadProcessed;
  let $log;
  let STITCH_CONFIG;
  const docs = [{a: 'doc'}];

  describe('resolve', () => {
    beforeEach(() => {
      module($provide => {
        $db = {
          db: () => $db,
          collection: () => $db,
          // deleteOne: () => $db,
          find: () => $db,
          then: (resolve) => resolve(docs),
          execute: () => $db,
        };
        spyOn($db, 'db').and.callThrough();
        spyOn($db, 'collection').and.callThrough();
        spyOn($db, 'find').and.callThrough();
        spyOn($db, 'execute').and.callThrough();

        Stitch = {
          use: () => Stitch,
          query: (cb) => cb($db),
        };
        $provide.value('Stitch', Stitch);
      });

      inject($injector => {
        STITCH_CONFIG = $injector.get('STITCH_CONFIG');
        loadProcessed = $injector.get('loadProcessed');
      });
    });

    it('should be handled correctly', function () {
      const value = loadProcessed(project, variant, task);
      expect(value).toBe(docs);
      expect($db.db).toHaveBeenCalledWith(STITCH_CONFIG.PERF.DB_PERF);
      expect($db.collection).toHaveBeenCalledWith(STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS);
      expect($db.find).toHaveBeenCalledWith({ project, variant, task });
      expect($db.execute).toHaveBeenCalledWith();

    });

  });

  describe('reject', () => {
    beforeEach(() => {
      module($provide => {
        $db = {
          db: () => $db,
          collection: () => $db,
          // deleteOne: () => $db,
          find: () => $db,
          then: (resolve, reject) => reject('error!'),
          execute: () => $db,
        };
        spyOn($db, 'db').and.callThrough();
        spyOn($db, 'collection').and.callThrough();
        spyOn($db, 'find').and.callThrough();
        spyOn($db, 'execute').and.callThrough();

        Stitch = {
          use: () => Stitch,
          query: (cb) => cb($db),
        };
        $provide.value('Stitch', Stitch);
      });

      inject($injector => {
        STITCH_CONFIG = $injector.get('STITCH_CONFIG');
        loadProcessed = $injector.get('loadProcessed');
        $log= $injector.get('$log');
        spyOn($log, 'error').and.callThrough();
      });
    });

    it('should be handled correctly', function () {
      const value = loadProcessed(project, variant, task);
      expect(value).toEqual([]);
      expect($db.db).toHaveBeenCalledWith(STITCH_CONFIG.PERF.DB_PERF);
      expect($db.collection).toHaveBeenCalledWith(STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS);
      expect($db.find).toHaveBeenCalledWith({ project, variant, task });
      expect($db.execute).toHaveBeenCalledWith();
      expect($log.error).toHaveBeenCalled();

    });

  });

});

describe('loadUnprocessed', function () {
  beforeEach(module('MCI'));

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';

  let loadUnprocessed;
  let $log;
  let STITCH_CONFIG;
  const docs = [{a: 'doc'}];

  describe('resolve', () => {
    beforeEach(() => {
      module($provide => {
        $db = {
          db: () => $db,
          collection: () => $db,
          // deleteOne: () => $db,
          find: () => $db,
          then: (resolve) => resolve(docs),
          execute: () => $db,
        };
        spyOn($db, 'db').and.callThrough();
        spyOn($db, 'collection').and.callThrough();
        spyOn($db, 'find').and.callThrough();
        spyOn($db, 'execute').and.callThrough();

        Stitch = {
          use: () => Stitch,
          query: (cb) => cb($db),
        };
        $provide.value('Stitch', Stitch);
      });

      inject($injector => {
        STITCH_CONFIG = $injector.get('STITCH_CONFIG');
        loadUnprocessed = $injector.get('loadUnprocessed');
      });
    });

    it('should be handled correctly', function () {
      const value = loadUnprocessed(project, variant, task);
      expect(value).toBe(docs);
      expect($db.db).toHaveBeenCalledWith(STITCH_CONFIG.PERF.DB_PERF);
      expect($db.collection).toHaveBeenCalledWith(STITCH_CONFIG.PERF.COLL_UNPROCESSED_POINTS);
      expect($db.find).toHaveBeenCalledWith({ project, variant, task });
      expect($db.execute).toHaveBeenCalledWith();

    });

  });

  describe('reject', () => {
    beforeEach(() => {
      module($provide => {
        $db = {
          db: () => $db,
          collection: () => $db,
          find: () => $db,
          then: (resolve, reject) => reject('error!'),
          execute: () => $db,
        };
        spyOn($db, 'db').and.callThrough();
        spyOn($db, 'collection').and.callThrough();
        spyOn($db, 'find').and.callThrough();
        spyOn($db, 'execute').and.callThrough();

        Stitch = {
          use: () => Stitch,
          query: (cb) => cb($db),
        };
        $provide.value('Stitch', Stitch);
      });

      inject($injector => {
        STITCH_CONFIG = $injector.get('STITCH_CONFIG');
        loadUnprocessed = $injector.get('loadUnprocessed');
        $log= $injector.get('$log');
        spyOn($log, 'error').and.callThrough();
      });
    });

    it('should be handled correctly', function () {
      const value = loadUnprocessed(project, variant, task);
      expect(value).toEqual([]);
      expect($db.db).toHaveBeenCalledWith(STITCH_CONFIG.PERF.DB_PERF);
      expect($db.collection).toHaveBeenCalledWith(STITCH_CONFIG.PERF.COLL_UNPROCESSED_POINTS);
      expect($db.find).toHaveBeenCalledWith({ project, variant, task });
      expect($db.execute).toHaveBeenCalledWith();
      expect($log.error).toHaveBeenCalled();

    });

  });

});

describe('RevisionsMapper', function () {
  beforeEach(module('MCI'));

  let RevisionsMapper;
  beforeEach(inject($injector => RevisionsMapper = $injector.get('RevisionsMapper')));

  describe('empty', function () {
    it('should handle null', function () {
      const mapper = new RevisionsMapper();
      expect(mapper.allRevisionsMap).toEqual({});
      expect(mapper.taskRevisionsMap).toEqual({});
      expect(mapper.taskOrdersMap).toEqual({});
      expect(mapper.allOrders).toEqual([]);
      expect(mapper.taskOrders).toEqual([]);
    });

    it('should handle empty array', function () {
      const mapper = new RevisionsMapper([]);
      expect(mapper.allRevisionsMap).toEqual({});
      expect(mapper.taskRevisionsMap).toEqual({});
      expect(mapper.taskOrdersMap).toEqual({});
      expect(mapper.allOrders).toEqual([]);
      expect(mapper.taskOrders).toEqual([]);
    });
  });
  it('should no matching tasks', function () {
    const points = [
        {
          "tasks" : [
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-replSet-initialsync-logkeeper",
              "task" : "initialsync-logkeeper"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_waiting_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mongos_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mixed_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "crud_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "industry_benchmarks"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "map_reduce_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "misc_workloads"
            }
          ],
          "revision" : "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7",
          "order" : 6552
        },
        {
          "tasks" : [
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_waiting_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mongos_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mixed_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "crud_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "industry_benchmarks"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "map_reduce_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "misc_workloads"
            }
          ],
          "revision" : "932c2f345598d8e1d283e8c2bb54fd8d0e11c853",
          "order" : 6556
        }
      ]
    ;
    const taskId = {"project":"sys-perf","variant":"linux-1-node-replSet","task":"genny_workloads"};
    const mapper = new RevisionsMapper(points, taskId);
    expect(mapper.allRevisionsMap).toEqual({
      "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7": 6552,
      "932c2f345598d8e1d283e8c2bb54fd8d0e11c853": 6556
    });
    expect(mapper.taskRevisionsMap).toEqual({});
    expect(mapper.taskOrdersMap).toEqual({});
    expect(mapper.allOrders).toEqual([
      6552,
      6556
    ]);
    expect(mapper.taskOrders).toEqual([]);
  });

  it('should handle matching tasks', function () {
    const points = [
        {
          "tasks" : [
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-replSet-initialsync-logkeeper",
              "task" : "initialsync-logkeeper"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_waiting_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mongos_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mixed_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "crud_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "industry_benchmarks"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "map_reduce_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "misc_workloads"
            }
          ],
          "revision" : "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7",
          "order" : 6552
        },
        {
          "tasks" : [
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_waiting_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mongos_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mixed_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "crud_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "industry_benchmarks"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "map_reduce_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "misc_workloads"
            }
          ],
          "revision" : "932c2f345598d8e1d283e8c2bb54fd8d0e11c853",
          "order" : 6556
        }
      ]
    ;
    const taskId = {"project" : "sys-perf",
                    "variant" : "linux-3-shard",
                    "task" : "crud_workloads"};

    const mapper = new RevisionsMapper(points, taskId);
    const revisionMapping = {
      "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7": 6552,
      "932c2f345598d8e1d283e8c2bb54fd8d0e11c853": 6556
    };
    const orderMapping ={
      "6552": "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7",
      "6556": "932c2f345598d8e1d283e8c2bb54fd8d0e11c853"
    };
    const orders = [
      6552,
      6556
    ];
    expect(mapper.allRevisionsMap).toEqual(revisionMapping );
    expect(mapper.taskRevisionsMap).toEqual(revisionMapping );
    expect(mapper.taskOrdersMap).toEqual(orderMapping);
    expect(mapper.allOrders).toEqual(orders);
    expect(mapper.taskOrders).toEqual(orders);
  });

  it('should handle a subset tasks', function () {
    const points = [
        {
          "tasks" : [
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "task_id"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-replSet-initialsync-logkeeper",
              "task" : "initialsync-logkeeper"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_waiting_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mongos_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mixed_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "crud_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "industry_benchmarks"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "map_reduce_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "misc_workloads"
            }
          ],
          "revision" : "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7",
          "order" : 6552
        },
        {
          "tasks" : [
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "move_chunk_waiting_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mongos_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "mixed_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "crud_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "industry_benchmarks"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "map_reduce_workloads"
            },
            {
              "project" : "sys-perf",
              "variant" : "linux-3-shard",
              "task" : "misc_workloads"
            }
          ],
          "revision" : "932c2f345598d8e1d283e8c2bb54fd8d0e11c853",
          "order" : 6556
        }
      ]
    ;
    const taskId = {
      "project" : "sys-perf",
      "variant" : "linux-3-shard",
      "task" : "task_id"
    };

    const mapper = new RevisionsMapper(points, taskId);
    const revisionMapping = {
      "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7": 6552,
      "932c2f345598d8e1d283e8c2bb54fd8d0e11c853": 6556
    };
    const orderMapping ={
      "6552": "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7",
      "6556": "932c2f345598d8e1d283e8c2bb54fd8d0e11c853"
    };
    const orders = [
      6552,
      6556
    ];
    const taskMapping = {
      "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7": 6552,
    };
    const taskOrderMapping ={
      "6552": "6408dcd1b5f4fa1747fa2acac50b8cd004343ca7",
    };
    const taskOrders = [
      6552,
    ];
    expect(mapper.allRevisionsMap).toEqual(revisionMapping );
    expect(mapper.taskRevisionsMap).toEqual(taskMapping );
    expect(mapper.allOrdersMap).toEqual(orderMapping);
    expect(mapper.taskOrdersMap).toEqual(taskOrderMapping);
    expect(mapper.allOrders).toEqual(orders);
    expect(mapper.taskOrders).toEqual(taskOrders);
  });

});

describe('loadRevisions', function () {
  let project = 'sys-perf';
  let variant = 'standalone-linux';
  let task_name = 'bestbuy_agg';

  beforeEach(module('MCI'));

  let loadRevisions;
  let STITCH_CONFIG;
  let revisions=[];
  beforeEach(() => {
    module($provide => {
      $db = {
        db: () => $db,
        collection: () => $db,
        aggregate: () => $db,
        catch: () => $db,
        then: (resolve) => resolve(revisions),
        execute: () => $db,
      };
      spyOn($db, 'db').and.callThrough();
      spyOn($db, 'collection').and.callThrough();
      spyOn($db, 'aggregate').and.callThrough();
      spyOn($db, 'then').and.callThrough();

      Stitch = {
        use: () => Stitch,
        query: (cb) => cb($db),
      };
      $provide.value('Stitch', Stitch);
    });

    inject($injector => {
      STITCH_CONFIG = $injector.get('STITCH_CONFIG');
      loadRevisions = $injector.get('loadRevisions');
    });
  });

  it('should handle null', function () {
    const mapper = loadRevisions(project, variant, task_name);

    expect($db.db).toHaveBeenCalledWith(STITCH_CONFIG.PERF.DB_PERF);
    expect($db.collection).toHaveBeenCalledWith(STITCH_CONFIG.PERF.COLL_POINTS);

    expect(mapper.allRevisionsMap).toEqual({});
    expect(mapper.taskRevisionsMap).toEqual({});
    expect(mapper.taskOrdersMap).toEqual({});
    expect(mapper.allOrders).toEqual([]);
    expect(mapper.taskOrders).toEqual([]);
  });

});

describe('loadBuildFailures', function () {
  let project = 'sys-perf';
  let variant = 'standalone-linux';
  let task_name = 'bestbuy_agg';

  beforeEach(module('MCI'));

  let loadBuildFailures;
  let loadRevisions;
  let STITCH_CONFIG;
  let revisions=[];
  let revisionsQ;

  describe('database access', function () {
    const buildFailures = [];
    beforeEach(() => {
      module($provide => {
        revisionsQ = {
          then: (resolve) => resolve(revisions),
        };
        $db = {
          db: () => $db,
          collection: () => $db,
          aggregate: () => $db,
          then: (resolve) => resolve(revisions),
        };
        spyOn($db, 'db').and.callThrough();
        spyOn($db, 'collection').and.callThrough();
        spyOn($db, 'aggregate').and.callThrough();
        spyOn($db, 'then').and.callThrough();
        loadRevisions = jasmine.createSpy('loadRevisions').and.returnValue(revisionsQ);
        spyOn(revisionsQ, 'then').and.returnValue(buildFailures);

        Stitch = {
          use: () => Stitch,
          query: (cb) => cb($db),
        };
        $provide.value('loadRevisions', loadRevisions);
        $provide.value('Stitch', Stitch);
      });

      inject($injector => {
        STITCH_CONFIG = $injector.get('STITCH_CONFIG');
        loadBuildFailures = $injector.get('loadBuildFailures');
      });
    });

    it('should handle null', function () {
      let scope = {};
      const value = loadBuildFailures(scope, project, variant, task_name);
      expect(value).toBe(buildFailures);

      expect(loadRevisions).toHaveBeenCalledWith(project, variant, task_name);
      expect($db.db).toHaveBeenCalledWith(STITCH_CONFIG.PERF.DB_PERF);
      expect($db.collection).toHaveBeenCalledWith(STITCH_CONFIG.PERF.COLL_BUILD_FAILURES);
      expect($db.aggregate).toHaveBeenCalled();
    });
  });

  describe('processing', function () {
    describe('no docs', function () {
      const buildFailures = [];
      const mapper = {};
      beforeEach(() => {
        module($provide => {
          revisionsQ = {
            then: (resolve) => resolve(mapper),
          };
          loadRevisions = jasmine.createSpy('loadRevisions').and.returnValue(revisionsQ);
          spyOn(revisionsQ, 'then').and.callFake(function(cb) {
            return cb(mapper);
          });
          Stitch = {
            use: () => Stitch,
            query: () => Stitch,
            then: (cb) => cb(buildFailures),
          };
          $provide.value('loadRevisions', loadRevisions);
          $provide.value('Stitch', Stitch);
        });

        inject($injector => {
          STITCH_CONFIG = $injector.get('STITCH_CONFIG');
          loadBuildFailures = $injector.get('loadBuildFailures');
        });
      });

      it('should handle no docs', function () {
        let scope = {};
        const value = loadBuildFailures(scope, project, variant, task_name);
        expect(value).toEqual({});

        expect(loadRevisions).toHaveBeenCalledWith(project, variant, task_name);
      });
    });
    describe('docs', function () {
      const buildFailures = [
        {
          "_id": "BF-12176",
          "key": "BF-12176",
          "tests": "0_1c_avg_latency",
          "first_failing_revision": "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb",
          "fix_revision": []
        },
        {
          "_id": "BF-12176",
          "key": "BF-12176",
          "tests": "0_1c_delete",
          "first_failing_revision": "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb",
          "fix_revision": []
        },
        {
          "_id": "BF-12176",
          "key": "BF-12176",
          "tests": "0_1c_findOne",
          "first_failing_revision": "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb",
          "fix_revision": []
        },
        {
          "_id": "BF-11542",
          "key": "BF-11542",
          "tests": "out_replaceDocuments_100000_to_target_identical_distribution",
          "first_failing_revision": "53d99b27f09b2d0162c44644cd8c9191a9879121",
          "fix_revision": []
        }
      ];
      // This mapping covers the following cases:
      //   * an exact match, 9ac90b128ebeb1bb431ebe3fe9176cb6142818cb
      //   * a near match, 6c2879fca21a4a43c4e173115850b979b434cb88
      //   * no match, 53d99b27f09b2d0162c44644cd8c9191a9879121
      const mapper = {
        allOrdersMap: {
          14422: '6c2879fca21a4a43c4e173115850b979b434cb88',
          15302: '9ac90b128ebeb1bb431ebe3fe9176cb6142818cb',
        },
        allRevisionsMap: {
          '6c2879fca21a4a43c4e173115850b979b434cb88': 14422,
          '9ac90b128ebeb1bb431ebe3fe9176cb6142818cb': 15302,
        },
        taskRevisionsMap: {
          '9ac90b128ebeb1bb431ebe3fe9176cb6142818cb': 15302,
        },
        taskOrdersMap: {
          15302: '9ac90b128ebeb1bb431ebe3fe9176cb6142818cb',
        },
        allOrders: [14422, 15302],
        taskOrders: [15302],
      };
      const expected = {
        "0_1c_avg_latency": [
          {
            "_id": "BF-12176",
            "key": "BF-12176",

            "tests": "0_1c_avg_latency",
            "first_failing_revision": "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb",
            "fix_revision": [],
            "order": 15302,
            "orders": [
              15302
            ],
            "revisions": [
              "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb"
            ]
          }
        ],
        "0_1c_delete": [
          {
            "_id": "BF-12176",
            "key": "BF-12176",

            "tests": "0_1c_delete",
            "first_failing_revision": "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb",
            "fix_revision": [],
            "order": 15302,
            "orders": [
              15302
            ],
            "revisions": [
              "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb"
            ]
          }
        ],
        "0_1c_findOne": [
          {
            "_id": "BF-12176",
            "key": "BF-12176",

            "tests": "0_1c_findOne",
            "first_failing_revision": "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb",
            "fix_revision": [],
            "order": 15302,
            "orders": [
              15302
            ],
            "revisions": [
              "9ac90b128ebeb1bb431ebe3fe9176cb6142818cb"
            ]
          }
        ],
        "out_replaceDocuments_100000_to_target_identical_distribution": [
          {
            "_id": "BF-11542",
            "key": "BF-11542",
            "tests": "out_replaceDocuments_100000_to_target_identical_distribution",
            "first_failing_revision": "53d99b27f09b2d0162c44644cd8c9191a9879121",
            "fix_revision": [],
            "orders": [
              null
            ],
            "order": null,
            "revisions": [
              null
            ]
          }
        ]
      };
      beforeEach(() => {
        module($provide => {
          revisionsQ = {
            then: (resolve) => resolve(mapper),
          };
          loadRevisions = jasmine.createSpy('loadRevisions').and.returnValue(revisionsQ);
          spyOn(revisionsQ, 'then').and.callFake(function (cb) {
            return cb(mapper);
          });
          Stitch = {
            use: () => Stitch,
            query: () => Stitch,
            then: (cb) => cb(buildFailures),
          };
          $provide.value('loadRevisions', loadRevisions);
          $provide.value('Stitch', Stitch);
        });

        inject($injector => {
          STITCH_CONFIG = $injector.get('STITCH_CONFIG');
          loadBuildFailures = $injector.get('loadBuildFailures');
        });
      });

      it('should handle no docs', function () {
        let scope = {};
        const value = loadBuildFailures(scope, project, variant, task_name);
        expect(value).toEqual(expected);
        expect(loadRevisions).toHaveBeenCalledWith(project, variant, task_name);
      });
    });
  });

});
