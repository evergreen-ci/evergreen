mciModule.controller('DistrosCtrl', function($scope, $window, mciDistroRestService) {

  $scope.distros = $window.distros;

  $scope.providers = [{
    'id': 'ec2',
    'display': 'EC2 (On-Demand Instance)'
  }, {
    'id': 'ec2-spot',
    'display': 'EC2 (Spot Instance)'
  }, {
    'id': 'static',
    'display': 'Static IP/VM'
  }, {
    'id': 'digitalocean',
    'display': 'Digital Ocean'
  }, {
    'id': 'docker',
    'display': 'Docker'
  }];

  $scope.architectures = [{
    'id': 'windows_amd64',
    'display': 'Windows 64-bit'
  }, {
    'id': 'windows_386',
    'display': 'Windows 32-bit'
  }, {
    'id': 'darwin_amd64',
    'display': 'OSX 64-bit'
  }, {
    'id': 'darwin_386',
    'display': 'OSX 32-bit'
  }, {
    'id': 'linux_amd64',
    'display': 'Linux 64-bit'
  }, {
    'id': 'linux_386',
    'display': 'Linux 32-bit'
  }, {
    'id': 'solaris_amd64',
    'display': 'Solaris 64-bit'
  }];

  $scope.ids = [];

  $scope.keys = [];

  $scope.modalOpen = false;

  if ($scope.distros != null) {
    $scope.activeDistro = $scope.distros[0];
  }

  $scope.initOptions = function() {
    var keys = [];

    if ($window.keys !== null) {
      keys = Object.keys($window.keys);
    }

    for (var i = 0; i < $scope.distros.length; i++) {
      $scope.ids.push($scope.distros[i]._id);
    }

    for (var i = 0; i < keys.length; i++) {
      $scope.keys.push({
        name: keys[i],
        location: $window.keys[keys[i]],
      });
    }
  };

  $scope.isUnique = function(id) {
    return $scope.ids.indexOf(id) == -1;
  };

  $scope.setActiveDistro = function(distro) {
    $scope.activeDistro = distro;
  };

  $scope.setKeyValue = function(key, value) {
    $scope.activeDistro[key] = value;
  };

  $scope.getKeyDisplay = function(key, display) {
    for (var i = 0; i < $scope[key].length; i++) {
      if ($scope[key][i].id === display) {
        return $scope[key][i].display;
      }
    }
    return display;
  };

  $scope.addHost = function() {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.hosts == null) {
      $scope.activeDistro.settings.hosts = [];
    }
    $scope.activeDistro.settings.hosts.push({});
    $scope.scrollElement('#hosts-table');
  }

  $scope.removeHost = function(host) {
    var index = $scope.activeDistro.settings.hosts.indexOf(host);
    $scope.activeDistro.settings.hosts.splice(index, 1);
  }

  $scope.addMount = function(mount_point) {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.mount_points == null) {
      $scope.activeDistro.settings.mount_points = [];
    }
    $scope.activeDistro.settings.mount_points.push({});
    $scope.scrollElement('#mounts-table');
  }

  $scope.scrollElement = function(elt) {
    $(elt).animate({
      scrollTop: $(elt)[0].scrollHeight
    }, 'slow');
  }

  $scope.removeMount = function(mount_point) {
    var index = $scope.activeDistro.settings.mount_points.indexOf(mount_point);
    $scope.activeDistro.settings.mount_points.splice(index, 1);
  }

  $scope.addSSHOption = function() {
    if ($scope.activeDistro.ssh_options == null) {
      $scope.activeDistro.ssh_options = [];
    }
    $scope.activeDistro.ssh_options.push('');
    $scope.scrollElement('#ssh-options-table');
  }

  $scope.removeSSHOption = function(ssh_option) {
    var index = $scope.activeDistro.ssh_options.indexOf(ssh_option);
    $scope.activeDistro.ssh_options.splice(index, 1);
  }

  $scope.addExpansion = function(expansion) {
    if ($scope.activeDistro.expansions == null) {
      $scope.activeDistro.expansions = [];
    }
    $scope.activeDistro.expansions.push({});
    $scope.scrollElement('#expansions-table');
  }

  $scope.removeExpansion = function(expansion) {
    var index = $scope.activeDistro.expansions.indexOf(expansion);
    $scope.activeDistro.expansions.splice(index, 1);
  }

  $scope.saveConfiguration = function() {
    if ($scope.activeDistro.new) {
      mciDistroRestService.addDistro(
        $scope.activeDistro, {
          success: function(distros, status) {
            $window.location.reload(true);
          },
          error: function(jqXHR, status, errorThrown) {
            $window.location.reload(true);
            console.log(jqXHR);
          }
        }
      );
    } else {
      mciDistroRestService.modifyDistro(
        $scope.activeDistro._id,
        $scope.activeDistro, {
          success: function(distros, status) {
            $window.location.reload(true);
          },
          error: function(jqXHR, status, errorThrown) {
            $window.location.reload(true);
            console.log(jqXHR);
          }
        }
      );
    }
  };

  $scope.removeConfiguration = function() {
    mciDistroRestService.removeDistro(
      $scope.activeDistro._id, {
        success: function(distros, status) {
          $window.location.reload(true);
        },
        error: function(jqXHR, status, errorThrown) {
          $window.location.reload(true);
          console.log(jqXHR);
        }
      }
    );
  };

  $scope.newDistro = function() {
    if (!$scope.hasNew) {
      var defaultOptions = {
        '_id': 'new distro',
        'arch': 'linux_amd64',
        'provider': 'ec2',
        'new': true,
      };

      if ($scope.keys.length != 0) {
        defaultOptions.ssh_key = $scope.keys[0].name;
      }
      $scope.distros.unshift(defaultOptions);
      $scope.hasNew = true;
    }
    $scope.setActiveDistro($scope.distros[0]);
    $('#distros-list-container').animate({ scrollTop: 0 }, 'slow');
    $('#identifier').focus();
  };

  $scope.openConfirmationModal = function(option) {
    $scope.confirmationOption = option;
    $scope.modalTitle = 'Configuration';
    var modal = $('#admin-modal').modal('show');

    if (option === 'removeDistro') {
      if (modal.data('bs.modal').isShown) {
        $scope.modalOpen = true;
      } else {
        $scope.modalOpen = false;
      }
    }

    $(document).keyup(function(ev) {
      if ($scope.modalOpen && ev.keyCode === 13) {
        if ($scope.confirmationOption === 'removeDistro') {
          $scope.removeConfiguration();
          $('#admin-modal').modal('hide');
        }
      }
    });
  }

  $scope.checkPortRange = function(min, max) {
    if ($scope.form.portRange.minPort.$invalid) 
        || ($scope.form.portRange.maxPort.$invalid) {
        return false
    } 
    return (!min && !max) || (min >= 0 && min <= max);
  }

  // scroll to top of window on page reload
  $(window).on('beforeunload', function() {
    $(window).scrollTop(0);
  });

});

