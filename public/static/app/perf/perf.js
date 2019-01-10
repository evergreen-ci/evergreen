var numericFilter = function(x) {
  return !_.isNaN(parseInt(x))
}

// since we are using an older version of _.js that does not have this function
var findIndex = function(list, predicate) {
  for(var i=0;i<list.length;i++){
    if(predicate(list[i])){
      return i
    }
  }
}

mciModule.controller('PerfController', function PerfController(
  $scope, $window, $http, $location, $log, $q, ChangePointsService,
  DrawPerfTrendChart, PROCESSED_TYPE, Settings, Stitch, STITCH_CONFIG,
  TestSample, TrendSamples
) {
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

  // set this to false if we want the display to happen after the plots are rendered
  $scope.showToolbar = true
  $scope.hiddenGraphs = {}
  $scope.compareItemList = []
  $scope.perfTagData = {}
  $scope.compareForm = {}
  $scope.savedCompares = []
  $scope.jiraHost = $window.jiraHost

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
    var locationHash = decodeURIComponent($location.hash());
    if(locationHash.length > 0) {
      hash = JSON.parse(locationHash)
    }
    if(Object.keys($scope.hiddenGraphs).length > 0){
      hash.hiddenGraphs = Object.keys($scope.hiddenGraphs)
    }
    if(tab >= 0){
      hash.perftab = tab
    }

    if($scope.savedCompares.length > 0){
      hash.compare = $scope.savedCompares
    }else{
      delete hash.compare
    }

    if ($scope.metricSelect.value != $scope.metricSelect.default) {
      hash.metric = $scope.metricSelect.value.key
    }

    setTimeout(function(){
      $location.hash(encodeURIComponent(JSON.stringify(hash)))
      $scope.$apply()
    }, 1)
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
  $scope.user = $window.user
  $scope.Math = $window.Math;
  $scope.conf = $window.plugins["perf"];
  $scope.task = $window.task_data;
  $scope.tablemode = 'maxthroughput';
  $scope.threadLevelsRadio = {
    options: [
      {key: 'maxonly', val: 'Max Only'},
      {key: 'all', val: 'All'}
    ],
    value: Settings.perf.trendchart.threadLevelMode,
  }

  $scope.metricSelect = {
    options: [],
    default: {key: 'ops_per_sec', name: 'ops/sec (Default)'},
    value: undefined,
  }
  $scope.metricSelect.options.push($scope.metricSelect.default)
  $scope.metricSelect.value = $scope.metricSelect.default

  // perftab refers to which tab should be selected. 0=graph, 1=table, 2=trend, 3=trend-table
  $scope.perftab = 2;
  $scope.project = $window.project;
  $scope.compareHash = "ss";
  $scope.comparePerfSamples = [];

  // Linear or Log Scale
  $scope.scaleModel = {
    name: 'Linear',
    linearMode: Settings.perf.trendchart.linearMode.enabled,
  }
  $scope.rangeModel = {
    name: 'Origin',
    originMode: Settings.perf.trendchart.originMode.enabled,
  }

  $scope.toolBar = {
    isOpen: false
  }

  $scope.$watch('scaleModel.linearMode', function(newVal, oldVal) {
    // Force comparison by value
    if (oldVal === newVal) return;
    Settings.perf.trendchart.linearMode.enabled = newVal
    $scope.redrawGraphs()
  })

  $scope.$watch('rangeModel.originMode', function(newVal, oldVal) {
    // Force comparison by value
    if (oldVal === newVal) return;
    Settings.perf.trendchart.originMode.enabled = newVal
    $scope.redrawGraphs()
  })

  $scope.$watch('threadLevelsRadio.value', function(newVal, oldVal) {
    // Force comparison by value
    if (oldVal === newVal) return;
    Settings.perf.trendchart.threadLevelMode = newVal
    $scope.redrawGraphs()
  })

  $scope.$watch('metricSelect.value', function(newVal, oldVal) {
    // Force comparison by value
    if (oldVal === newVal) return;
    $scope.syncHash(-1)
    $scope.redrawGraphs()
  })

  $scope.$watch('toolBar.isOpen', function(newVal, oldVal) {
    d3.selectAll('md-fab-toolbar').style('pointer-events', newVal ? 'all' : 'none')
  })

  $scope.$watch('currentHash', function(){
    $scope.hoverSamples = {}
    $scope.dateLabel = moment($scope.currentHashDate).format('ll')
    if(!!$scope.perfSample){
      var testNames = $scope.perfSample.testNames()

      for(var i=0;i<testNames.length;i++){
        var s = $scope.trendSamples.sampleInSeriesAtCommit(
          testNames[i], $scope.currentHash
        )
        $scope.hoverSamples[testNames[i]] = s
      }
    }
  })

  var drawTrendGraph = function(scope) {
    scope.locked = false;
    // Extract params
    var trendSamples = scope.trendSamples,
        tests = scope.perfSample.testNames(),
        taskId = scope.task.id,
        compareSamples = scope.comparePerfSamples;

    // Creates new, non-isolated scope for charts
    var chartsScope = scope.$new()
    for (var i = 0; i < tests.length; i++) {
      var key = tests[i];
      var series = trendSamples.seriesByName[key];
      var containerId = 'perf-trendchart-' + cleanId(taskId) + '-' + i;
      var cps = scope.changePoints
      var bfs = scope.buildFailures

      DrawPerfTrendChart({
        series: series,
        // Concat orfaned change points and build failures
        changePoints: (cps[key] || []).concat(cps[undefined] || []),
        buildFailures: (bfs[key] || []).concat(bfs[undefined] || []),
        key: key,
        scope: chartsScope,
        containerId: containerId,
        compareSamples: compareSamples,
        threadMode: scope.threadLevelsRadio.value,
        linearMode: scope.scaleModel.linearMode,
        originMode: scope.rangeModel.originMode,
        metric: scope.metricSelect.value.key,
      })
    }
    scope.showToolbar = true
  }

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
    var compareTest = _.find(sample.data.results, function(x) {
      return x.name == testKey
    });
    var compareMaxOps = $scope.getMax(compareTest.results);
    var pctDiff = (referenceOps-compareMaxOps)/referenceOps;
    return pctDiff;
  }

  $scope.getMax = function(r){
    return _.max(_.filter(_.pluck(_.values(r), 'ops_per_sec'), numericFilter));
  }

  let cleanId = function(id){
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
            return ("ops_per_sec_values" in d) && (d.ops_per_sec_values != undefined && d.ops_per_sec_values.length > 1);
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

  function markChangePoints(points, mark) {
    ChangePointsService.markPoints(points, mark).then(function() {
      // Acked points should remain (at least for now)
      if (mark != PROCESSED_TYPE.ACKNOWLEDGED) {
        $scope.$emit('changePointsRemove', _.pluck(points, 'suspect_revision'))
      }
    }, _.noop)
  }

  $scope.ackChangePoints = function(points) {
    markChangePoints(points, PROCESSED_TYPE.ACKNOWLEDGED)
  }

  $scope.hideChangePoints = function(points) {
    markChangePoints(points, PROCESSED_TYPE.HIDDEN)
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
        function(resp){ console.log(resp.data) });
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
        drawTrendGraph($scope);
        drawDetailGraph($scope.perfSample, $scope.comparePerfSamples, $scope.task.id);
      }, 0)
  }

  if ($scope.conf.enabled){
    if ($location.hash().length>0){
      try {
        var hashparsed = JSON.parse(decodeURIComponent($location.hash()))
        if ('hiddenGraphs' in hashparsed){
          for (let i = 0; i < hashparsed.hiddenGraphs.length; i++) {
            $scope.hiddenGraphs[hashparsed.hiddenGraphs[i]]=true
          }
        }
        if ('perftab' in hashparsed){
          $scope.perftab = hashparsed.perftab
        }
        if ('compare' in hashparsed){
          for (let i = 0; i < hashparsed.compare.length; i++) {
            $scope.addComparisonForm(hashparsed.compare[i], false)
          }
        }
      } catch (e) { }
    }
    // Populate the graph and table for this task
    $http.get("/plugin/json/task/" + $scope.task.id + "/perf/").then(
      function(resp){
        var d = resp.data;
        $scope.perfSample = new TestSample(d);
        if("tag" in d && d.tag.length > 0){
          $scope.perfTagData.tag = d.tag
        }
        setTimeout(function(){drawDetailGraph($scope.perfSample, $scope.comparePerfSamples, $scope.task.id)},0);

        // This code loads change points for current task from the mdb cloud
        var changePointsQ = Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
          return db
            .db(STITCH_CONFIG.PERF.DB_PERF)
            .collection(STITCH_CONFIG.PERF.COLL_UNPROCESSED_POINTS)
            .find({
              project: $scope.task.branch,
              task: $scope.task.display_name,
              variant: $scope.task.build_variant,
            })
            .execute()
        }).then(
          function(docs) {
            return _.groupBy(docs, 'test')
          }, function(err) {
            $log.error('Cannot load change points!', err)
            return {} // Try to recover an error
        }).then(function(data) {
          $scope.changePoints = data
        })

        var buildFailuresQ = Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
          return db
            .db(STITCH_CONFIG.PERF.DB_PERF)
            .collection(STITCH_CONFIG.PERF.COLL_BUILD_FAILURES)
            .aggregate([
              {$match: {
                project: $scope.task.branch,
                tasks: $scope.task.display_name,
                buildvariants: $scope.task.build_variant,
              }},
              // Denormalization
              {$unwind: {path: '$tests', preserveNullAndEmptyArrays: true}},
              {$unwind: {path: '$first_failing_revision', preserveNullAndEmptyArrays: true}},
            ])
        }).then(
          function(docs) {
            return _.groupBy(docs, 'tests')
          }, function(err) {
            $log.error('Cannot load build failures!', err)
            return {} // Try to recover an error
        }).then(function(data) {
          $scope.buildFailures = data
        })

        // Populate the trend data
        var chartDataQ = $http.get("/plugin/json/history/" + $scope.task.id + "/perf").then(
          function(resp) {
            $scope.trendSamples = new TrendSamples(resp.data);
            $scope.metricSelect.options = [$scope.metricSelect.default].concat(
              _.map(
                _.without($scope.trendSamples.metrics, $scope.metricSelect.default.key),
                function(d) { return {key: d, name: d} }
              )
            )

          // Some copypasted checks
          if ($scope.conf.enabled){
            if ($location.hash().length>0){
              try {
                if ('metric' in hashparsed) {
                  let metric = hashparsed.metric
                  $scope.metricSelect.value = _.findWhere(
                    $scope.metricSelect.options, {key: metric}
                  ) || $scope.metricSelect.default
                }
              } catch (e) {}
            }
          }
          })

        // Once trend chart data and change points get loaded
        $q.all([chartDataQ, changePointsQ.catch(), buildFailuresQ.catch()])
          .then(function(ret) {
            setTimeout(function() {
              drawTrendGraph($scope)
            }, 0)
          })
      })

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
