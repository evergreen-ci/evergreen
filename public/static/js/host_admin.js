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

  $scope.setHostStatus = function(status) {
    $scope.newStatus = status;
  };

  $scope.openAdminModal = function(opt) {
    $scope.adminOption = opt;
    $scope.modalOpen = true;
    var modal = $('#admin-modal').modal('show');

    if (opt === "statusChange") {
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
          $('#admin-modal').modal('hide');
        }
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
