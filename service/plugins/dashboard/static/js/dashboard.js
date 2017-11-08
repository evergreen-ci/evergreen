mciModule.controller('DashboardController', function PerfController($scope, $window, $http, $location){

  // list of all statuses that a project can be
  $scope.status_list = ["pass", "forced accept", "undesired", "unacceptable", "no info"];

  // if this is an app level plugin retrieve app level information
  if ($window.appData){
    $scope.branches = $window.appData.branches;
    $scope.defaultBranch = $window.appData.default_branch;
    $scope.defaultBaselines = $window.appData.default_baselines;
    $scope.hidePassingTasks = true;
    $scope.showUnfinishedTasks = true;
  }

  if ($window.plugins){
    $scope.conf = $window.plugins["dashboard"];
  }

  // check the location hash for a branch and use that if it's there.
  if ($location.search().branch){
    $scope.defaultBranch = $location.search().branch;
  }


  if ($scope.branches){
    $scope.branchNames = _.keys($scope.branches);
    $scope.currentBranch = $scope.branchNames[0];
    // if there is a branch in the location or a master branch make those the default.
    if (_.contains($scope.branchNames, $scope.defaultBranch)){
      $scope.currentBranch = $scope.defaultBranch;
    } else if (_.contains($scope.branchNames, "master")){
      $scope.currentBranch = "master";
    }
    $scope.dashboardProjects = $scope.branches[$scope.currentBranch];
    $scope.projects = {};
    _.each($scope.dashboardProjects, function(projectName){
      $scope.projects[projectName] = {};
    })

    // check the location hash for project baselines
    _.each($scope.branches, function(projects, branchName){
      _.each(projects, function(projectName){
        if ($location.search()[projectName]){
          $scope.defaultBaselines[projectName] = $location.search()[projectName];
        }
      })
    })
  }



  // if there is a version, this is a version level dashboard.
  if ($window.version) {
    $scope.version = $window.version.Version;
    $scope.project = $scope.version.id;
    $scope.hidePassingTasks = false;
    $scope.showUnfinishedTasks = false;
  }


  // resetData will reset the projects.
  // The projects object is a map from projects to the data associated with that project,
  // which includes the following:
  // counts - the number of each type of status of the tasks
  // baselines - a list of all the baselines that are available to toggle between
  var resetProjectData = function(project){
    $scope.projects[project] = {};
  }

  $scope.getProjectData = function(project){
    if ($scope.projects[project]){
      return $scope.projects[project];
    }
    return {};
  }

    // gets the color class associated with the state
    $scope.getColor = function(state){
      if (state){
        return "dashboard-" + state.toLowerCase().split(" ").join("-")
      }
    };

    $scope.getWidth = function(state, project){
      var projectData = $scope.getProjectData(project);
      if (projectData.counts){
        return (projectData.counts[state.toLowerCase()] / projectData.total) * 100 + "%";
      }
    }

    $scope.getOrder = function(test){
      return _.indexOf($scope.status_list, test.state);
    };

    $scope.filterPassed = function(test){
      if ($scope.hidePassingTasks){
        return test.state != 'pass';
      }
      return true;
    }
    $scope.getNumberPassing = function(project, task){
      return _.filter($scope.getBaselineData(task, project), function(t){
        return t.state == 'pass';
      }).length;
    }
    $scope.allPassing = function(project, task){
      return $scope.hidePassingTasks && $scope.getNumberPassing(project, task) == $scope.getBaselineData(task,project).length;
    }


    $scope.getBaselines = function(baseline, project){
      return $scope.getProjectData(project).baselines;
    }

    $scope.formatBaseline = function(baseline) {
      if (baseline){
        return baseline.split('-')[0];
      }
      return baseline;
    }

    $scope.showProgress = function(project) {
      return !_.isEmpty($scope.getProjectData(project).counts);
    };

    $scope.getData = function(project){
      return $scope.getProjectData(project).dashboardData;
    }

    $scope.getColWidth = function(){
      return "col-lg-" + (12/$scope.dashboardProjects.length);

    }

    $scope.getCommitInfo = function(project) {
      return $scope.getProjectData(project).commitInfo;
    }

    $scope.setBranch = function(branchName) {
      $scope.dashboardProjects = $scope.branches[branchName];
      $scope.currentBranch = branchName;
      $location.search("branch", branchName);
    }

    $scope.getTicketName = function(ticket) {
      return (ticket.name ? ticket.name : ticket);
    }
    $scope.getJiraLink = function(ticket) {
      return "https://jira.mongodb.com/browse/" + $scope.getTicketName(ticket);
    }

    $scope.getStrikethroughClass = function(ticket){
      var completedStatuses = ["closed", "resolved"];

      var currentStatus = ticket.status ? ticket.status.toLowerCase() : "";
      return _.contains(completedStatuses, currentStatus) ? "strikethrough" : "";
    }

    $scope.setBaseline = function(baseline, project){
      $scope.projects[project].currentBaseline = baseline;
      $location.search(project, baseline);
      getTestStatuses(project);
    };

    $scope.getBaselineData = function(task, project){
      baseline =  _.find(task.data.baselines, function(b){
        return $scope.projects[project].currentBaseline == b.version;
      })
      return baseline.data
    };

    $scope.changeRevision = function(increment, project){
      var projectedSkip = $scope.projects[project].skip + increment;
      if (projectedSkip < 0){
        return;
      }
      if (increment > 0 && $scope.projects[project].lastRevision){
        return;
      }
      if ($scope.projects[project].skip + increment < 0) {
        return;
      }
      $scope.projects[project].skip += increment;
      getDashboardForProject(project);
      return;
    }

    $scope.sortTables = function(project){
      return function(task){
        return $scope.allPassing(project, task);
      }
    }

    var getTestStatuses = function(project) {
      var status = {}
        // for each task in the data, get the metrics of the selected baseline.
        _.each($scope.projects[project].dashboardData, function(task){
          var counts = _.countBy($scope.getBaselineData(task, project), function(t){return t.state.toLowerCase();});
          for (key in counts) {
            if (status[key]){
              status[key] += counts[key];
            } else {
              status[key] = counts[key];
            }
          }
        });
        var total = getTotal(status);

        $scope.projects[project].total = getTotal(status);
        $scope.projects[project].counts = status;
      };

    var getTotal = function(statuses) {
      var sum = 0;
      for (key in statuses) {
        sum += statuses[key];
      }
      return sum;
    };

    var setInitialBaselines = function(d, project){
      if (d.length > 0) {
        data = d[0].data;
      } else {
        console.log("no baselines");
        return
      }
      if (data && data.baselines){
        $scope.projects[project].baselines = _.pluck(data.baselines, 'version')
        if (_.contains($scope.projects[project].baselines, $scope.projects[project].currentBaseline)){
          $scope.setBaseline($scope.projects[project].currentBaseline, project);
          return
        }
        if ($scope.defaultBaselines && $scope.defaultBaselines[project]){
          var defBaseline = $scope.defaultBaselines[project]
          if (_.contains($scope.projects[project].baselines, defBaseline)){
            $scope.setBaseline(defBaseline, project);
            return
          }
        }
        $scope.setBaseline($scope.projects[project].baselines[0], project);
      }
    };

    var getUnfinishedTasks = function(project){
      if ($scope.projects[project].unfinishedTasks) {
        return
      }
      var versionId = "";
      var projectId = project;
      if ($scope.version){
        projectId = $scope.version.Project;
        versionId = $scope.version.Id;
      } else {
        versionId = $scope.projects[project].commitInfo.version_id;
      }
      if (!$scope.projects[project].unfinishedTasks) {
        $http.get("/plugin/dashboard/tasks/project/" + project + "/version/" + versionId).then(
        function(resp){
          var d = resp.data;
          if (d != null){
            var dashData = $scope.projects[project].dashboardData;
            var allTasks = d;
            var existingTasks = _.reduce(dashData, function(existingTasks, data){
              if (!existingTasks[data.task_name]) {
                existingTasks[data.task_name] = [];
              }
              existingTasks[data.task_name].push(data.variant);
              return existingTasks;
            }, {})
             // compute the intersection of all tasks and ones that have finished
            $scope.projects[project].unfinishedTasks = _.mapObject(allTasks, function(variantList, taskName){
              if (existingTasks[taskName]){
                return _.difference(variantList, existingTasks[taskName]);
              }
              return variantList;
            })
        }
      });
    }
    }



  // gets the dashbord data and populates the baseline list.
  // In this case, the dashboardData's key is the version id.
  var getVersionDashboard = function() {
    $scope.projects = {};
    $http.get("/plugin/json/version/" + $scope.version.id + "/dashboard/").then(
    function(resp){
      var d = resp.data;
      if (d != null){
        $scope.projects[$scope.version.id] = {};
        $scope.projects[$scope.version.id].dashboardData = d;
              // take the first task's data and get the set of baselines from it
              // NOTE: this is assuming that tasks all have the same baselines.
              setInitialBaselines(d, $scope.version.id);
              getUnfinishedTasks($scope.version.id);
            }
      });
  };

  // gets the dashboard for the app level by performing a get for
  // each project id and populating the dashboard data
  var getAppDashboard = function(){
      // get the latest version data for the whole dashboard
      _.each($scope.branches, function(projects, branchName){
        _.each(projects, function (projectName) {
          getDashboardForProject(projectName);
        })
      })
    }

    var getDashboardForProject = function(projectName){
      var skip = 0;
      if ($scope.getProjectData(projectName).skip){
        skip = $scope.getProjectData(projectName).skip;
      }
      var currentBaseline = $scope.getProjectData(projectName).currentBaseline;
      resetProjectData(projectName);
      $scope.projects[projectName].skip = skip;
      $scope.projects[projectName].currentBaseline = currentBaseline;
      $http.get("/plugin/json/version/latest/" + projectName + "/dashboard?skip="+ skip).then(
      function(resp){
        var d = resp.data;
        $scope.projects[projectName].dashboardData = d.json_tasks;
        $scope.projects[projectName].commitInfo= d.commit_info;
        $scope.projects[projectName].endOfVersion = d.last_revision;
        $scope.projects[projectName].counts = {};
        $scope.projects[projectName].unfinishedTasks = {};
        getUnfinishedTasks(projectName);
        setInitialBaselines(d.json_tasks, projectName);
      })

    }

    var getInitialData = function(){
      if ($scope.dashboardProjects){
        getAppDashboard();
      } else {
        getVersionDashboard();
      }
    }
    getInitialData();


    })
mciModule.directive('dashboardTable', function(){
  return {
    restrict: 'E',
    templateUrl:'/plugin/dashboard/static/partials/dashboard_table.html',
  }
})

mciModule.directive('baselineDropdown', function(){
  return {
    restrict: 'E',
    templateUrl:'/plugin/dashboard/static/partials/baselines.html',
  }
})

mciModule.directive('pageButtons', function(){
  return {
    restrict: 'E',
    templateUrl:'/plugin/dashboard/static/partials/page_buttons.html',
  }
})

mciModule.directive('unfinishedTasks', function(){
  return {
    restrict: 'E',
    templateUrl:'/plugin/dashboard/static/partials/unfinished_tasks.html',
  }
})
