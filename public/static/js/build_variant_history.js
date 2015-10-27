mciModule.controller('BuildVariantHistoryController',['$scope', '$filter','$window', 'mciBuildVariantHistoryRestService', 'errorPasserService', function($scope, $filter, $window,
  mciBuildVariantHistoryRestService, errorPasser) {
  $scope.buildVariant = $window.buildVariant;
  $scope.project = $window.project;
  $scope.taskNames = $window.taskNames;
  $scope.versionsByRevision = {};
  $scope.versions = [];
  $scope.tasksByTaskNameByCommit = [];
  $scope.lastVersionId = '';

  var buildVersionsByRevisionMap = function(versions) {
    _.each(versions, function(version) {
      $scope.versions.push(version);
      $scope.versionsByRevision[version.revision] = version;
    });
  };

  var buildTasksByTaskNameCommitMap = function(tasksByCommit) {
    _.each(tasksByCommit, function(revisionToTasks) {
      var taskNameTaskMap = {};
      _.each(revisionToTasks.tasks, function(task) {
        taskNameTaskMap[task.display_name] = task;
      });

      $scope.tasksByTaskNameByCommit.push({
        _id: revisionToTasks._id,
        tasksByTaskName: taskNameTaskMap
      });
    });
  };

  /* Populate initial page data */
  buildVersionsByRevisionMap($window.versions);
  buildTasksByTaskNameCommitMap($window.tasksByCommit);

  var numVersions = $scope.versions.length;
  if (numVersions > 0) {
    $scope.lastVersionId = $scope.versions[numVersions - 1].id;
  }

  $scope.getGridClass = function(cell) {
    if (cell) {
      return $filter('statusFilter')(cell);
    }

    return "skipped";
  };

  $scope.loadMore = function() {
    mciBuildVariantHistoryRestService.getBuildVariantHistory(
      $scope.project,
      $scope.buildVariant, {
        format: 'json',
        before: $scope.lastVersionId,
      }, {
        success: function(data, status) {
          if (data.Versions) {
            buildVersionsByRevisionMap(data.Versions);
            $scope.lastVersionId = $scope.versions[$scope.versions.length - 1].id;
          }

          if (data.Tasks) {
            buildTasksByTaskNameCommitMap(data.Tasks);
          }
        },

        error: function(jqXHR, status, errorThrown) {
          errorPasser.pushError('Error getting build variant history: ' + jqXHR,'errorHeader');
        }
      }
    );
  };
}]);
