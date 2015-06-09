function TaskTimingController($scope, $http, $window, $filter, $locationHash) {
  $scope.allProjects = $window.allProjects;
  var initialHash = $locationHash.get();

  $scope.currentProject = $window.activeProject;
  var setProject = initialHash.project || $window.project;


  _.each($scope.allProjects, function(project) {
    if (project.Name === setProject) {
      $scope.currentProject = project;
    }
  });

  console.log($scope.currentProject)
  $scope.buildVariant = $scope.currentProject.BuildVariants[0];
  if (initialHash.buildVariant) {
    for (var i = 0; i < $scope.currentProject.BuildVariants.length; ++i) {
      if ($scope.currentProject.BuildVariants[i].Name === initialHash.buildVariant) {
        $scope.buildVariant = $scope.currentProject.BuildVariants[i];
        console.log("$$$ " + $scope.buildVariant.Name);
      }
    }
  }

  $scope.taskName = $scope.buildVariant.TaskNames[0];
  if (initialHash.taskName) {
    for (var i = 0; i < $scope.buildVariant.TaskNames.length; ++i) {
      if ($scope.buildVariant.TaskNames[i] === initialHash.taskName) {
        $scope.taskName = initialHash.taskName;
      }
    }
  }

  // Make sure we update bv and taskName on project change
  $scope.updateProject = function() {
    $scope.buildVariant = $scope.currentProject.BuildVariants[0];
    $scope.updateBuildVariant();
  };

  // Make sure we update taskName on buildVariant change
  $scope.updateBuildVariant = function() {
    $scope.taskName = $scope.buildVariant.TaskNames[0];
  };

  $scope.taskData = {};
  $scope.computedData = {};
  $scope.timeDiffOptions = [
    {
      name: "Start \u21E2 Finish",
      diff: ['finish_time', 'start_time']
    },
    {
      name: "Scheduled \u21E2 Start",
      diff: ['start_time', 'scheduled_time']
    }
  ];
  $scope.timeDiff = $scope.timeDiffOptions[0];
  $scope.numTasks = 50;
  $scope.numTasksOptions = [25, 50, 100, 200, 500, 1000, 2000];

  $scope.recompute = function() {
    $scope.computedData = {
      minTime: Number.POSITIVE_INFINITY,
      maxTime: Number.NEGATIVE_INFINITY
    };

    _.each($scope.taskData, function(task) {
      task.moment = {
        create_time: moment(task.create_time),
        dispatch_time: moment(task.dispatch_time),
        scheduled_time: moment(task.scheduled_time),
        start_time: moment(task.start_time),
        finish_time: moment(task.finish_time)
      };

      var diff = $scope.timeDiff.diff;
      task.runTime = task.moment[diff[0]].diff(task.moment[diff[1]]);

      if (task.runTime < $scope.computedData.minTime) {
        $scope.computedData.minTime = task.runTime;
      }
      if (task.runTime > $scope.computedData.maxTime) {
        $scope.computedData.maxTime = task.runTime;
      }
    });
  };

  $scope.load = function(before, limit) {
    var query = (!!before ? 'before=' + encodeURIComponent(before) : '');
    query += (query.length > 0 && !!limit ? '&' : '');
    query += (!!limit ? 'limit=' + encodeURIComponent(limit) : '');

    $http.get('/json/task_timing/' +
      encodeURIComponent($scope.currentProject.Name) + '/' +
      encodeURIComponent($scope.buildVariant.Name) + '/' +
      encodeURIComponent($scope.taskName) + '?' +
      query).
      success(function(data) {
        $scope.taskData = data.reverse();

        $scope.recompute();
      }).
      error(function(data) {
        alert("Error - " + JSON.stringify(data));
        $scope.taskData = [];
      });

    $locationHash.set({
      project: $scope.currentProject.Name,
      buildVariant: $scope.buildVariant.Name,
      taskName: $scope.taskName
    });
  };

  $scope.computeHeight = function(task, height) {
    return task.runTime / ($scope.computedData.maxTime * 1.1) * height;
  };

  $scope.computeWidth = function(width) {
    return width / $scope.taskData.length;
  };

  $scope.computeSampleTime = function(tickHeight, maxHeight) {
    return tickHeight / maxHeight * ($scope.computedData.maxTime * 1.1);
  };

  $scope.redirectToTask = function(task) {
    $window.open('/task/' + task.id, '_blank');
  };

  $scope.getTooltip = function(task) {
    return $filter('stringifyNanoseconds')(task.runTime * 1000000) + '\n' +
      'Started At: ' + task.moment.start_time.format('MM/D/YY HH:MMa');
  };

  $scope.load(null, $scope.numTasks);
}

function LiteVersionController($scope, $http) {
  $scope.version = {};
  $scope.loading = true;
  $scope.error = false;

  $scope.load = function(version) {
    $scope.loading = true;
    $http.get('/version_json/' + version).
      success(function(data) {
        $scope.version = data;
        $scope.loading = false;
      }).
      error(function() {
        $scope.error = true;
        $scope.loading = false;
      });
  };
}
