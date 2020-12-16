mciModule.controller('AdminOptionsCtrl', ['$scope', 'mciHostRestService', 'notificationService', function($scope, hostRestService, notifier) {
  $scope.setHostId = function(host) {
    $scope.host = host;
  };

  $scope.filterCurrentHostStatus = function(status) {
    return status != $scope.host.status;
  }

  $scope.validHostStatuses = ["running", "decommissioned", "quarantined", "terminated"].filter($scope.filterCurrentHostStatus);
  $scope.newStatus = $scope.validHostStatuses[0];
  $scope.modalTitle = 'Modify Host';
  $scope.notes={};

  $scope.canReprovision = function() {
    return host.status === 'running' && (host.distro.bootstrap_settings.method === 'ssh' || host.distro.bootstrap_settings.method === 'user-data');
  }

  $scope.updateStatus = function() {
    hostRestService.updateStatus(
      $scope.host.id,
      'updateStatus',
      { status: $scope.newStatus, notes: $scope.notes.text },
      {
        success: function(resp) {
          window.location.reload();
        },
        error: function(resp) {
          notifier.pushNotification('Error updating host status: ' + resp.data, 'errorModal');
        }
      }
    );
  };

  $scope.setRestartJasper = function() {
    hostRestService.setRestartJasper(
      $scope.host.id,
      'restartJasper',
      {},
      {
        success: function(resp) {
          window.location.reload();
        },
        error: function(resp) {
          notifier.pushNotification('Error marking host as needing Jasper restarted: ' + resp.data, 'errorModal');
        }
      }
    );
  };

  $scope.setReprovisionToNew = function() {
    hostRestService.setReprovisionToNew(
      $scope.host.id,
      'reprovisionToNew',
      {},
      {
        success: function(resp) {
          window.location.reload();
        },
        error: function(resp) {
          notifier.pushNotification('Error marking host as needing to reprovision: ' + resp.data, 'errorModal');
        }
      }
    );
  };

  $scope.setHostStatus = function(status) {
    $scope.newStatus = status;
  };

  $scope.openAdminModal = function(opt) {
    $scope.adminOption = opt;
    $scope.modalOpen = true;
    var modal = $('#admin-modal').modal('show');

    if (opt === 'statusChange' || opt === 'restartJasper' || opt === 'reprovisionToNew') {
      modal.on('shown.bs.modal', function() {
        $scope.modalOpen = true;
      });

      modal.on('hide.bs.modal', function() {
        $scope.modalOpen = false;
      });
    }

    $(document).keyup(function(ev) {
      if ($scope.modalOpen && ev.keyCode === 13) {
        if ($scope.adminOption === 'statusChange') {
          $scope.updateStatus();
        } else if ($scope.adminOption === 'restartJasper') {
          $scope.setRestartJasper();
        } else if ($scope.adminOption === 'reprovisionToNew') {
          $scope.setReprovisionToNew();
        }
        $('#admin-modal').modal('hide');
      }
    });
  };
}]);


mciModule.directive('adminUpdateStatus', function() {
  return {
    restrict: 'E',
    templateUrl: '/static/partials/host_status_update.html'
  };
});

mciModule.directive('adminRestartJasper', function() {
  return {
    restrict: 'E',
    template: 
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Restart host Jasper service?' +
        '<button type="button" class="btn btn-danger" style="float: right;" ng-disabled="noClose" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setRestartJasper()" ng-disabled="noClose">' +
          '<span ng-if="!noClose">Yes</span>' +
        '</button>' +
      '</div>' +
    '</div>'
  };
});

mciModule.directive('adminReprovisionToNew', function() {
  return {
    restrict: 'E',
    template: 
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Reprovision host?' +
        '<button type="button" class="btn btn-danger" style="float: right;" ng-disabled="noClose" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setReprovisionToNew()" ng-disabled="noClose">' +
          '<span ng-if="!noClose">Yes</span>' +
        '</button>' +
      '</div>' +
    '</div>'
  };
});
