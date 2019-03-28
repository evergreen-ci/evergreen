describe("PerfPluginTests", function () {
    beforeEach(module("MCI"));
    var $filter;
    beforeEach(inject(function(_$filter_) {
        $filter = _$filter_;
    }));
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
                    }
                }]
            }
        };

        expect(converter(testdata)).toEqual(convertedData);
    })
})
