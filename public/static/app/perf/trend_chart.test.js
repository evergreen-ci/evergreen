describe('DrawPerfTrendChartTest', function () {
  beforeEach(module('MCI'));
  let svcFactory = null;
  let rootScope = null;

  beforeEach(inject(function ($injector, $rootScope) {
    svcFactory = $injector.get('DrawPerfTrendChart');
    rootScope = $rootScope
  }));

  it('should not generate error when given non-constant max indices', function () {
    let scope = {
      task:{
        id: "sys_perf_linux_standalone_big_update_bb9114dc71bfcf42422471f7789eca00881b8864_19_01_03_20_13_57",
      },
      $parent: rootScope,
      $on: function(){}
    }

    let params = {
      series: [
        {
          "revision": "bb9114dc71bfcf42422471f7789eca00881b8864",
          "task_id": "sys_perf_linux_standalone_big_update_bb9114dc71bfcf42422471f7789eca00881b8864_19_01_03_20_13_57",
          "order": 14925,
          "createTime": "2019-01-03T20:13:57Z",
          "threadResults": [{
            "ops_per_sec": 52104931,
            "ops_per_sec_values": [52104931],
            "threadLevel": "1"
          }],
          "ops_per_sec": 52104931,
          "ops_per_sec_values": [52104931],
          "threadLevel": "1"
        },
        {
          "revision": "bd209676619a5395f849bd75334aa378527d2181",
          "task_id": "sys_perf_linux_standalone_big_update_bd209676619a5395f849bd75334aa378527d2181_19_01_04_18_52_02",
          "order": 14943,
          "createTime": "2019-01-04T18:52:02Z",
          "threadResults": [{
            "ops_per_sec": 51883813,
            "ops_per_sec_values": [51883813],
            "threadLevel": "100"
          }],
          "ops_per_sec": 51883813,
          "ops_per_sec_values": [51883813],
          "threadLevel": "100"
        }
      ],
      buildFailures: [],
      changePoints: [],
      key: 'someKey',
      scope: scope,
      containerId: 'containerId',
      compareSamples: [],
      threadMode: 'maxonly',
      linearMode: true,
      originMode: true
    };

    document.getElementById = function (){
      return {}
    };
    svcFactory(params)
  });
});