function TaskTimingController($scope, $http, $window, $filter, $locationHash, mciTime, errorPasserService) {

  $scope.currentProject = $window.activeProject;
  $scope.currentBV = "";
  $scope.currentTask = "";
  $scope.currentHover = -1;

  var allTasksField = "All Tasks";
  var time_taken = "tt";
  var makespan = "makespan";
  var processing_time = "tpt";
  var nsPerMs = 1000000;


  $scope.taskData = {};
  $scope.locked = false;
  $scope.hoverInfo = {hidden: true};
  $scope.allTasks = false;
  
  $scope.requestViewOptions = [
    {
    name: "Commits",
    requester: "gitter_request"
  },
  {
    name: "Patches",
    requester : "patch_request"
  }
];

$scope.currentRequest = $scope.requestViewOptions[0];
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
    name: "Time Taken",
    type: time_taken,
  },
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


$scope.numTasks = 50;
$scope.numTasksOptions = [25, 50, 100, 200, 500, 1000, 2000];


$scope.setNumTasks = function(num){
  $scope.numTasks = num;
  $scope.load();
}

var initialHash = $locationHash.get();

  // check if there are no build variants in the project
  if ($scope.currentProject.build_variants != []) {
    $scope.currentBV = $scope.currentProject.build_variants[0];
  }

  // try to get the task from the current build variant, 
  // if there isn't one then get the first task 
  if ($scope.currentProject.task_names != []) {
    if ($scope.currentBV.task_names != []) {
      $scope.currentTask = $scope.currentBV.task_names[0]; 
    } else {
      $scope.currentTask = $scope.currentProject.task_names[0];
    }
  }

  // add an all tasks field
  $scope.currentProject.task_names.unshift(allTasksField);


  // TODO do we keep this? 
  if (initialHash.buildVariant) {
    for (var i = 0; i < $scope.currentProject.build_variants.length; ++i) {
      if ($scope.currentProject.build_variants[i].Name === initialHash.buildVariant) {
        $scope.currentBV = $scope.currentProject.build_variants[i];
        console.log("$$$ " + $scope.buildVariant.Name);
      }
    }
  }


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
    if ($scope.allTasks) {
      return "/build/" + $scope.hoverInfo.id;
    } else {
      return "/task/" + $scope.hoverInfo.id;
    }
  }

  // filter function to remove zero times from a list of times
  var nonZeroTimeFilter = function(y){return (+y) != (+new Date(0))}

  function calculateMakespan(build) {
    var tasks = build.tasks;
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
    var tasks = build.tasks;
    return mciTime.fromNanoseconds(_.reduce(tasks, function(sum, task){return sum + task.time_taken}, 0));
  }


  $scope.load = function(before) {

    $scope.hoverInfo.hidden = true;
    $scope.locked = false;
    // check that task exists in build variant
    if (!$scope.checkTaskForGraph($scope.currentTask) && !$scope.allTasks){
      return;
    }
    
    var limit = $scope.numTasks;
    var query = (!!before ? 'before=' + encodeURIComponent(before) : '');
    query += (query.length > 0 && !!limit ? '&' : '');
    query += (!!limit ? 'limit=' + encodeURIComponent(limit) : '');
    url = '/json/task_timing/' +
    encodeURIComponent($scope.currentProject.name) + '/' +
    encodeURIComponent($scope.currentBV.name) + '/' +
    encodeURIComponent($scope.currentRequest.requester)
    if (!$scope.allTasks) {
      url += '/' + encodeURIComponent($scope.currentTask) 
    }
    $http.get(
      url + '?' + query).
    success(function(data) {
      $scope.taskData = data.tasks.reverse();
      $scope.versions = data.versions.reverse();
      $scope.current_time = mciTime.fromNanoseconds(data.current_time);
      console.log($scope.taskData);

      setTimeout(function(){$scope.drawDetailGraph()},0);
    }).
    error(function(data) {
      errorPasserService.pushError("Error loading data: `" + data.error+"`", 'errorHeader');
      $scope.taskData = [];
    });

    $locationHash.set({
      project: $scope.currentProject.name,
      buildVariant: $scope.currentBV.name,
      taskName: $scope.currentTaskName,
    });
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

    var margin = { top: 20, right: 60, bottom: 100, left: 100 };
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

    var xMap = function(task){
      return moment(task.create_time);
    }

    var yMap  = function(task){
      if ($scope.allTasks){ 
        if ($scope.allTasksView.type == time_taken) {
          if (task.time_taken){
            return mciTime.fromNanoseconds(task.time_taken)
          } else {
            return 0
          }
        } else if ($scope.allTasksView.type == makespan) {
          return calculateMakespan(task);
        } else {
          return calculateTotalProcessingTime(task);
        }
      }
      var a1 = moment(task[$scope.timeDiff.diff[0]]);
      var a2 = moment(task[$scope.timeDiff.diff[1]]);
      return mciTime.fromMilliseconds(a1.diff(a2));
    }

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
    .tickFormat(function(d){
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

    // draw dots
    svg.selectAll(".task-circle")
    .data($scope.taskData)
    .enter().append("circle")
    .attr("class", "task-circle")
    .attr("r", 3.5)
    .attr("cx", scaledX)
    .attr("cy", scaledY)
    .style("stroke", function(task){
      return task.status == 'success' ? 'green' : 'red';
    })

    // create a focus circle 
    var focus = svg.append("circle")
    .attr("r", 4.5);


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
      .on("click", function(){
        $scope.locked = !$scope.locked
        $scope.$digest() 
      })
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
          $scope.hoverInfo = {
            "revision" : $scope.versions[i].revision,
            "duration" : formatDuration(yMap($scope.taskData[i])),
            "id" : $scope.taskData[i].id,
            "message" : $scope.versions[i].message,
            "author": $scope.versions[i].author,
            "create_time": $scope.versions[i].create_time,
            "hidden": false
          }

          // set the focus's location to be the location of the closest point
          focus.attr("cx", scaledX($scope.taskData[i], i))
          .attr("cy", scaledY($scope.taskData[i]));

          $scope.$digest();
        });          
    }

    $scope.load();

  };

