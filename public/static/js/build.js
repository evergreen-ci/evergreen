function BuildVariantHistoryController($scope, $http, $filter, $timeout, $window) {
  $scope.userTz = $window.userTz;
  $scope.builds = [];
  $scope.buildId = "";
  $scope.buildResults = {};

  $scope.setBuildId = function(buildId) {
    $scope.buildId = buildId;
    if (!$scope.build.PatchInfo) {
      $scope.loadHistory();
    }
  };

  $scope.checkTaskHidden = function(task) {
    return !task.activated;
  };

  var computeBuildResults = function(buildData) {
    var build = buildData.Build;
    $scope.buildResults[build._id] = [];

    for (var j = 0; j < build.tasks.length; ++j) {
      if (!build.tasks[j].activated) {
        build.tasks[j].status = 'unscheduled';
      }

      $scope.buildResults[build._id].push({
        "class": $filter('statusFilter')(build.tasks[j]),
        "tooltip": build.tasks[j].display_name + " - " + $filter('statusLabel')(build.tasks[j]),
        "link": '/task/' + build.tasks[j].id
      });
    }
  };

  $scope.setBuilds = function(data) {
    var builds = data.builds;
    $scope.buildResults = {};
    if (data.lastSuccess) {
      $scope.lastSuccess = data.lastSuccess;
      $scope.showLastSuccess = true;
      computeBuildResults($scope.lastSuccess);
    } else {
      $scope.lastSuccess = null;
      $scope.showLastSuccess = false;
    }

    $scope.builds = builds;
    for (var i = 0; i < builds.length; ++i) {
      if ($scope.showLastSuccess && builds[i].Build._id == $scope.lastSuccess.Build._id) {
        $scope.showLastSuccess = false;
      }
      computeBuildResults(builds[i]);
    }
    $scope.lastUpdate = new Date();
  }

  $scope.loadHistory = function() {
    $http.get('/json/build_history/' + $scope.buildId)
      .success(function(data) {
        $scope.setBuilds(data);
      })
      .error(function(data) {
        console.log("Error getting build history: " + data);
      });
  };
}

function BuildViewController($scope, $http, $timeout, mciTime, $window) {
  $scope.build = {};
  $scope.computed = {};
  $scope.loading = false;
  $scope.lastUpdate = null;

  $scope.setBuild = function(build) {
    $scope.build = build;
    $scope.commit = {
      message: $scope.build.Version.message,
      author: $scope.build.Version.author,
      author_email: $scope.build.Version.author_email,
      push_time: $scope.build.Version.create_time,
      gitspec: $scope.build.Build.gitspec,
      repo_owner: $scope.build.repo_owner,
      repo_name: $scope.build.repo_name
    };

    $scope.computed = {};

    build.Build.activated_time = new Date(build.Build.activated_time);

    build.Build.start_time = mciTime.fromMilliseconds(build.Build.start_time);
    build.Build.finish_time = mciTime.fromMilliseconds(build.Build.finish_time);
    build.CurrentTime = mciTime.fromNanoseconds(build.CurrentTime);

    build.Build.time_taken = mciTime.finishConditional(build.Build.start_time, build.Build.finish_time, build.CurrentTime) * 1000 * 1000;

    if ($scope.build.PatchInfo) {
      $scope.showBaseCommitLink = $scope.build.PatchInfo.BaseBuildId !== '';

      if ($scope.build.PatchInfo.StatusDiffs) {
        // setup diff data to use statusFilter
        for (var i = 0; i < $scope.build.PatchInfo.StatusDiffs.length; ++i) {

          var original = $scope.build.PatchInfo.StatusDiffs[i].diff.original;

          // in case the base task has not yet run
          if (_.size(original) !== 0) {
            $scope.build.PatchInfo.StatusDiffs[i].diff.original = {
              'task_end_details': original,
              'status': original.status,
            };
          }

          var patch = $scope.build.PatchInfo.StatusDiffs[i].diff.patch;

          // in case the patch task has not yet run
          if (_.size(patch) !== 0) {
            $scope.build.PatchInfo.StatusDiffs[i].diff.patch = {
              'task_end_details': patch,
              'status': patch.status,
            };
          }
        }
      }
    }

    // Initialize to 1 so we avoid divide-by-zero errors
    $scope.computed.maxTaskTime = 1;
    for (var i = 0; i < build.Tasks.length; ++i) {
      if (build.Tasks[i].Task.status === 'started' || build.Tasks[i].Task.status === 'dispatched') {
        var d = new Date(build.Tasks[i].Task.start_time).getTime();
        if (build.CurrentTime && d) {
          build.Tasks[i].Task.time_taken = (build.CurrentTime - d) * 1000 * 1000;
        }
      }
      if (build.Tasks[i].Task.time_taken > $scope.computed.maxTaskTime) {
        $scope.computed.maxTaskTime = build.Tasks[i].Task.time_taken;
      }
    }

    $scope.lastUpdate = mciTime.now();
  };

  $scope.setBuild($window.build);
  $scope.plugins = $window.plugins

}
