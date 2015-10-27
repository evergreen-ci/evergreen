mciModule.controller('AdminOptionsCtrl', ['$scope', '$filter', 'mciHostsRestService', 'errorPasserService', function($scope, $filter, hostsRestService, errorPasser) {
  $scope.modalTitle = 'Modify Hosts';
  $scope.validHostStatuses = ["running", "decommissioned", "quarantined"];
  $scope.newStatus = $scope.validHostStatuses[0];

  $scope.updateStatus = function() {
    var selectedHosts = $scope.selectedHosts();
    var hostIds = [];
    for (var i = 0; i < selectedHosts.length; ++i) {
      hostIds.push(selectedHosts[i].id);
    }
    hostsRestService.updateStatus(
      hostIds,
      'updateStatus',
      { status: $scope.newStatus },
      {
        success: function(data, status) {
          window.location.reload();
        },
        error: function(jqXHR, status, errorThrown) {
          errorPasser.pushError('Error updating host status: ' + jqXHR, 'errorModal');
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
    templateUrl: '/static/partials/hosts_status_update.html'
  };
});

