var numericFilter = function(x){
  return !isNaN(parseInt(x))
}

// since we are using an older version of _.js that does not have this function
var findIndex = function(list, predicate) {
  for(var i=0;i<list.length;i++){
    if(predicate(list[i])){
      return i
    }
  }
}

function average (arr){
  if(!arr || arr.length == 0) return // undefined for 0-length array
  return _.reduce(arr, function(memo, num){
    return memo + num;
  }, 0) / arr.length;
}



mciModule.controller('PerfController', function PerfController($scope, $window, $http, $location){
    /* for debugging
    $sce, $compile){

  var templateUrl = $sce.getTrustedResourceUrl('/plugin/perf/static/task_perf_data3.html');
  $http.get(templateUrl).success(function(template) {
      // template is the HTML template as a string

      // Let's put it into an HTML element and parse any directives and expressions
      // in the code. (Note: This is just an example, modifying the DOM from within
      // a controller is considered bad style.)
      $compile($("#perfcontents").html(template).contents())($scope);
  }, function() {});
  */

  $scope.hiddenGraphs = {}
  $scope.compareItemList = []
  $scope.perfTagData = {}
  $scope.compareForm = {}
  $scope.savedCompares = []

  $scope.isGraphHidden = function(k){
    return $scope.hiddenGraphs[k] == true
  }

  $scope.toggleGraph = function(k){
    if(k in $scope.hiddenGraphs){
      delete $scope.hiddenGraphs[k]
    }else{
      $scope.hiddenGraphs[k] = true
    }
    $scope.syncHash(-1)
  }

  $scope.syncHash = function(tab){
    var hash = {}
    if($location.hash().length > 0){
      hash = JSON.parse($location.hash())
    }
    if(Object.keys($scope.hiddenGraphs).length > 0){
      hash.hiddenGraphs = Object.keys($scope.hiddenGraphs)
    }
    if(tab >=0){
      hash.perftab = tab
    }

    if($scope.savedCompares.length > 0){
      hash.compare = $scope.savedCompares
    }else{
      delete hash.compare
    }
    setTimeout(function(){
      $location.hash(encodeURIComponent(JSON.stringify(hash)))
      $scope.$apply()
    },1)
  }

  $scope.checkEnter = function(keyEvent){
    if (keyEvent.which === 13){
      compareItemList.push($scope.compareHash)
      $scope.compareHash = ''
    }
  }

  $scope.removeCompareItem = function(index){
    $scope.comparePerfSamples.splice(index,1);
    $scope.savedCompares.splice(index,1);
    $scope.redrawGraphs()
    $scope.syncHash(-1)
  }

  $scope.deleteTag = function(){
    $http.delete("/plugin/json/task/" + $scope.task.id + "/perf/tag").then(
      function(resp){ delete $scope.perfTagData.tag; },
      function(){console.log("error")}
    );
  }

  // needed to do Math.abs in the template code.
  $scope.Math = $window.Math;
  $scope.conf = $window.plugins["perf"];
  $scope.task = $window.task_data;
  $scope.tablemode = "maxthroughput";

  // perftab refers to which tab should be selected. 0=graph, 1=table, 2=trend, 3=trend-table
  $scope.perftab = $scope.task.patch_info ? 1 : 2;
  $scope.project = $window.project;
  $scope.compareHash = "ss";
  $scope.comparePerfSamples = [];

  $scope.$watch('currentHash', function(){
    $scope.hoverSamples = {}
    if(!!$scope.perfSample){
      var testNames = $scope.perfSample.testNames()
      for(var i=0;i<testNames.length;i++){
        var s = $scope.trendSamples.sampleInSeriesAtCommit(testNames[i], $scope.currentHash)
        $scope.hoverSamples[testNames[i]] = s
      }
    }
  })

  //$scope.$watch('perftab',$scope.syncHash)

  // converts a percentage to a color. Higher -> greener, Lower -> redder.
  $scope.percentToColor = function(percent) {
    var percentColorRanges = [
      {min:-Infinity, max:-15,  color: "#FF0000"},
      {min:-15,       max:-10,  color: "#FF5500"},
      {min:-10,       max:-5,        color: "#FFAA00"},
      {min:-5,        max:-2.5,      color: "#FEFF00"},
      {min:-2.5,      max:5,         color: "#A9FF00"},
      {min:5,         max:10,        color: "#54FF00"},
      {min:10,        max:+Infinity, color: "#00FF00"}
    ];

    for(var i=0;i<percentColorRanges.length;i++){
      if(percent>percentColorRanges[i].min && percent<=percentColorRanges[i].max){
        return percentColorRanges[i].color;
      }
    }
    return "";
  }

  $scope.percentDiff = function(val1, val2){
    return (val1 - val2)/Math.abs(val2);
  }

  $scope.getPctDiff = function(referenceOps, sample, testKey){
    if(sample == null) return "";
    var compareTest = _.find(sample.data.results, function(x){return x.name == testKey});
    var compareMaxOps = $scope.getMax(compareTest.results);
    var pctDiff = (referenceOps-compareMaxOps)/referenceOps;
    return pctDiff;
  }

  $scope.getMax = function(r){
    return _.max(_.filter(_.pluck(_.values(r), 'ops_per_sec'), numericFilter));
  }

  cleanId = function(id){
    return id.replace(/\./g,"-")
  }
  $scope.cleanId = cleanId

  function drawDetailGraph(sample, compareSamples, taskId){
    var testNames = sample.testNames();
    for(var i=0;i<testNames.length;i++){
      var testName = testNames[i];
      $("#chart-" + cleanId(taskId) + "-" + i).empty();
      var series1 = sample.threadsVsOps(testName);
      var margin = { top: 20, right: 50, bottom: 30, left: 80 };
      var width = 450 - margin.left - margin.right;
      var height = 200 - margin.top - margin.bottom;
      var svg = d3.select("#chart-" + cleanId(taskId) + "-" + i)
        .append("svg")
        .attr("width", width + margin.left + margin.right)
        .attr("height", height + margin.top + margin.bottom)
        .append("g")
        .attr("transform", "translate(" + margin.left + "," + margin.top + ")");

      var series = [series1];
      var numSeries = 1;
      if(compareSamples){
        for(var j=0;j<compareSamples.length;j++){
          var compareSeries = compareSamples[j].threadsVsOps(testName);
          series.push(compareSeries);
        }
      }

      var y
      if(d3.max(_.flatten(_.pluck(_.flatten(series), "ops_per_sec_values")))){
        y = d3.scale.linear()
          .domain([0, d3.max(_.flatten(_.pluck(_.flatten(series), "ops_per_sec_values")))])
          .range([height, 0]);
      }else{
        y = d3.scale.linear()
          .domain([0, d3.max(_.flatten(_.pluck(_.flatten(series), "ops_per_sec")))])
          .range([height, 0]);
      }


      var x = d3.scale.ordinal()
        .domain(_.pluck(_.flatten(series), "threads"))
        .rangeRoundBands([0, width]);
      var x1 = d3.scale.ordinal()
        .domain(d3.range(series.length))
        .rangeBands([0, x.rangeBand()], .3);

      var z = d3.scale.category10();

      var bar = svg.selectAll("g")
        .data(series)
        .enter().append("g")
        .style("fill", function(d, i) { return z(i); })
        .attr("transform", function(d, i) { return "translate(" + x1(i) + ",0)"; });

      bar.selectAll("rect")
        .data(function(d){return d})
        .enter().append("rect")
        .attr('stroke', 'black')
        .attr('x', function(d, i) {
          return x(d.threads);
        })
        .attr('y', function(d){
          return y(d.ops_per_sec)
        })
        .attr('height', function(d) {
          return height-y(d.ops_per_sec)
        })
        .attr("width", x1.rangeBand());

      var yAxis = d3.svg.axis()
        .scale(y)
        .orient("left");

      var errorBarArea = d3.svg.area()
        .x(function(d) {
          return x(d.threads) + (x1.rangeBand() / 2);
        })
        .y0(function(d) {
          return y(d3.min(d.ops_per_sec_values))
        })
        .y1(function(d) {
          return y(d3.max(d.ops_per_sec_values))
        }).interpolate("linear");


      bar.selectAll(".err")
        .data(function(d) {
          return d.filter(function(d){
            return ("ops_per_sec_values" in d) && ("ops_per_sec_values" in d) && (d.ops_per_sec_values != undefined && d.ops_per_sec_values.length > 1);
          })
        })
      .enter().append("svg")
        .attr("class", "err")
        .append("path")
        .attr("stroke", "red")
        .attr("stroke-width", 1.5)
        .attr("d", function(d) {
          return errorBarArea([d]);
        });

      var xAxis = d3.svg.axis()
        .scale(x)
        .orient("bottom");
      svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + height + ")")
        .call(xAxis);
      svg.append("g")
        .attr("class", "y axis")
        .call(yAxis);

      if(i==0 && series.length > 1){
        $('#legend').empty()
        var legendHeight = (series.length * 20);
        var legendWidth = 200;
        var legend_y = d3.scale.ordinal()
          .domain(d3.range(series.length))
          .rangeRoundBands([0, legendHeight],.2);
        var svg = d3.select("#legend")
          .append("svg")
          .attr("width", legendWidth)
          .attr("height", legendHeight + 10)
          .append("g");
        svg.selectAll("rect")
          .data(series)
          .enter()
          .append("rect")
          .attr("fill", function(d,i){return z(i)})
          .attr("x", function(d,i){return 0})
          .attr("y", function(d,i){return 5 + legend_y(i)})
          .attr("width", legendWidth/3)
          .attr("height", legend_y.rangeBand());
        svg.selectAll("text")
          .data(series)
          .enter()
          .append("text")
          .attr("x", function(d,i){return (legendWidth/3)+10})
          .attr("y", function(d,i){return legend_y(i)})
          .attr("dy", legend_y.rangeBand())
          .attr("class", "mono")
          .text(function(d,i){
            if(i==0){
              return "this task";
            }else{
              return compareSamples[i-1].getLegendName()//series.legendName
            }
          });
      }
    }
  }

  $scope.getSampleAtCommit = function(series, commit) {
    return _.find(series, function(x){return x.revision == commit});
  }

  $scope.getCommits = function(seriesByName){
    // get a unique list of all the revisions in the test series, accounting for gaps where some tests might have no data,
    // in order of push time.
    return _.uniq(_.pluck(_.sortBy(_.flatten(_.values(seriesByName)), "order"), "revision"), true);
  }

  $scope.setTaskTag = function(keyEvent){
    if (keyEvent.which === 13){
      $http.post("/plugin/json/task/" + $scope.task.id + "/perf/tag", {tag:$scope.perfTagData.input}).then(
        function(resp){ $scope.perfTagData.tag = $scope.perfTagData.input},
        function(){ console.log("error")}
      );
    }
    return true
  }

  $scope.addComparisonForm = function(formData, draw){
    var commitHash = formData.hash
    var saveObj = {}
    if(commitHash){
      saveObj.hash = commitHash
    }else{
      saveObj.tag = formData.tag
    }
    if(!!formData.tag && !!formData.tag.tag){
      formData.tag = formData.tag.tag
    }
    $scope.savedCompares.push(saveObj)
    if(!!commitHash){
      $http.get("/plugin/json/commit/" + $scope.project + "/" + commitHash + "/" + $scope.task.build_variant + "/" + $scope.task.display_name + "/perf").then(
        function(resp){
          var d = resp.data;
          var compareSample = new TestSample(d);
          $scope.comparePerfSamples.push(compareSample)
          if(draw)
            $scope.redrawGraphs()
        },
        function(resp){console.log(resp.data) });
    }else if(!!formData.tag && formData.tag.length > 0){
      $http.get("/plugin/json/tag/" + $scope.project + "/" + formData.tag + "/" + $scope.task.build_variant + "/" + $scope.task.display_name + "/perf").then(
        function(resp){
          var d = resp.data;
          var compareSample = new TestSample(d);
          $scope.comparePerfSamples.push(compareSample)
          if(draw)
            $scope.redrawGraphs()
        },
        function(resp){console.log(resp.data) });
    }

    $scope.compareForm = {}
    $scope.syncHash(-1)
  }

  $scope.addComparisonHash = function(hash){
    $scope.addComparisonForm({hash:hash}, true)
  }

  $scope.updateCompares = function(){
  }

  $scope.redrawGraphs = function(){
      setTimeout(function(){
        drawDetailGraph($scope.perfSample, $scope.comparePerfSamples, $scope.task.id);
        drawTrendGraph($scope.trendSamples, $scope.perfSample.testNames(), $scope, $scope.task.id, $scope.comparePerfSamples);
      },0)
  }

  if($scope.conf.enabled){
    if($location.hash().length>0){
      try{
        var hashparsed = JSON.parse($location.hash())
        if('hiddenGraphs' in hashparsed){
          for(var i=0;i<hashparsed.hiddenGraphs.length;i++){
            $scope.hiddenGraphs[hashparsed.hiddenGraphs[i]]=true
          }
        }
        if('perftab' in hashparsed){
          $scope.perftab = hashparsed.perftab
        }
        if('compare' in hashparsed){
          for(var i=0;i<hashparsed.compare.length;i++){
            $scope.addComparisonForm(hashparsed.compare[i], false)
          }
        }
      }catch (e){ }
    }
    // Populate the graph and table for this task
    $http.get("/plugin/json/task/" + $scope.task.id + "/perf/").then(
      function(resp){
        var d = resp.data;
        $scope.perfSample = new TestSample(d);
        var w = 700;
        var bw = 1;
        var h = 100;
        if("tag" in d && d.tag.length > 0){
          $scope.perfTagData.tag = d.tag
        }
        setTimeout(function(){drawDetailGraph($scope.perfSample, $scope.comparePerfSamples, $scope.task.id)},0);

        // Populate the trend data
        $http.get("/plugin/json/history/" + $scope.task.id + "/perf").then(
          function(resp){
            var d = resp.data;
            $scope.trendSamples = new TrendSamples(d);
            setTimeout(function(){drawTrendGraph($scope.trendSamples, $scope.perfSample.testNames(), $scope, $scope.task.id,  $scope.comparePerfSamples)},0);
          });
      });

    $http.get("/plugin/json/task/" + $scope.task.id + "/perf/tags").then(
      function(resp){
        var d = resp.data;
        $scope.tags = d.sort(function(a,b){return a.tag.localeCompare(b.tag)})
    })

    if($scope.task.patch_info && $scope.task.patch_info.Patch.Githash){
      //pre-populate comparison vs. base commit of patch.
      $scope.addComparisonHash($scope.task.patch_info.Patch.Githash);
    }
  }
})

