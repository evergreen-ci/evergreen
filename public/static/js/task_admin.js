mciModule.controller('AdminOptionsCtrl', ['$scope', 'mciTasksRestService', function($scope, taskRestService) {
    $scope.modalOpen = false;
    $scope.modalTitle = 'Task';
    $scope.adminOptionVals = {};

	$scope.setTask = function(task) {
        $scope.task = task;
        $scope.taskId = task.id;
        $scope.isAborted = ($scope.task.status == 'undispatched' && !$scope.task.activated && $scope.task.dispatch_time != 0)
        $scope.canRestart = ($scope.task.status == "success" || $scope.task.status == "failed" || $scope.isAborted);
        $scope.canAbort = ($scope.task.status == "dispatched" || $scope.task.status == "started");
        $scope.canSchedule = !$scope.task.activated && !$scope.canRestart && !$scope.isAborted;
        $scope.canUnschedule = $scope.task.activated && ($scope.task.status == "undispatched") ;
	};

	$scope.abort = function() {
        taskRestService.takeActionOnTask(
            $scope.taskId, 
            'abort', 
            {},
            {
                success: function(data, status) {
                    window.location.reload(true);
                },
                error: function(jqXHR, status, errorThrown) {
                    alert('Error aborting: ' + jqXHR);
                }
            }
        );
	};

    $scope.restart = function() {
        taskRestService.takeActionOnTask(
            $scope.taskId, 
            'restart', 
            {},
            {
                success: function(data, status) {
                    window.location.reload(true);
                },
                error: function(jqXHR, status, errorThrown) {
                    alert('Error restarting: ' + jqXHR);
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
                success: function(data, status) {
                    window.location.reload(true);
                },
                error: function(jqXHR, status, errorThrown) {
                    alert('Error setting priority: ' + jqXHR);
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
                success: function(data, status) {
                    window.location.reload(true);
                },
                error: function(jqXHR, status, errorThrown) {
                    alert('Error setting active = ' + active + ': ' + jqXHR);
                }
            }
        );
    }

	$scope.openAdminModal = function(opt) {
		$scope.adminOption = opt;
		var modal = $('#admin-modal').modal('show');

        if (opt === "setPriority") {
            modal.on('shown.bs.modal', function() {
                $('#input-priority').focus();
                $scope.modalOpen = true;
                $scope.$apply();
            });

            modal.on('hide.bs.modal', function() {
                $scope.modalOpen = false;
                $scope.$apply();
            });
        } else {
            modal.on('shown.bs.modal', function() {
                $scope.modalOpen = true;
                $scope.$apply();
            });

            modal.on('hide.bs.modal', function() {
                $scope.modalOpen = false;
                $scope.$apply();
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
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Abort current task?' +
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
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Unschedule current task?' +
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
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="restart()">Yes</button>' +
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
