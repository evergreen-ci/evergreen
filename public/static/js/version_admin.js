mciModule.controller('AdminOptionsCtrl', ['$scope', '$rootScope', 'mciVersionsRestService','notificationService', function($scope, $rootScope, versionRestService, notifier) {

    $scope.adminOptionVals = {};
    $scope.modalTitle = 'Modify Version';

	function setVersionActive(active, abort) {
        versionRestService.takeActionOnVersion(
            $scope.version.Version.id,
            'set_active',
            { active: active, abort: abort },
            {
                success: function(data, status) {
                    $scope.closeAdminModal()
                    $rootScope.$broadcast("version_updated", data)
                    notifier.pushNotification(
                      "Version " + (active ? "scheduled." : "unscheduled.") + 
                      (abort ? "\n In progress tasks will be aborted." : ""),
                      'notifyHeader', 'success');
                },
                error: function(jqXHR, status, errorThrown) {
                    notifier.pushNotification('Error setting version activation: ' + jqXHR,'errorModal');
                }
            }
        );
	}
	function setVersionPriority(newPriority) {
        versionRestService.takeActionOnVersion(
            $scope.version.Version.id,
            'set_priority',
            { priority: newPriority },
            {
                success: function(data, status) {
                    $scope.closeAdminModal()
                    $rootScope.$broadcast("version_updated", data)
                    var msg = "Priority for version set to " + newPriority + "."
                    notifier.pushNotification(msg, 'notifyHeader', 'success');
                },
                error: function(jqXHR, status, errorThrown) {
                    notifier.pushNotification('Error changing priority: ' + jqXHR,'errorModal');
                }
            }
        );
	}

	$scope.updateScheduled = function(isActive) {
		var abortSet = $scope.adminOptionVals.abort ? true : false
		// only read in the abort checkbox if we are setting active to false
		var abortVersion = isActive ? false : abortSet;
		setVersionActive(isActive, abortVersion);
	}

	$scope.updatePriority = function() {
		var newPriority = parseInt($scope.adminOptionVals.priority);
		if(isNaN(newPriority)) {
			notifier.pushNotification('New priority value must be an integer','errorModal');
		} else {
			setVersionPriority(parseInt($scope.adminOptionVals.priority));
		}
	}

	$scope.closeAdminModal = function() {
		var modal = $('#admin-modal').modal('hide');
    }

	$scope.openAdminModal = function(opt) {
		$scope.adminOption = opt
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
                if ($scope.adminOption === 'unschedule') {
                    $scope.updateScheduled(false);
                    $('#admin-modal').modal('hide');
                } else if ($scope.adminOption === 'schedule') {
                    $scope.updateScheduled(true);
                    $('#admin-modal').modal('hide');
                }
            }
        });
	}

}]);

mciModule.directive('adminScheduleAll', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Schedule all tasks?' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="updateScheduled(true)">Yes</button>' +
      '</div>' +
    '</div>'
  }
});

mciModule.directive('adminUnscheduleAll', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        '<div>' +
          'Unschedule all tasks?' +
          '<div style="float:right">' +
            '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
            '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="updateScheduled(false)">Yes</button>' +
        '</div>' +
      '</div>' +
      '<div styl="float:right">' +
        '<input type="checkbox" id="passed" name="passed" ng-model="adminOptionVals.abort" class="ng-valid ng-dirty"> ' +
        '<label for="passed" style="font-weight:normal;font-size:.8em;">  Abort tasks that have already started</label>' +
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

