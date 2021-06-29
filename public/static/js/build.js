mciModule.controller('BuildVariantHistoryController', function ($scope, $http, $filter, $timeout, $window) {
  $scope.userTz = $window.userTz;
  $scope.builds = [];
  $scope.buildId = "";
  $scope.buildResults = {};

  $scope.setBuildId = function (buildId) {
    $scope.buildId = buildId;
    if (!$scope.build.PatchInfo) {
      $scope.loadHistory();
    }
  };

  $scope.checkTaskHidden = function (task) {
    return !task.activated;
  };

  var computeBuildResults = function (buildData) {
    var tasks = buildData.Tasks;
    $scope.buildResults[buildData.Build._id] = [];

    for (var j = 0; j < tasks.length; ++j) {
      $scope.buildResults[buildData.Build._id].push({
        "class": $filter('statusFilter')(tasks[j].Task),
        "tooltip": tasks[j].Task.display_name + " - " + $filter('statusLabel')(tasks[j].Task),
        "link": '/task/' + tasks[j].Task.id
      });
    }
  };


  $scope.setBuilds = function (resp) {
    var data = resp.data;
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

  $scope.loadHistory = function () {
    $http.get('/json/build_history/' + $scope.buildId).then(
      function (resp) {
        $scope.setBuilds(resp);
      },
      function (resp) {
        console.log("Error getting build history: " + JSON.stringify(resp.data));
      });
  };
});


mciModule.controller('BuildViewController', function ($scope, $http, $timeout, $rootScope, mciTime, $window, $mdDialog, mciSubscriptionsService, notificationService, $mdToast) {
  var nsPerMs = 1000000
  $scope.build = {};
  $scope.computed = {};
  $scope.loading = false;
  $scope.lastUpdate = null;
  $scope.jiraHost = $window.jiraHost;
  $scope.hide_add_subscription = true;
  $scope.triggers = [{
      trigger: "outcome",
      resource_type: "BUILD",
      label: "this build finishes",
    },
    {
      trigger: "failure",
      resource_type: "BUILD",
      label: "this build fails",
    },
    {
      trigger: "success",
      resource_type: "BUILD",
      label: "this build succeeds",
    },
    {
      trigger: "exceeds-duration",
      resource_type: "BUILD",
      label: "the runtime for this build exceeds some duration",
      extraFields: [{
        text: "Build duration (seconds)",
        key: "build-duration-secs",
        validator: validateDuration
      }],
    },
    {
      trigger: "runtime-change",
      resource_type: "BUILD",
      label: "the runtime for this build changes by some percentage",
      extraFields: [{
        text: "Percent change",
        key: "build-percent-change",
        validator: validatePercentage
      }],
    },
    {
      trigger: "outcome",
      resource_type: "TASK",
      label: "a task in this build finishes",
      regex_selectors: taskRegexSelectors()
    },
    {
      trigger: "failure",
      resource_type: "TASK",
      label: "a task in this build fails",
      regex_selectors: taskRegexSelectors()
    },
    {
      trigger: "succeeds",
      resource_type: "TASK",
      label: "a task in this build succeeds",
      regex_selectors: taskRegexSelectors()
    },
  ];

  var dateSorter = function (a, b) {
    return (+a) - (+b)
  }

  $scope.addSubscription = function () {
    omitMethods = {};
    omitMethods[SUBSCRIPTION_JIRA_ISSUE] = true;
    omitMethods[SUBSCRIPTION_EVERGREEN_WEBHOOK] = true;
    promise = addSubscriber($mdDialog, $scope.triggers, omitMethods);

    $mdDialog.show(promise).then(function (data) {
      if (data.resource_type === "BUILD") {
        addSelectorsAndOwnerType(data, "build", $scope.build.Build._id);

      } else {
        // this block assumes that the resource_types of all subscriptions getting here are "TASK"
        addInSelectorsAndOwnerType(data, "task", "build", $scope.build.Build._id);
      }
      $scope.saveSubscription(data);
    });
  };

  $scope.saveSubscription = function (subscription) {
    var success = function () {
      $mdToast.show({
        templateUrl: "/static/partials/subscription_confirmation_toast.html",
        position: "bottom right"
      });
    };
    var failure = function (resp) {
      notificationService.pushNotification('Error saving subscriptions: ' + resp.data.error, 'notifyHeader');
    };
    mciSubscriptionsService.post([subscription], {
      success: success,
      error: failure
    });
  }

  $scope.setBuild = function (build) {
    $scope.build = build;
    $scope.commit = {
      message: $scope.build.Version.message,
      author: $scope.build.Version.author,
      author_email: $scope.build.Version.author_email,
      create_time: $scope.build.Version.create_time,
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
      } else {
        // use the start/end time rather than time taken so that display tasks display the wall clock time
        build.Tasks[i].Task.time_taken = 1000 * 1000 * (Date.parse(build.Tasks[i].Task.finish_time) - Date.parse(build.Tasks[i].Task.start_time));
      }
      if (build.Tasks[i].Task.time_taken > $scope.computed.maxTaskTime) {
        $scope.computed.maxTaskTime = build.Tasks[i].Task.time_taken;
      }
    }

    $scope.lastUpdate = mciTime.now();

    $scope.makeSpanMS = build.makespan / nsPerMs;
    $scope.totalTimeMS = build.time_taken / nsPerMs;
  };

  $rootScope.$on("build_updated", function (e, newBuild) {
    newBuild.PatchInfo = $scope.build.PatchInfo
    $scope.setBuild(newBuild);
  });


  $scope.setBuild($window.build);

  $scope.plugins = $window.plugins

});