mciModule.controller('AdminOptionsCtrl', ['$scope', '$window', '$rootScope', 'mciTasksRestService', 'notificationService', function($scope, $window, $rootScope, taskRestService, notifier) {
    $scope.modalOpen = false;
    $scope.modalTitle = 'Modify Task';
    $scope.adminOptionVals = {};

	$scope.setTask = function(task) {
        var dispatchTime = +new Date(task.dispatch_time);
        $scope.task = task;
        $scope.taskId = task.id;
        $scope.isAborted = ($scope.task.status == 'undispatched' && !$scope.task.activated && $scope.task.dispatch_time > 0)
        $scope.canRestart = (
          (
            $scope.task.status !== "started" &&
            $scope.task.status !== "unstarted" &&
            $scope.task.status !== "undispatched" &&
            $scope.task.status !== "dispatched" &&
            $scope.task.status !== "inactive"
          ) ||
          $scope.isAborted || (
            $scope.task.display_only &&
            $scope.task.task_waiting=="blocked"
          )
        )
        $scope.canAbort = ($scope.task.status == "dispatched" || $scope.task.status == "started");
        $scope.canSchedule = !$scope.task.activated && !$scope.canRestart && !$scope.isAborted;
        $scope.canUnschedule = $scope.task.activated && ($scope.task.status == "undispatched") ;
        $scope.canSetPriority = ($scope.task.status == "undispatched");
        $scope.canOverrideDependencies = ($scope.task.r == "patch_request" || $scope.task.r == "github_pull_request" || $window.permissions.project_tasks>=30);
	};

    function doModalSuccess(message, data, reload){
        $scope.closeAdminModal();
        if (reload) {
          $window.location.reload();
        } else {
          $rootScope.$broadcast("task_updated", data);
          $scope.setTask(data);
          notifier.pushNotification(message, 'notifyHeader', 'success');
        }
    }

    function doModalFailure(message){
        $scope.closeAdminModal();
        notifier.pushNotification(message, 'notifyHeader', 'error');
    }

	$scope.abort = function() {
        taskRestService.takeActionOnTask(
            $scope.taskId,
            'abort',
            {},
            {
                success: function(resp) {
                    var message = "Task aborted" +
                        ($scope.task.r === "merge_test" ? " and version removed from commit queue." : ".")
                    doModalSuccess(message, resp.data, $scope.task.display_only);
                },
                error: function(resp) {
                    notifier.pushNotification('Error aborting: ' + resp.data.error,'errorModal');
                }
            }
        );
	};

    $scope.restart = function() {
        $scope.noClose = true;
        taskRestService.takeActionOnTask(
            $scope.taskId,
            'restart',
            {},
            {
                success: function(resp) {
                    doModalSuccess("Task scheduled to restart.", resp.data, $scope.task.display_only);
                },
                error: function(resp) {
                    notifier.pushNotification('Error restarting: ' + resp.data,'errorModal');
                    doModalFailure("Error restarting: " + resp.data);
                }
            }
        );
    };

    $scope.setPriority = function() {
        taskRestService.takeActionOnTask(
            $scope.taskId,
            'set_priority',
            { priority: $scope.adminOptionVals.priority },
            {
                success: function(resp) {
                    var data = resp.data;
                    doModalSuccess("Priority for task updated to "+ data.priority +".", data, $scope.task.display_only);
                },
                error: function(resp) {
                    notifier.pushNotification('Error setting priority: ' + resp.data,'errorModal');
                }
            }
        );
    };

    $scope.setActive = function(active) {
        taskRestService.takeActionOnTask(
            $scope.taskId,
            'set_active',
            { active: active },
            {
                success: function(resp) {
                    var data = resp.data;
                    var message = "Task marked as " + (active ? "scheduled" : "unscheduled") +
                        (!active && $scope.task.r  === "merge_test" ? " and version removed from commit queue." : ".");
                    doModalSuccess(message, data, $scope.task.display_only);
                },
                error: function(resp) {
                    notifier.pushNotification('Error setting active = ' + active + ': ' + resp.data,'errorModal');
                }
            }
        );
    }

	$scope.closeAdminModal = function() {
		var modal = $('#admin-modal').modal('hide');
    }

	$scope.openAdminModal = function(opt) {
		$scope.adminOption = opt;
		var modal = $('#admin-modal').modal( {"show": true, "backdrop": "static"});

        if (opt === "setPriority") {
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
                } else if ($scope.adminOption === 'restart') {
                    $scope.restart();
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'unschedule') {
                    $scope.setActive(false);
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'schedule') {
                    $scope.setActive(true);
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'priority') {
                    $scope.setPriority();
                    $('#admin-modal').modal('hide');
                }
            }
        });
	};

}]);

mciModule.directive('adminAbortTask', function() {
    let merge_test = "merge_test"
    return {

        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Abort current task?' +
        '<div ng-show="task.r === merge_test">' +
            'This will remove version from the commit queue.' +
        '</div>' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="abort()">Yes</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminScheduleTask', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Schedule current task?' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setActive(true)">Yes</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminUnscheduleTask', function() {
    let merge_test = "merge_test"
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Unschedule current task?' +
        '<div ng-show="task.r === merge_test">' +
            'This will remove version from the commit queue.' +
        '</div>' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setActive(false)">Yes</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminRestartTask', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Restart current task?' +
        '<button type="button" class="btn btn-danger" style="float: right;" ng-disabled="noClose" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="restart()" ng-disabled="noClose">' +
          '<md-progress-circular ng-if="noClose" class="md-warn md-accent md-hue-2" md-mode="indeterminate" md-diameter="20px"></md-progress-circular>' +
          '<span ng-if="!noClose">Yes</span>' +
        '</button>' +
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
        '<form style="display: inline" ng-submit="setPriority()">' +
          '<input type="text" id="input-priority" placeholder="number" ng-model="adminOptionVals.priority">' +
        '</form>' +
        '<button type="submit" class="btn btn-primary" style="float: right; margin-left: 10px;" ng-click="setPriority()">Set</button>' +
      '</div>' +
    '</div>'
  }
});
