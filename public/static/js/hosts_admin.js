mciModule.controller('AdminOptionsCtrl', ['$scope', '$filter', 'mciHostsRestService', 'notificationService', function($scope, $filter, hostsRestService, notifier) {
  $scope.modalTitle = 'Modify Hosts';
  $scope.validHostStatuses = ["running", "decommissioned", "quarantined", "terminated", "stopped"];
  $scope.newStatus = $scope.validHostStatuses[0];
  $scope.notes={};

  $scope.updateStatus = function() {
    var selectedHosts = $scope.selectedHosts();
    var hostIds = [];
    for (var i = 0; i < selectedHosts.length; ++i) {
      hostIds.push(selectedHosts[i].id);
    }
    hostsRestService.updateStatus(
      hostIds,
      'updateStatus',
      { status: $scope.newStatus, notes: $scope.notes.text },
      {
        success: function(resp) {
          window.location.reload();
        },
        error: function(resp) {
          notifier.pushNotification('Error updating host status: ' + resp.data.error, 'errorModal');
        }
      }
    );
  };

  $scope.setRestartJasper = function() {
    var selectedHosts = $scope.selectedHosts();
    var hostIDs = [];
    for (var i = 0; i < selectedHosts.length; ++i) {
      hostIDs.push(selectedHosts[i].id);
    }
    hostsRestService.setRestartJasper(
      hostIDs,
      'restartJasper',
      {},
      {
        success: function(resp) {
          window.location.reload();
        },
        error: function(resp) {
          notifier.pushNotification('Error marking host(s) as needing Jasper service restarted: ' + resp.data, 'errorModal');
        }
      }
    );
  };

  $scope.setReprovisionToNew = function() {
    var selectedHosts = $scope.selectedHosts();
    var hostIDs = [];
    for (var i = 0; i < selectedHosts.length; ++i) {
      hostIDs.push(selectedHosts[i].id);
    }
    hostsRestService.setReprovisionToNew(
      hostIDs,
      'reprovisionToNew',
      {},
      {
        success: function(resp) {
          window.location.reload();
        },
        error: function(resp) {
          notifier.pushNotification('Error marking host(s) as needing to reprovision: ' + resp.data, 'errorModal');
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
    templateUrl: '/static/partials/hosts_status_update.html'
  };
});

mciModule.directive('adminRestartJasper', function() {
  return {
    restrict: 'E',
    template: 
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Restart Jasper service on [[hostCount]] [[hostCount | pluralize:\'host\']]?' +
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
        'Reprovision [[hostCount]] [[hostCount | pluralize:\'host\']]?' +
        '<button type="button" class="btn btn-danger" style="float: right;" ng-disabled="noClose" data-dismiss="modal">Cancel</button>' +
        '<button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="setReprovisionToNew()" ng-disabled="noClose">' +
          '<span ng-if="!noClose">Yes</span>' +
        '</button>' +
      '</div>' +
    '</div>'
  };
});
