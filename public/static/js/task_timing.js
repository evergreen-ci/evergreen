mciModule.controller('TaskTimingController', function($scope, $http, $window, $filter, $locationHash, mciTime, notificationService) {
    $scope.currentProject = $window.activeProject;
    // sort the task names for the current project
    $scope.currentProject.task_names.sort()
    $scope.currentProject.build_variants.sort(function(a,b){
        return (a.name < b.name) ? -1 : 1
    });
    $scope.currentBV = "";
    $scope.currentTask = "";
    $scope.currentHover = -1;

    var allTasksField = "All Tasks";
    var time_taken = "tt";
    var makespan = "makespan";
    var processing_time = "tpt";
    var repotracker_requester = "gitter_request";
    var patch_requester = "patch_request"
    var nsPerMs = 1000000;

    var initialHash = $locationHash.get();
    // TODO do we keep this?
    if (initialHash.buildVariant) {
        for (var i = 0; i < $scope.currentProject.build_variants.length; ++i) {
            if ($scope.currentProject.build_variants[i].name === initialHash.buildVariant) {
                $scope.currentBV = $scope.currentProject.build_variants[i];
            }
        }
    } else {
        // check if there are no build variants in the project
        if ($scope.currentProject.build_variants != []) {
            $scope.currentBV = $scope.currentProject.build_variants[0];
        }
    }

    // try to get the task from the current build variant,
    // if there isn't one then get the first task
    if(initialHash.taskName){
        $scope.currentTask = initialHash.taskName
    } else {
        if ($scope.currentProject.task_names != []) {
            if ($scope.currentBV.task_names != []) {
                $scope.currentTask = $scope.currentBV.task_names[0];
            } else {
                $scope.currentTask = $scope.currentProject.task_names[0];
            }
        }
    }
    if (initialHash.onlySuccessful === true) {
      $scope.onlySuccessful = true;
    } else {
      $scope.onlySuccessful = false;
    }

    $scope.taskData = {};
    $scope.locked = false;
    $scope.hoverInfo = {hidden: true};
    $scope.allTasks = false;

    $scope.requestViewOptions = [
        {
            name: "Commits",
            requester: repotracker_requester
        },
        {
            name: "Patches",
            requester : patch_requester
        }
    ];

    // use the location hash for patches vs commit view
    if (initialHash.requester) {
      r = initialHash.requester;
      if (r == repotracker_requester){
        $scope.currentRequest = $scope.requestViewOptions[0];
      }
      if (r == patch_requester){
        $scope.currentRequest = $scope.requestViewOptions[1];
      }
    }  else {
      $scope.currentRequest = $scope.requestViewOptions[0];
    }

    $scope.setCurrentRequest = function(requestView) {
        $scope.currentRequest = requestView;
        $scope.load()
    }

    // normal task options
    $scope.timeDiffOptions = [
        {
            name: "Start \u21E2 Finish",
            diff: ["finish_time", "start_time"]

        },
        {
            name: "Scheduled \u21E2 Start",
            diff: ["start_time", "scheduled_time"]
        },
        {
            name: "Scheduled \u21E2 Finish",
            diff: ["finish_time", "scheduled_time"]
        }
    ];

    $scope.timeDiff = $scope.timeDiffOptions[0];

    $scope.setTimeDiff = function(diff) {
        $scope.timeDiff = diff;
        $scope.load();
    }

    // options for the all tasks functionality
    $scope.allTasksOptions = [
        {
            name: "Makespan",
            type: makespan,

        },
        { name: "Total Processing Time",
            type: processing_time,
        },
    ];
    $scope.allTasksView = $scope.allTasksOptions[0];


    $scope.setAllTasksView = function(view) {
        $scope.allTasksView = view;
        $scope.load();
    }

    $scope.isAllTasks = function(){
      return $scope.allTasks || $scope.currentTask == "All Tasks";
    }


    if (initialHash.limit) {
      $scope.numTasks = initialHash.limit;
    } else {
      $scope.numTasks = 50;
    }
    $scope.numTasksOptions = [25, 50, 100, 200, 500, 1000, 2000];


    $scope.setNumTasks = function(num){
        $scope.numTasks = num;
        $scope.load();
    }

    // add an all tasks field
    $scope.currentProject.task_names.unshift(allTasksField);



    $scope.setBuildVariant = function(bv) {
        $scope.currentBV = bv;
        $scope.load();

    };

    $scope.setTaskName = function(task) {
        if (task == allTasksField){
            $scope.allTasks = true;
            $scope.currentTask = task;
            $scope.load();
            return
        }
        $scope.allTasks = false;
        $scope.currentTask = task;
        $scope.load();
    }


    // check that task is in list of build variants
    $scope.checkTaskForGraph = function (task) {
        if (task == allTasksField){
            return true;
        }
        return _.some($scope.currentBV.task_names, function(name){ return name == task});
    }


    $scope.getLink = function() {
        if ($scope.isAllTasks()) {
            return "/build/" + $scope.hoverInfo.id;
        } else {
            return "/task/" + $scope.hoverInfo.id;
        }
    }

    // filter function to remove zero times from a list of times
    var nonZeroTimeFilter = function(y){return (+y) != (+new Date(0))}

    var isPatch = function(){
        return $scope.currentRequest.requester == patch_requester;
    }

    var getHoverInfo = function(i){
        var hoverInfo;
        if (isPatch()){
            hoverInfo =  {
                "revision" : $scope.versions[i].Revision,
                "duration" : formatDuration(yMap($scope.taskData[i])),
                "id" : $scope.taskData[i].id,
                "message" : $scope.versions[i].Description,
                "author": $scope.versions[i].Author,
                "create_time": $scope.versions[i].CreateTime,
                "hidden": false
            }
        } else {
            hoverInfo =  {
                "revision" : $scope.versions[i].revision,
                "duration" : formatDuration(yMap($scope.taskData[i])),
                "id" : $scope.taskData[i].id,
                "message" : $scope.versions[i].message,
                "author": $scope.versions[i].author,
                "create_time": $scope.versions[i].create_time,
                "hidden": false
            }
        }
        if (!$scope.isAllTasks()){
          hoverInfo.host = $scope.taskData[i].host
          hoverInfo.distro = $scope.taskData[i].distro
        }
        return hoverInfo
    }

    function calculateMakespan(build) {
        var tasks = build.tasks;
        if (!tasks) {
            return 0;
        }
        // extract the start an end times for the tasks in the build, discarding the zero times
        var taskStartTimes = _.filter(_.pluck(tasks, "start_time").map(function(x){return new Date(x)}), nonZeroTimeFilter).sort();
        var taskEndTimes = _.filter(tasks.map(function(x){
            if(x.time_taken == 0 || +new Date(x.start_time) == +new Date(0)){
                return new Date(0);
            }else {
              return new Date((+new Date(x.start_time)) + (x.time_taken/nsPerMs));
            }
          }), nonZeroTimeFilter).sort();


        if(taskStartTimes.length == 0 || taskEndTimes.length == 0) {
          return 0;
        } else {
          var makeSpan = taskEndTimes[taskEndTimes.length-1] - taskStartTimes[0];
          if (makeSpan < 0) {
            return 0;
          }
          return makeSpan;
        }

      }

      function calculateTotalProcessingTime (build) {
        var tasks = _.filter(build.tasks, function(task){return nonZeroTimeFilter(new Date(task.start_time));})
        return mciTime.fromNanoseconds(_.reduce(tasks, function(sum, task){return sum + task.time_taken}, 0));
      }


      var xMap = function(task){
        return moment(task.create_time);
      }

      var yMap  = function(task){
        if ($scope.isAllTasks()){
         if ($scope.allTasksView.type == makespan) {
          return calculateMakespan(task);
        } else {
          return calculateTotalProcessingTime(task);
        }
      }
      var a1 = moment(task[$scope.timeDiff.diff[0]]);
      var a2 = moment(task[$scope.timeDiff.diff[1]]);
      return mciTime.fromMilliseconds(a1.diff(a2));
    }

    // isValidDate checks that none of the start, finish or scheduled times are zero
    // and that the span from start-end is >= 1 sec in order to reduce noise
    var isValidDate = function(task){
      var start = moment(task.start_time);
      var end = moment(task.finish_time);

      return nonZeroTimeFilter(start) &&
             nonZeroTimeFilter(end) &&
             nonZeroTimeFilter(+new Date(task.scheduled_time)) &&
             end.diff(start, 'seconds') !== 0; // moment.diff will round toward 0 so this discards tasks that took <1 sec
    }

    $scope.load = function(before) {
      $scope.hoverInfo.hidden = true;
      $scope.locked = false;
      // check that task exists in build variant
      if (!$scope.checkTaskForGraph($scope.currentTask) && !$scope.isAllTasks()){
          return;
      }

      var limit = $scope.numTasks;
      var query = (!!before ? 'before=' + encodeURIComponent(before) : '');
      query += (query.length > 0 && !!limit ? '&' : '');
      query += (!!limit ? 'limit=' + encodeURIComponent(limit) : '');
      query += $scope.onlySuccessful ? "&onlySuccessful=true" : "";
      url = '/json/task_timing/' +
          encodeURIComponent($scope.currentProject.name) + '/' +
          encodeURIComponent($scope.currentBV.name) + '/' +
          encodeURIComponent($scope.currentRequest.requester) + '/' +
          encodeURIComponent($scope.currentTask)
      $http.get(
      url + '?' + query).
      success(function(data) {
          $scope.taskData = ($scope.isAllTasks()) ? data.builds.reverse() : data.tasks.reverse();
          $scope.versions = ($scope.currentRequest.requester == repotracker_requester) ? data.versions.reverse() : data.patches.reverse();
          $scope.versions = _.filter($scope.versions, function(v, i){
              return isValidDate($scope.taskData[i]);
          })
          $scope.taskData = _.filter($scope.taskData, isValidDate)
          setTimeout(function(){$scope.drawDetailGraph()},0);
      }).
      error(function(data) {
          notificationService.pushNotification("Error loading data: `" + data.error+"`", 'errorHeader', "error");
          $scope.taskData = [];
      });

      setTimeout(function(p, bv, t, r, l, o){
          return function(){
              $locationHash.set({ project: p, buildVariant: bv, taskName: t, requester:r, limit: l, onlySuccessful: o});
          }
      }($scope.currentProject.name, $scope.currentBV.name, $scope.currentTask, $scope.currentRequest.requester, $scope.numTasks, $scope.onlySuccessful), 0)
    };

    // formatting function for the way the y values should show up.
    // TODO: figure out how to make this look better...
    var formatDuration = function(d) {
        if (d == 0) {
            return "0s"
        }
        return $filter('stringifyNanoseconds')(d * nsPerMs, true, true)
    }

    $scope.drawDetailGraph = function(){
        var graphId = "#tt-graph"
        $(graphId).empty();

        // get the width of the column div for the width of the graph
        var graph = d3.select(graphId)[0][0];
        var colWidth = graph.clientWidth;

        var margin = { top: 20, right: 60, bottom: 100, left: 110 };
        var width = colWidth - margin.left - margin.right;
        var height = (colWidth/2) - margin.top - margin.bottom;
        var svg = d3.select(graphId)
        .append("svg")
        .attr("width", width + margin.left + margin.right)
        .attr("height", height + margin.top + margin.bottom)
        .append("g")
        .attr("transform", "translate(" + margin.left + "," + margin.top + ")");


        // sort task data by create time
        $scope.taskData.sort(function(a,b){
            return moment(a.create_time).diff(moment(b.create_time));
        })



        var maxTime = d3.max($scope.taskData, function(task){return yMap(task);});
        var minTime = d3.min($scope.taskData, function(task){ return yMap(task);});

        var yScale = d3.scale.linear()
        .domain([minTime, maxTime])
        .range([height, 0]);

        var xScale = d3.scale.linear()
        .domain([0, $scope.taskData.length - 1 ])
        .range([0, width])

        var yAxis = d3.svg.axis()
        .scale(yScale)
        .orient("left")
        .tickFormat(formatDuration)
        .ticks(10)
        .tickSize(-width)

        var xAxis = d3.svg.axis()
        .scale(xScale)
        .orient("bottom")
        .ticks($scope.taskData.length)
        .tickFormat(function(d, i){
            // every 5 ticks
            if ($scope.numTasks >= 200 && i % 5 != 0) {
                return "";
            }
            // every 2 ticks
            if ($scope.numTasks >= 100 && i % 2 != 0){
                return "";
            }
            var task = $scope.taskData[d];
            if (task) {
                return moment(task.create_time).format("M/D")}
                return ""
        })
        .tickSize(10)

        // Define the line
        var valueline = d3.svg.line()
        .x(function(d, i) { return xScale(i);})
        .y(function(d) { return yScale(yMap(d)); });

        svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + height + ")")
        .call(xAxis);
        svg.append("g")
        .attr("class", "y axis")
        .call(yAxis);

        // now rotate text on x axis
        svg.selectAll(".x text")  // select all the text elements for the xaxis
        .attr("transform", function(d) {
            return "translate(" + this.getBBox().height*-2 + "," + (this.getBBox().height ) + ")rotate(-45)";
        })
        .attr("font-size", 10)


        var scaledX = function(x, i){return xScale(i);}
        var scaledY = function(y){return yScale(yMap(y));}

        // Add the valueline path.
        svg.append("path")
        .attr("class", "line")
        .attr("d", valueline($scope.taskData));

        // add the tooltip area to the webpage
        var tooltip = d3.select("body").append("div")
        .attr("class", "tooltip")
        .style("opacity", 0);

        var radius = $scope.numTasks >= 100 ? 1 : 3.5;
        // draw dots
        svg.selectAll(".task-circle")
        .data($scope.taskData)
        .enter().append("circle")
        .attr("class", "task-circle")
        .attr("r", radius)
        .attr("cx", scaledX)
        .attr("cy", scaledY)
        .style("stroke", function(task){
            return task.status == 'success' ? 'green' : 'red';
        })

        // create a focus circle
        var focus = svg.append("circle")
        .attr("r", radius + 1);


        // create a transparent rectangle for tracking hovering
        svg.append("rect")
        .attr("class", "overlay")
        .attr("y", 0)
        .attr("width", width)
        .attr("height", height + margin.top)
        .on("mouseover", function() {
            focus.style("display", null);
        })
        .on("mouseout", function() {
            focus.style("display", "none");
        })
        // lock the scope if you click in the rectangle
        .on("click", function(scope){
            return function(){
                scope.locked = !scope.locked
                scope.$digest()
            }
        }($scope))
        .on("mousemove", function(){
            if($scope.locked){
                return;
            }
            var i = Math.round(xScale.invert(d3.mouse(this)[0]));
            // if the revision is already there then return
            if(i == $scope.currentHover){
                return
            }

            // set the revision to be the current hash
            $scope.currentHover = i;
            $scope.hoverInfo = getHoverInfo(i);

            // set the focus's location to be the location of the closest point
            focus.attr("cx", scaledX($scope.taskData[i], i))
            .attr("cy", scaledY($scope.taskData[i]));
            $scope.$digest();
        });
    }

    $scope.load();
});