// Class to contain a collection of samples in a series.
function TrendSamples(samples){
  this.samples = samples;

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
  for (var i = 0; i < samples.length; i++) {
    for (var j = 0; j < samples[i].data.results.length; j++) {
      var name = samples[i].data.results[j].name;
      if (!(name in this.seriesByName)) {
        this.seriesByName[name] = [];
      }
      var rec = samples[i].data.results[j];
      var sorted = _.sortBy(_.filter(_.values(rec.results), function(x){return typeof(x)=="object"}), "ops_per_sec");
      this.seriesByName[name].push({
        revision: samples[i].revision,
        task_id: samples[i].task_id,
        "ops_per_sec": sorted[sorted.length-1].ops_per_sec,
        "ops_per_sec_values": sorted[sorted.length-1].ops_per_sec_values,
        order: samples[i].order,
      });
    }
  }

  for(key in this.seriesByName){
    this.seriesByName[key] = _.sortBy(this.seriesByName[key], 'order');
    this.testNames.unshift(key);
  }

  for(var i=0;i<this.testNames.length;i++){
    //make an index for commit hash -> sample for each test series
    var k = this.testNames[i];
    this._sampleByCommitIndexes[k] = _.groupBy(this.seriesByName[k], "revision"), function(x){return x[0]};
    for(t in this._sampleByCommitIndexes[k]){
      this._sampleByCommitIndexes[k][t] = this._sampleByCommitIndexes[k][t][0];
    }
  }

  // Returns a list of samples for a given test, sorted in the order that they were committed.
  this.tasksByCommitOrder = function(){
    if(!this._tasks){
      this._tasks = _.sortBy(_.uniq(_.flatten(_.values(this.seriesByName)), false,  function(x){return x.task_id}), "order");
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
    return this._sampleByCommitIndexes[testName][revision];
  }

  this.indexOfCommitInSeries = function(testName, revision){
    var t = this.tasksByCommitOrderByTestName(testName)
    return findIndex(t, function(x){x.revision==revision})
  }

  this.noiseAtCommit = function(testName, revision){
    var sample = this._sampleByCommitIndexes[testName][revision];
    if(sample && sample.ops_per_sec_values && sample.ops_per_sec_values.length > 1){
      var r = (_.max(sample.ops_per_sec_values) - _.min(sample.ops_per_sec_values)) / average(sample.ops_per_sec_values);
      return r;
    }
  }

}

function TestSample(sample){
  this.sample = sample;
  this._threads = null;
  this._maxes = {};

  this.threads = function(){
    if(this._threads == null){
      this._threads = _.uniq(_.filter(_.flatten(_.map(this.sample.data.results, function(x){ return _.keys(x.results) }), true), numericFilter));
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
    return _.pluck(_(testInfo.results).pairs().filter(function(x){return typeof(x[1]) == "object" && "ops_per_sec" in x[1]}), 0)
  }

  this.threadsVsOps = function(testName) {
    var testInfo = this.resultForTest(testName);
    var result = [];
    if (!testInfo)
      return;
    var series = testInfo.results;

    var keys = this.resultKeys(testName)
    for (var j = 0; j < keys.length; j++) {
      result.push({
        threads: parseInt(keys[j]),
        ops_per_sec: series[keys[j]].ops_per_sec,
        ops_per_sec_values: series[keys[j]].ops_per_sec_values,
      });
    }
    _.sortBy(result, "threads");
    return result;
  }

  this.resultForTest = function(testName){
      return _.find(this.sample.data.results, function(x){return x.name == testName});
  }

  this.maxThroughputForTest = function(testName){
    if(!_.has(this._maxes, testName)){
      var d = this.resultForTest(testName);
      if(!d){
        return;
      }
      this._maxes[testName] = _.max(_.filter(_.pluck(_.values(d.results), 'ops_per_sec'), numericFilter));
    }
    return this._maxes[testName];
  }


}

var drawTrendGraph = function(trendSamples, tests, scope, taskId, compareSamples) {
  scope.d3data = {}
  for (var i = 0; i < tests.length; i++) {
    var testNameIndex = i

    // clear out the DOM element containing the graph for this task
    $("#perf-trendchart-" + cleanId(taskId) + "-" + i).empty();
    var margin = { top: 20, right: 50, bottom: 30, left: 50 }
    var width = 960 - margin.left - margin.right;
    var height = 200 - margin.top - margin.bottom;

    var key = tests[i];
    var svg = d3.select("#perf-trendchart-" + cleanId(taskId) + "-" + i)
      .append("svg")
      .attr('class', "series")
      .attr("width", width + margin.left + margin.right)
      .attr("height", height + margin.top + margin.bottom);
    var series = trendSamples.seriesByName[key];
    var ops = _.pluck(series, 'ops_per_sec');
    var opsValues = _.pluck(series, 'ops_per_sec_values');
    var hasValues = !_.contains(opsValues, undefined)

    var seriesMax = d3.max(ops)
    var seriesAvg = d3.mean(ops)

    var compareMax = 0
    if(compareSamples){
        compareMax = d3.max(_.map(compareSamples, function(x){return x.maxThroughputForTest(key)}))
    }

    // If the upper and lower y-axis values are very close to the average
    // (within 10%) add extra padding to the upper and lower bounds of the graph for display
    var yAxisUpperBound = d3.max([compareMax, seriesMax, seriesAvg*1.1])
    var yAxisLowerBound = d3.min([d3.min(ops), seriesAvg*.9])

    // create extra padding if seriesMax

    var y = d3.scale.linear()
      .domain([yAxisLowerBound, yAxisUpperBound])
      .range([height, 0]);
    var x = d3.scale.linear()
      .domain([0, ops.length - 1])
      .range([0, width]);

    var z = d3.scale.category10();

    var line = d3.svg.line()
      .x(function(d, i) {
        return x(i);
      })
      .y(function(d) {
        return y(d.ops_per_sec)
      });
    svg.append("path")
      .data([series])
      .attr("class", "line")
      .attr("d", line);

    if(hasValues){
      var maxline = d3.svg.line()
        .x(function(d, i){
          return x(i);
        })
        .y(function(d){
          return y(d3.max(d.ops_per_sec_values))
        })

      var minline = d3.svg.line()
        .x(function(d, i){
          return x(i);
        })
        .y(function(d){
          return y(d3.min(d.ops_per_sec_values))
        })

      svg.append("path")
        .data([series])
        .attr("class", "error-line")
        .attr("d", maxline);
      svg.append("path")
        .data([series])
        .attr("class", "error-line")
        .attr("d", minline);
    }

    var focus = svg.append("circle")
      .attr("r", 4.5);
    scope.d3data[key] = { x:x, y:y, z:z, focus:focus }

    svg.selectAll(".point")
      .data(series)
      .enter()
      .append("svg:circle")
      .attr("class", function(d){
        if(d.task_id == scope.task.id){
          return "point current"
        }else if(!!scope.comparePerfSample && d.revision == scope.comparePerfSample.sample.revision){
          return "point compare"
        }
        return "point";
      })
      .attr("cx", function(d, i) {
        return x(i);
      })
      .attr("cy", function(d) {
        return y(d.ops_per_sec);
      })
      .attr("r", function(d){
        return d.task_id == scope.task.id ? 5 : 2;
      });
    svg.append("rect")
      .attr("class", "overlay")
      .attr("y", margin.top)
      .attr("width", width)
      .attr("height", height)
      .on("mouseover", function() {
        focus.style("display", null);
      })
      .on("mouseout", function() {
        focus.style("display", "none");
      })
      .on("click", function(s){
        return function(){
          s.locked = !s.locked
          scope.$digest()
        }
      }(scope))
      .on("mousemove", function(data, f, xscale, scope, series, ts) {
        return function() {
          if(scope.locked){
            return;
          }

          var i = Math.round(xscale.invert(d3.mouse(this)[0]));
          var hash = data[i].revision
          if(hash == scope.currentHash){
            return
          }
          for(var q=0;q<tests.length;q++){
            var d = scope.d3data[tests[q]]
            var index = findIndex(ts.tasksByCommitOrderByTestName(tests[q]), function(x){return x.revision==hash})
            var tempSample = ts.sampleInSeriesAtCommit(tests[q], hash)
            if(index && tempSample){
              d.focus.attr("cx", d.x(index)).attr("cy", d.y(tempSample.ops_per_sec))
            }else{
              d.focus.attr("cx", 0).attr("cy", 0)
            }
          }
          scope.currentSample = data[i];
          scope.currentHash = data[i].revision;
          scope.currentHoverSeries = series;
          scope.$digest();
        }
      }(series, focus, x, scope, key, trendSamples))

    var avgOpsPerSec = d3.mean(ops)
    if (compareSamples) {
      for(var j=0;j<compareSamples.length;j++){
        var compareSample = compareSamples[j]
        var compareMax = compareSample.maxThroughputForTest(key)
        if (!isNaN(compareMax)) {
          var compareLine = d3.svg.line()
            .x(function(d, i) {
              return x(i);
            })
            .y(function(d) {
              return y(compareMax);
            })

          svg.append("line")
            .attr("stroke", function(d,i){return z(j+1)})
            .attr("stroke-width", "1")
            .attr("stroke-dasharray", "5,5")
            .attr("class", "mean-line")
            .attr({
              x1: x(0),
              x2: x(width),
              y1: y(compareMax),
              y2: y(compareMax)
            })
        }
      }
    }

    var padding = 30
    var yAxis = d3.svg.axis()
      .scale(y)
      .orient("left")
      .ticks(5);
    svg.append("g")
      .attr("class", "axis")
      .call(yAxis);
  }

}
