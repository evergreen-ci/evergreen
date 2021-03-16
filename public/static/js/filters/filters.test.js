describe('FiltersTest', function () {
  beforeEach(module('MCI'));

  let statusLabel

  beforeEach(inject(function ($injector) {
    let $filter = $injector.get('$filter')
    statusLabel = $filter('statusLabel')
  }))


  describe('statusLabel filter test', function () {
    it('Tasks with overriden dependencies are not blocked', function () {
      expect(
        statusLabel({
          task_waiting: 'blocked',
          override_dependencies: true,
          status: 'success'
        })
      ).toBe('success')
    })

    it('Successfull tasks displayed as successfull', function () {
      expect(
        statusLabel({
          status: 'success'
        })
      ).toBe('success')
    })

    it('Waiting tasks displayed as blocked', function () {
      expect(
        statusLabel({
          task_waiting: 'blocked'
        })
      ).toBe('blocked')
    })
  })
});

describe('expandedHistoryConverter', function () {
  beforeEach(module('MCI'));

  let expandedHistoryConverter;
  let rawData = [{
      "name": "8ff0d32307c7d1bc7afa4b30ce4366d5f9f22850",
      "info": {
        "project": "sys-perf",
        "version": "sys_perf_4807c2165b552670c9fe66c79c6d5be34e845023",
        "order": 18362,
        "variant": "linux-1-node-15gbwtcache",
        "task_name": "out_of_cache_scanner",
        "task_id": "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_4807c2165b552670c9fe66c79c6d5be34e845023_19_09_04_18_45_37",
        "execution": 0,
        "test_name": "out_of_cache_scanner",
        "trial": 0,
        "parent": "",
        "tags": null,
        "args": null
      },
      "created_at": "2019-09-06T01:14:10.000Z",
      "completed_at": "2019-09-06T02:03:19.000Z",
      "artifacts": null,
      "rollups": {
        "stats": null,
        "processed_at": null,
        "valid": false
      }
    },
    {
      "name": "4bc9e4beb31e36d3cd83adbd75f5f8eb63c5a09c",
      "info": {
        "project": "sys-perf",
        "version": "sys_perf_4807c2165b552670c9fe66c79c6d5be34e845023",
        "order": 18362,
        "variant": "linux-1-node-15gbwtcache",
        "task_name": "out_of_cache_scanner",
        "task_id": "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_4807c2165b552670c9fe66c79c6d5be34e845023_19_09_04_18_45_37",
        "execution": 0,
        "test_name": "ColdScanner-Scan.2",
        "trial": 0,
        "parent": "8ff0d32307c7d1bc7afa4b30ce4366d5f9f22850",
        "tags": null,
        "args": null
      },
      "created_at": "2019-09-06T01:14:10.000Z",
      "completed_at": "2019-09-06T02:03:19.000Z",
      "artifacts": [{
        "type": "s3",
        "bucket": "genny-metrics",
        "prefix": "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_4807c2165b552670c9fe66c79c6d5be34e845023_19_09_04_18_45_37_0",
        "path": "ColdScanner-Scan.2",
        "format": "ftdc",
        "compression": "none",
        "schema": "raw-events",
        "tags": null,
        "created_at": "2019-09-06T02:03:23.790Z",
        "download_url": "https://genny-metrics.s3.amazonaws.com/sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_4807c2165b552670c9fe66c79c6d5be34e845023_19_09_04_18_45_37_0/ColdScanner-Scan.2"
      }],
      "rollups": {
        "stats": [{
            "name": "AverageLatency",
            "val": 7010.185110530556,
            "version": 3,
            "user": false
          },
          {
            "name": "AverageSize",
            "val": 130,
            "version": 3,
            "user": false
          },
          {
            "name": "OperationThroughput",
            "val": 2059576.0991914733,
            "version": 4,
            "user": false
          },
          {
            "name": "SizeThroughput",
            "val": 267744892.89489153,
            "version": 4,
            "user": false
          },
          {
            "name": "ErrorRate",
            "val": 0,
            "version": 4,
            "user": false
          },
          {
            "name": "Latency50thPercentile",
            "val": 780947002426.5,
            "version": 4,
            "user": false
          },
          {
            "name": "Latency80thPercentile",
            "val": 1104667169411,
            "version": 4,
            "user": false
          },
          {
            "name": "Latency90thPercentile",
            "val": 1104678496947.6333,
            "version": 4,
            "user": false
          },
          {
            "name": "Latency95thPercentile",
            "val": 1104684900770,
            "version": 4,
            "user": false
          },
          {
            "name": "Latency99thPercentile",
            "val": 1104684900770,
            "version": 4,
            "user": false
          },
          {
            "name": "WorkersMin",
            "val": 6,
            "version": 3,
            "user": false
          },
          {
            "name": "WorkersMax",
            "val": 6,
            "version": 3,
            "user": false
          },
          {
            "name": "LatencyMin",
            "val": 697970870799,
            "version": 4,
            "user": false
          },
          {
            "name": "LatencyMax",
            "val": 1104684900770,
            "version": 4,
            "user": false
          },
          {
            "name": "DurationTotal",
            "val": 699173000000,
            "version": 4,
            "user": false
          },
          {
            "name": "ErrorsTotal",
            "val": 0,
            "version": 3,
            "user": false
          },
          {
            "name": "OperationsTotal",
            "val": 1440000000,
            "version": 3,
            "user": false
          },
          {
            "name": "SizeTotal",
            "val": 187200000000,
            "version": 3,
            "user": false
          },
          {
            "name": "OverheadTotal",
            "val": 6057087203,
            "version": 1,
            "user": false
          }
        ],
        "processed_at": null,
        "valid": false
      }
    }
  ];

  beforeEach(inject(function ($injector) {
    const $filter = $injector.get('$filter');
    expandedHistoryConverter = $filter('expandedHistoryConverter');
  }));

  it('should handle empty', function () {
    expect(expandedHistoryConverter(undefined)).toBe(null);
    expect(expandedHistoryConverter(null)).toBe(null);
    expect(expandedHistoryConverter([])).toEqual([]);
  });

  it('should convert data', function () {
    expect(expandedHistoryConverter(rawData)).toEqual([{
      "data": {
        "results": [{
          "name": "out_of_cache_scanner",
          "isExpandedMetric": true,
          "results": {
            "1": {}
          }
        }, {
          "name": "ColdScanner-Scan.2",
          "isExpandedMetric": true,
          "results": {
            "6": {
              "AverageLatency": 7010.185110530556,
              "AverageLatency_values": [7010.185110530556],
              "AverageSize": 130,
              "AverageSize_values": [130],
              "OperationThroughput": 2059576.0991914733,
              "OperationThroughput_values": [2059576.0991914733],
              "SizeThroughput": 267744892.89489153,
              "SizeThroughput_values": [267744892.89489153],
              "ErrorRate": 0,
              "ErrorRate_values": [0],
              "Latency50thPercentile": 780947002426.5,
              "Latency50thPercentile_values": [780947002426.5],
              "Latency80thPercentile": 1104667169411,
              "Latency80thPercentile_values": [1104667169411],
              "Latency90thPercentile": 1104678496947.6333,
              "Latency90thPercentile_values": [1104678496947.6333],
              "Latency95thPercentile": 1104684900770,
              "Latency95thPercentile_values": [1104684900770],
              "Latency99thPercentile": 1104684900770,
              "Latency99thPercentile_values": [1104684900770],
              "WorkersMin": 6,
              "WorkersMin_values": [6],
              "WorkersMax": 6,
              "WorkersMax_values": [6],
              "LatencyMin": 697970870799,
              "LatencyMin_values": [697970870799],
              "LatencyMax": 1104684900770,
              "LatencyMax_values": [1104684900770],
              "DurationTotal": 699173000000,
              "DurationTotal_values": [699173000000],
              "ErrorsTotal": 0,
              "ErrorsTotal_values": [0],
              "OperationsTotal": 1440000000,
              "OperationsTotal_values": [1440000000],
              "SizeTotal": 187200000000,
              "SizeTotal_values": [187200000000],
              "OverheadTotal": 6057087203,
              "OverheadTotal_values": [6057087203]
            }
          }
        }]
      },
      "create_time": "2019-09-06T01:14:10.000Z",
      "order": 18362,
      "version_id": "sys_perf_4807c2165b552670c9fe66c79c6d5be34e845023",
      "project_id": "sys-perf",
      "task_name": "out_of_cache_scanner",
      "variant": "linux-1-node-15gbwtcache",
      "task_id": "sys_perf_linux_1_node_15gbwtcache_out_of_cache_scanner_4807c2165b552670c9fe66c79c6d5be34e845023_19_09_04_18_45_37",
      "revision": "4807c2165b552670c9fe66c79c6d5be34e845023"
    }])
  })

  it("should handle data with multiple thread levels as separate results", () => {
    const input = [{
        "name": "1",
        "info": {
          "project": "myproj",
          "version": "v1",
          "order": 1,
          "variant": "linux-3-node-v1",
          "task_name": "task1",
          "task_id": "task1",
          "execution": 0,
          "test_name": "mytest",
          "trial": 0,
          "parent": "",
          "tags": null,
          "args": {
            "thread_level": 64
          }
        },
        "created_at": "2020-04-02T17:13:31.692Z",
        "completed_at": "2020-04-02T17:30:56.512Z",
        "artifacts": null,
        "rollups": {
          "stats": [{
              "name": "ops_per_sec",
              "val": 45042.20454565928,
              "version": 0,
              "user": true
            },
            {
              "name": "average_read_latency_us",
              "val": 1111.5834836163208,
              "version": 0,
              "user": true
            },
            {
              "name": "95th_read_latency_us",
              "val": 2950,
              "version": 0,
              "user": true
            },
            {
              "name": "99th_read_latency_us",
              "val": 5230,
              "version": 0,
              "user": true
            }
          ],
          "processed_at": "2020-04-02T18:12:21.868Z"
        },
        "analysis": {
          "change_points": null,
          "processed_at": "2020-04-14T15:46:50.247Z"
        }
      },
      {
        "name": "2",
        "info": {
          "project": "myproj",
          "version": "v1",
          "order": 1,
          "variant": "linux-3-node-v1",
          "task_name": "task1",
          "task_id": "task1",
          "execution": 0,
          "test_name": "mytest",
          "trial": 0,
          "parent": "",
          "tags": null,
          "args": {
            "thread_level": 1
          }
        },
        "created_at": "2020-04-02T17:33:22.491Z",
        "completed_at": "2020-04-02T17:53:23.624Z",
        "artifacts": null,
        "rollups": {
          "stats": [{
              "name": "ops_per_sec",
              "val": 751.5374846251538,
              "version": 0,
              "user": true
            },
            {
              "name": "average_read_latency_us",
              "val": 362.5341057854027,
              "version": 0,
              "user": true
            },
            {
              "name": "95th_read_latency_us",
              "val": 420,
              "version": 0,
              "user": true
            },
            {
              "name": "99th_read_latency_us",
              "val": 490,
              "version": 0,
              "user": true
            }
          ],
          "processed_at": "2020-04-02T18:12:21.877Z"
        },
        "analysis": {
          "change_points": null,
          "processed_at": "2020-04-14T15:46:52.948Z"
        }
      },
    ];
    const expected = [{
      "data": {
        "results": [{
          "name": "mytest",
          "isExpandedMetric": true,
          "results": {
            "1": {
              "ops_per_sec": 751.5374846251538,
              "ops_per_sec_values": [751.5374846251538],
              "average_read_latency_us": 362.5341057854027,
              "average_read_latency_us_values": [362.5341057854027],
              "95th_read_latency_us": 420,
              "95th_read_latency_us_values": [420],
              "99th_read_latency_us": 490,
              "99th_read_latency_us_values": [490]
            },
            "64": {
              "ops_per_sec": 45042.20454565928,
              "ops_per_sec_values": [45042.20454565928],
              "average_read_latency_us": 1111.5834836163208,
              "average_read_latency_us_values": [1111.5834836163208],
              "95th_read_latency_us": 2950,
              "95th_read_latency_us_values": [2950],
              "99th_read_latency_us": 5230,
              "99th_read_latency_us_values": [5230]
            }
          }
        }]
      },
      "create_time": "2020-04-02T17:13:31.692Z",
      "order": 1,
      "version_id": "v1",
      "project_id": "myproj",
      "task_name": "task1",
      "variant": "linux-3-node-v1",
      "task_id": "task1"
    }];
    expect(expandedHistoryConverter(input)).toEqual(expected);
  });
});