mciModule.directive('removeDistro', function() {
  return {
    restrict: 'E',
    template: '<div class="row">' +
      ' <div class="col-lg-12">' +
      '   <div>' +
      '     Are you sure you want to remove this distro?' +
      '     <div style="float:right">' +
      '       <button type="button" class="btn btn-danger" style="float: right;" data-dismiss="modal">Cancel</button>' +
      '       <button type="button" class="btn btn-primary" style="float: right; margin-right: 10px;" ng-click="removeConfiguration()">Yes</button>' +
      '     </div>' +
      '   </div>' +
      ' </div>' +
      '</div>'
  }
});

mciModule.filter("providerDisplay", function() {
  return function(provider, scope) {
    return scope.getKeyDisplay('providers', provider);
  }
});

mciModule.filter("archDisplay", function() {
  return function(arch, scope) {
    return scope.getKeyDisplay('architectures', arch);
  }
});

mciModule.directive('unique', function() {
  return {
    require: 'ngModel',
    link: function(scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function(value) {
        var valid = scope.isUnique(value);
        ctrl.$setValidity('unique', valid);
        return value;
      });
      ctrl.$formatters.unshift(function(value) {
        var valid = scope.isUnique(value);
        ctrl.$setValidity('unique', valid);
        return value;
      });
    }
  };
});
