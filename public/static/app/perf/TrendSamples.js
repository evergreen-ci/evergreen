mciModule.factory('TrendSamples', function() {
  // Class to contain a collection of samples in a series.
  return function(samples){
    this.samples = samples;
    var NON_THREAD_LEVELS = ['start', 'end']

    // _sampleByCommitIndexes is a map of mappings of (githash -> sample data), keyed by test name.
    // e.g.
    // {
    //   "test-foo":  {"ab215e..." : { sample data }, "bced3f..." : { sample data }, ...
    //   "test-blah": {"ab215e..." : { sample data }, "bced3f..." : { sample data }, ...
    //   ..
    //}
    this._sampleByCommitIndexes = {};

    // seriesByName is a mapping of test names to sample data.
    this.seriesByName = {};

    this._tasksByName = {}

    // testNames is a unique list of all the tests that appear in *any* of the given list of samples.
    this.testNames = [];

    // Available metrics (ops_per_sec is default and mandatory one)
    this.metrics = []

    // Internal-use metrics accumilator hash table
    const metricsSet = new Set()

    for (var i = 0; i < samples.length; i++) {
      var sample = samples[i];

      if (sample) {
        for (var j = 0; j < sample.data.results.length; j++) {
          var rec = sample.data.results[j];

          // Create entry if not exists
          if (!(rec.name in this.seriesByName)) {
            this.seriesByName[rec.name] = [];
          }

          var maxValues = _.max(rec.results, function(d) {
            return null;
          })

          // Sort items by thread level
          // Change dict to array
          var threadResults = _.chain(rec.results)
            .omit(NON_THREAD_LEVELS)
            .map(function(v, k) {
              _.each(_.keys(v), (d) => metricsSet.add(d))
              v.threadLevel = k
              return v
            })
            .sortBy('-threadLevel')
            .value()

          let newSample = {
            revision: sample.revision,
            task_id: sample.task_id,
            order: sample.order,
            createTime: sample.create_time,
            threadResults: threadResults,
          }

          Object.assign(newSample, maxValues);

          this.seriesByName[rec.name].push(newSample);
        }
      }
    }

    // copy unique items to an array
    this.metrics = _.reject([...metricsSet], (d) => d.endsWith('_values'))
    // free mem
    metricsSet.clear()

    for (let key in this.seriesByName) {
      this.seriesByName[key] = _.sortBy(this.seriesByName[key], 'order');
      this.testNames.unshift(key);
    }

    for (let i = 0; i < this.testNames.length; i++) {
      //make an index for commit hash -> sample for each test series
      var k = this.testNames[i];
      // FIXME Unknown behavior (coma operator after _groupBy stmt)
      this._sampleByCommitIndexes[k] = _.groupBy(this.seriesByName[k], "revision"), function(x){return x[0]};
      for(let t in this._sampleByCommitIndexes[k]){
        this._sampleByCommitIndexes[k][t] = this._sampleByCommitIndexes[k][t][0];
      }
    }

    // Returns a list of samples for a given test, sorted in the order that they were committed.
    this.tasksByCommitOrder = function(){
      if (!this._tasks) {
        this._tasks = _.chain(this.seriesByName)
          .values()
          .flatten()
          .uniq(false, d => d.task_id)
          .sortBy('order')
          .value()
      }
      return this._tasks;
    }

    this.tasksByCommitOrderByTestName = function(testName){
       if(!(testName in this._tasksByName)){
          this._tasksByName[testName] = _.sortBy(_.uniq(this.seriesByName[testName], function(x){return x.task_id}), "order")
       }
       return this._tasksByName[testName]
    }

    this.sampleInSeriesAtCommit = function(testName, revision){
      let sample = this._sampleByCommitIndexes[testName];
      if (sample) {
        return this._sampleByCommitIndexes[testName][revision];
      }
      return null;
    }

    this.indexOfCommitInSeries = function(testName, revision){
      var t = this.tasksByCommitOrderByTestName(testName)
      return findIndex(t, function(x) { return x.revision==revision })
    }

    this.noiseAtCommit = function(testName, revision){
      var sample = this._sampleByCommitIndexes[testName][revision];
      if(sample && sample.ops_per_sec_values && sample.ops_per_sec_values.length > 1){
        var r = (_.max(sample.ops_per_sec_values) - _.min(sample.ops_per_sec_values)) / d3.mean(sample.ops_per_sec_values);
        return r;
      }
    }
  }
})
