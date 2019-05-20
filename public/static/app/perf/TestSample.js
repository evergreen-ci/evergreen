mciModule.factory('TestSample', function() {
  return function(sample){
    this.sample = sample;
    this._threads = null;
    this._maxes = {};

    this.threads = function(){
      if(this._threads == null){
        this._threads = _.uniq(
          _.filter(
            _.flatten(
              _.map(this.sample.data.results, function(x){
                return _.keys(x.results)
              }), true
            ), numericFilter
          )
        );
      }
      return this._threads;
    }

    this.testNames = function(){
      return _.pluck(this.sample.data.results, "name") ;
    }

    this.getLegendName = function(){
      if(!!this.sample.tag){
        return this.sample.tag
      }
      return this.sample.revision.substring(0,7)
    }

    // Returns only the keys that have results stored in them
    this.resultKeys = function(testName){
      var testInfo = this.resultForTest(testName);
      return _.pluck(_(testInfo.results).pairs().filter(function(x){return typeof(x[1]) == "object"}), 0)
    }

    this.threadsVsOps = function(testName) {
      var testInfo = this.resultForTest(testName);
      var result = [];
      if (!testInfo)
        return;
      var series = testInfo.results;

      var keys = this.resultKeys(testName)
      for (var j = 0; j < keys.length; j++) {
        let value = {threads: parseInt(keys[j])};
        for (key in series[keys[j]]) {
          value[key] = series[keys[j]][key];
        }
        result.push(value);
      }
      _.sortBy(result, "threads");
      return result;
    }

    this.resultForTest = function(testName){
      return _.findWhere(
        this.sample.data.results, {name: testName}
      );
    }

    this.maxThroughputForTest = function(testName, metric){
      if(!_.has(this._maxes, testName)){
        var d = this.resultForTest(testName);
        if(!d){
          return;
        }
        this._maxes[testName] = _.max(
          _.filter(
            _.pluck(
              _.values(d.results), metric
            ), numericFilter
          )
        );
      }
      return this._maxes[testName];
    }
  }
})
