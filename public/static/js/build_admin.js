mciModule.controller('AdminOptionsCtrl', ['$scope', 'mciBuildsRestService', 'errorPasserService', function($scope, buildRestService, errorPasser) {
    $scope.setBuild = function(build) {
        $scope.build = build;
        $scope.buildId = build._id;
    };

    $scope.adminOptionVals = {};
    $scope.modalOpen = false;
    $scope.modalTitle = 'Modify Build';

    $scope.abort = function() {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'abort',
            {},
            {
                success: function(data, status) {
                    window.location.reload();
                },
                error: function(jqXHR, status, errorThrown) {
                    errorPasser.pushError('Error aborting build: ' + jqXHR,'errorModal');
                }
            }
        );
    };

    $scope.restart = function() {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'restart',
            { abort: $scope.adminOptionVals.abort },
            {
                success: function(data, status) {
                    window.location.reload();
                },
                error: function(jqXHR, status, errorThrown) {
                    errorPasser.pushError('Error restarting build: ' + jqXHR,'errorModal');
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
                success: function(data, status) {
                    window.location.reload();
                },
                error: function(jqXHR, status, errorThrown) {
                    errorPasser.pushError('Error setting build priority: ' + jqXHR,'errorModal');
                }
            }
        );
    };

    $scope.setActive = function(active) {
        buildRestService.takeActionOnBuild(
            $scope.buildId,
            'set_active',
            { active: active },
            {
                success: function(data, status) {
                    window.location.reload();
                },
                error: function(jqXHR, status, errorThrown) {
                    errorPasser.pushError('Error aborting build: ' + jqXHR,'errorModal');
                }
            }
        );
    };

    $scope.openAdminModal = function(opt) {
        $scope.adminOption = opt;
        $scope.modalOpen = true;
        var modal = $('#admin-modal').modal('show');

        if (opt === "priority") {
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
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Abort all tasks for current build?' +
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
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Unschedule current build?' +
        '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setActive(false)">Yes</button>' +
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
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        '<div>' +
          'Restart all tasks?' +
          '<div style="float:right">' +
            '<button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
            '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="restart()">Yes</button>' +
        '</div>' +
      '</div>' +
      '<div styl="float:right">' +
        '<input type="checkbox" id="passed" name="passed" ng-model="adminOptionVals.abort" class="ng-valid ng-dirty"> ' +
        '<label for="passed" style="font-weight:normal;font-size:.8em;">  Abort in-progress tasks</label>' +
      '</div>' +
    '</div>'
  }
});
