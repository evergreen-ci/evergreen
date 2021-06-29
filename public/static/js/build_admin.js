mciModule.controller('AdminOptionsCtrl', [
  '$scope', '$rootScope', 'mciBuildsRestService', 'notificationService','$filter', 'RestartUtil',
  function($scope, $rootScope, buildRestService, notifier, $filter, RestartUtil) {
    $scope.selection = "completed";
    $scope.setBuild = function(uiBuild) {
      $scope.build = uiBuild.Build;
      $scope.setRestartSelection(RestartUtil.STATUS.ALL)
      $scope.buildId = $scope.build._id;
      $scope.buildTasks = uiBuild.Tasks
    };

    $scope.numToBeRestarted = function(){
        return $scope.buildTasks.filter(function(x){return x.checkedForRestart}).length;
    }

    $scope.adminOptionVals = {};
    $scope.modalOpen = false;
    $scope.modalTitle = 'Modify Build';
    $scope.statuses = RestartUtil.STATUS

    $scope.setRestartSelection = function(status){
        $scope.selection = status;
        if(status !== "") {
            _.each($scope.buildTasks, function(task) {
                task.checkedForRestart = status.matches(task.Task)
            })
        }
    }

    $scope.abort = function() {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'abort',
            {},
            {
                success: function(resp) {
                    var data = resp.data;
                    var message = "Build aborted" +
                        ($scope.build.r === "merge_test" ? " and version removed from commit queue." : ".");
                    $scope.closeAdminModal();
                    $rootScope.$broadcast("build_updated", data);
                    notifier.pushNotification(message, 'notifyHeader', 'success');
                },
                error: function(resp) {
                    notifier.pushNotification('Error aborting build: ' + resp.data,'errorModal');
                }
            }
        );
    };

    $scope.restart = function() {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'restart',
            { abort: $scope.adminOptionVals.abort,
              taskIds: $scope.buildTasks.filter(function(y){return y.checkedForRestart}).map(task => task.Task.id)
            },
            {
                success: function(resp) {
                    var data = resp.data;
                    $scope.closeAdminModal();
                    $rootScope.$broadcast("build_updated", data);
                    notifier.pushNotification("Build scheduled to restart.", 'notifyHeader', 'success');
                },
                error: function(resp) {
                    notifier.pushNotification('Error restarting build: ' + resp.data,'errorModal');
                }
            }
        );
    };

    $scope.updatePriority = function() {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'set_priority',
            { priority: $scope.adminOptionVals.priority },
            {
                success: function(resp) {
                    var data = resp.data;
                    $scope.closeAdminModal();
                    $rootScope.$broadcast("build_updated", data);
                    notifier.pushNotification("Priority for build updated to "+
                    $scope.adminOptionVals.priority + ".", 'notifyHeader', 'success');
                },
                error: function(resp) {
                    notifier.pushNotification('Error setting build priority: ' + resp.data,'errorModal');
                }
            }
        );
    };

    $scope.setActive = function(active) {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'set_active',
            { active: active,
              abort: $scope.adminOptionVals.abort },
            {
                success: function(resp) {
                    var data = resp.data;
                    $scope.closeAdminModal();
                    $rootScope.$broadcast("build_updated", data);
                    var notifyString = "Build marked as " + (active ? "scheduled" : "unscheduled") +
                        (!active && $scope.build.r === "merge_test" ? " and version removed from commit queue." : ".") +
                        (abort ? "\n In progress tasks will be aborted." : "");
                    notifier.pushNotification(notifyString, 'notifyHeader', 'success');
                },
                error: function(resp) {
                    notifier.pushNotification('Error scheduling build: ' + resp.data,'errorModal');
                }
            }
        );
    };

	$scope.closeAdminModal = function() {
		var modal = $('#admin-modal').modal('hide');
    }

    $scope.openAdminModal = function(opt) {
        $scope.adminOption = opt;
        $scope.modalOpen = true;
        var modal = $('#admin-modal').modal('show');

        if (opt === "priority") {
            modal.on('shown.bs.modal', function() {
                $('#input-priority').focus();
                $scope.modalOpen = true;
            });

            modal.on('hide.bs.modal', function() {
                $scope.modalOpen = false;
            });
        } else {
            modal.on('shown.bs.modal', function() {
                $scope.modalOpen = true;
            });

            modal.on('hide.bs.modal', function() {
                $scope.modalOpen = false;
            });
        }

        $(document).keyup(function(ev) {
            if ($scope.modalOpen && ev.keyCode === 13) {
                if ($scope.adminOption === 'abort') {
                    $scope.abort();
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'unschedule') {
                    $scope.setActive(false);
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'schedule') {
                    $scope.setActive(true);
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'restart') {
                    $scope.restart();
                    $('#admin-modal').modal('hide');
                }
            }
        });
    };
}]);

mciModule.directive('adminAbortBuild', function() {
    let merge_test = "merge_test"
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Abort all tasks for current build?' +
        '<div ng-show="build.r === merge_test">' +
            'This will remove the version from the commit queue.' +
        '</div>' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="abort()">Yes</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminScheduleBuild', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Schedule current build?' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setActive(true)">Yes</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminUnscheduleBuild', function() {
    let merge_test = "merge_test"
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Unschedule current build?' +
        '<div ng-show="build.r === merge_test">' +
            'This will remove the version from the commit queue.' +
        '</div>' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setActive(false)">Yes</button>' +
        '<div style="margin-top: 6px;">' +
          '<input type="checkbox" id="abort" name="passed" ng-model="adminOptionVals.abort" class="ng-valid ng-dirty">' +
          '<label for="abort" style="font-weight:normal;">Abort tasks that have already started</label>' +
        '</div>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminSetPriority', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Set new priority = ' +
        '<form style="display: inline" ng-submit="updatePriority()">' +
          '<input type="text" id="input-priority" placeholder="number" ng-model="adminOptionVals.priority">' +
        '</form>' +
        '<button type="submit" class="btn btn-primary" style="float: right; margin-left: 10px;" ng-click="updatePriority()">Set</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminRestartBuild', function() {
    return {
        restrict: 'E',
        templateUrl: "/static/partials/admin-restart-build.html"
  }
});