describe('expandedMetricConverter', function () {
  beforeEach(module('MCI'));

  beforeEach(inject(function ($injector) {
    const $filter = $injector.get('$filter');
    expandedMetricConverter = $filter('expandedMetricConverter');
  }));

  it('Correctly formatted data should be converted correctly', function () {
    const dataSample = [{
        "name": "87a18ca40e878b535409db66a071bb92a0bee20b",
        "info": {
          "project": "sys-perf",
          "version": "sys_perf_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2",
          "order": 19613,
          "variant": "wtdevelop-1-node-replSet",
          "task_name": "insert_remove",
          "task_id": "sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59",
          "execution": 0,
          "test_name": "insert_remove",
          "trial": 0,
          "parent": "",
          "tags": null,
          "args": null
        },
        "created_at": "2019-12-05T07:19:22.000Z",
        "completed_at": "2019-12-05T07:31:57.000Z",
        "artifacts": null,
        "rollups": {
          "stats": null,
          "processed_at": null,
          "valid": false
        }
      },
      {
        "name": "2668d260c2a7925689b21524d4c7b943fe91c3fa",
        "info": {
          "project": "sys-perf",
          "version": "sys_perf_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2",
          "order": 19613,
          "variant": "wtdevelop-1-node-replSet",
          "task_name": "insert_remove",
          "task_id": "sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59",
          "execution": 0,
          "test_name": "InsertRemoveTest-Insert",
          "trial": 0,
          "parent": "87a18ca40e878b535409db66a071bb92a0bee20b",
          "tags": null,
          "args": null
        },
        "created_at": "2019-12-05T07:19:22.000Z",
        "completed_at": "2019-12-05T07:31:57.000Z",
        "artifacts": [{
          "type": "s3",
          "bucket": "genny-metrics",
          "prefix": "sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59_0",
          "path": "InsertRemoveTest-Insert",
          "format": "ftdc",
          "compression": "none",
          "schema": "raw-events",
          "tags": null,
          "created_at": "2019-12-05T07:34:02.875Z",
          "download_url": "www.example.com"
        }],
        "rollups": {
          "stats": [{
              "name": "AverageLatency",
              "val": 3116886.561614268,
              "version": 3,
              "user": false
            },
            {
              "name": "AverageSize",
              "val": 14,
              "version": 3,
              "user": false
            },
            {
              "name": "OperationThroughput",
              "val": 15876.74890823621,
              "version": 4,
              "user": false
            },
            {
              "name": "SizeThroughput",
              "val": 222274.48471530696,
              "version": 4,
              "user": false
            },
            {
              "name": "ErrorRate",
              "val": 0,
              "version": 4,
              "user": false
            },
            {
              "name": "Latency50thPercentile",
              "val": 2775838,
              "version": 4,
              "user": false
            },
            {
              "name": "Latency80thPercentile",
              "val": 4533626.800000002,
              "version": 4,
              "user": false
            },
            {
              "name": "Latency90thPercentile",
              "val": 5624434.400000001,
              "version": 4,
              "user": false
            },
            {
              "name": "Latency95thPercentile",
              "val": 6629202.40000001,
              "version": 4,
              "user": false
            },
            {
              "name": "Latency99thPercentile",
              "val": 8782489.040000014,
              "version": 4,
              "user": false
            },
            {
              "name": "WorkersMin",
              "val": 100,
              "version": 3,
              "user": false
            },
            {
              "name": "WorkersMax",
              "val": 100,
              "version": 3,
              "user": false
            },
            {
              "name": "LatencyMin",
              "val": 262779,
              "version": 4,
              "user": false
            },
            {
              "name": "LatencyMax",
              "val": 572543762,
              "version": 4,
              "user": false
            },
            {
              "name": "DurationTotal",
              "val": 179755000000,
              "version": 4,
              "user": false
            },
            {
              "name": "ErrorsTotal",
              "val": 0,
              "version": 3,
              "user": false
            },
            {
              "name": "OperationsTotal",
              "val": 2853925,
              "version": 3,
              "user": false
            },
            {
              "name": "SizeTotal",
              "val": 39954950,
              "version": 3,
              "user": false
            },
            {
              "name": "OverheadTotal",
              "val": 9097063636926,
              "version": 1,
              "user": false
            }
          ],
          "processed_at": null,
          "valid": false
        }
      }
    ]

    const expected = {
      "data": {
        "results": [{
          "name": "insert_remove",
          "isExpandedMetric": true,
          "results": {
            "1": {}
          }
        }, {
          "name": "InsertRemoveTest-Insert",
          "isExpandedMetric": true,
          "results": {
            "100": {
              "AverageLatency": 3116886.561614268,
              "AverageLatency_values": [3116886.561614268],
              "AverageSize": 14,
              "AverageSize_values": [14],
              "OperationThroughput": 15876.74890823621,
              "OperationThroughput_values": [15876.74890823621],
              "SizeThroughput": 222274.48471530696,
              "SizeThroughput_values": [222274.48471530696],
              "ErrorRate": 0,
              "ErrorRate_values": [0],
              "Latency50thPercentile": 2775838,
              "Latency50thPercentile_values": [2775838],
              "Latency80thPercentile": 4533626.800000002,
              "Latency80thPercentile_values": [4533626.800000002],
              "Latency90thPercentile": 5624434.400000001,
              "Latency90thPercentile_values": [5624434.400000001],
              "Latency95thPercentile": 6629202.40000001,
              "Latency95thPercentile_values": [6629202.40000001],
              "Latency99thPercentile": 8782489.040000014,
              "Latency99thPercentile_values": [8782489.040000014],
              "WorkersMin": 100,
              "WorkersMin_values": [100],
              "WorkersMax": 100,
              "WorkersMax_values": [100],
              "LatencyMin": 262779,
              "LatencyMin_values": [262779],
              "LatencyMax": 572543762,
              "LatencyMax_values": [572543762],
              "DurationTotal": 179755000000,
              "DurationTotal_values": [179755000000],
              "ErrorsTotal": 0,
              "ErrorsTotal_values": [0],
              "OperationsTotal": 2853925,
              "OperationsTotal_values": [2853925],
              "SizeTotal": 39954950,
              "SizeTotal_values": [39954950],
              "OverheadTotal": 9097063636926,
              "OverheadTotal_values": [9097063636926]
            }
          }
        }]
      },
      "create_time": "2019-12-05T07:19:22.000Z",
      "order": 19613,
      "version_id": "sys_perf_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2",
      "project_id": "sys-perf",
      "task_name": "insert_remove",
      "variant": "wtdevelop-1-node-replSet",
      "task_id": "sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59",
      "revision": "95eedd93ea7a5b8b35b9bb042d5ca165736c17c2"
    }

    expect(expandedMetricConverter(dataSample, 0)).toEqual(expected);
  })

  it('Not passing an execution should use the latest one', function () {
    const dataSample = [{
        "name": "2668d260c2a7925689b21524d4c7b943fe91c3fa",
        "info": {
          "project": "sys-perf",
          "version": "sys_perf_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2",
          "order": 19613,
          "variant": "wtdevelop-1-node-replSet",
          "task_name": "insert_remove",
          "task_id": "sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59",
          "execution": 0,
          "test_name": "InsertRemoveTest-Insert",
          "trial": 0,
          "parent": "87a18ca40e878b535409db66a071bb92a0bee20b",
          "tags": null,
          "args": null
        },
        "created_at": "2019-12-05T07:19:22.000Z",
        "completed_at": "2019-12-05T07:31:57.000Z",
        "rollups": {
          "stats": [{
              "name": "AverageLatency",
              "val": 3116886.561614268,
              "version": 3,
              "user": false
            },
            {
              "name": "WorkersMin",
              "val": 100,
              "version": 3,
              "user": false
            },
            {
              "name": "WorkersMax",
              "val": 100,
              "version": 3,
              "user": false
            },
          ],
          "processed_at": null,
          "valid": false
        }
      },
      {
        "name": "2668d260c2a7925689b21524d4c7b943fe91c3fb",
        "info": {
          "project": "sys-perf",
          "version": "sys_perf_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2",
          "order": 19613,
          "variant": "wtdevelop-1-node-replSet",
          "task_name": "insert_remove",
          "task_id": "sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59",
          "execution": 1,
          "test_name": "InsertRemoveTest-Insert",
          "trial": 0,
          "parent": "87a18ca40e878b535409db66a071bb92a0bee20b",
          "tags": null,
          "args": null
        },
        "created_at": "2019-12-05T07:19:22.000Z",
        "completed_at": "2019-12-05T07:31:57.000Z",
        "rollups": {
          "stats": [{
              "name": "AverageLatency",
              "val": 12345,
              "version": 3,
              "user": false
            },
            {
              "name": "WorkersMin",
              "val": 100,
              "version": 3,
              "user": false
            },
            {
              "name": "WorkersMax",
              "val": 100,
              "version": 3,
              "user": false
            },
          ],
          "processed_at": null,
          "valid": false
        }
      }
    ]

    const expected = {
      create_time: '2019-12-05T07:19:22.000Z',
      order: 19613,
      version_id: 'sys_perf_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2',
      project_id: 'sys-perf',
      task_name: 'insert_remove',
      variant: 'wtdevelop-1-node-replSet',
      task_id: 'sys_perf_wtdevelop_1_node_replSet_insert_remove_95eedd93ea7a5b8b35b9bb042d5ca165736c17c2_19_12_05_04_28_59',
      revision: '95eedd93ea7a5b8b35b9bb042d5ca165736c17c2',
      "data": {
        "results": [{
          "name": "InsertRemoveTest-Insert",
          "isExpandedMetric": true,
          "results": {
            "100": {
              "AverageLatency": 12345,
              "AverageLatency_values": [12345],
              "WorkersMin": 100,
              "WorkersMin_values": [100],
              "WorkersMax": 100,
              "WorkersMax_values": [100]
            }
          }
        }]
      }
    };
    expect(expandedMetricConverter(dataSample)).toEqual(expected);
  });

  it('should handle thread-levels specified via "threads" arg', function () {
    const dataSample = [
      {
        "name": "876d091b3c827c5439b4110b71d2ef592839133e",
        "info": {
          "project": "sys-perf",
          "version": "sys_perf_dfe2b9dd1744725d9c3cdb6b981a78a9f8a62a7f",
          "order": 26994,
          "variant": "linux-1-node-replSet",
          "task_name": "tpcc",
          "task_id": "sys_perf_linux_1_node_replSet_tpcc_dfe2b9dd1744725d9c3cdb6b981a78a9f8a62a7f_21_03_08_18_40_55",
          "execution": 0,
          "test_name": "canary_client-cpuloop-10x",
          "trial": 0,
          "parent": "",
          "tags": null,
          "args": {
            "threads": 16
          }
        },
        "created_at": "2021-03-08T21:05:33.517Z",
        "completed_at": "2021-03-08T21:08:56.946Z",
        "artifacts": null,
        "rollups": {
          "stats": [
            {
              "name": "ops_per_sec",
              "val": 122747.96,
              "version": 0,
              "user": true
            }
          ],
          "processed_at": "2021-03-08T21:10:09.813Z"
        },
        "analysis": {
          "processed_at": "2021-03-16T00:03:26.869Z"
        }
      }
    ]

    const expected = {
      create_time: "2021-03-08T21:05:33.517Z",
      order: 26994,
      version_id: "sys_perf_dfe2b9dd1744725d9c3cdb6b981a78a9f8a62a7f",
      project_id: 'sys-perf',
      task_name: 'tpcc',
      variant: "linux-1-node-replSet",
      task_id: "sys_perf_linux_1_node_replSet_tpcc_dfe2b9dd1744725d9c3cdb6b981a78a9f8a62a7f_21_03_08_18_40_55",
      revision: 'dfe2b9dd1744725d9c3cdb6b981a78a9f8a62a7f',
      "data": {
        "results": [{
          "name": "canary_client-cpuloop-10x",
          "isExpandedMetric": true,
          "results": {
            "16": {
              "ops_per_sec": 122747.96,
              "ops_per_sec_values": [122747.96],
            }
          }
        }]
      }
    };
    expect(expandedMetricConverter(dataSample)).toEqual(expected);
  })
})