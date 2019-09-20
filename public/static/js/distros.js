mciModule.controller('DistrosCtrl', function($scope, $window, $location, $anchorScroll, $filter, mciDistroRestService) {

  $scope.readOnly = !$window.isSuperUser;

  $scope.distros = $window.distros;
  $scope.containerPoolDistros = $window.containerPoolDistros;
  $scope.containerPoolIds = $window.containerPoolIds;

  for (var i = 0; i < $scope.distros.length; i++) {
    $scope.distros[i].pool_size = $scope.distros[i].pool_size || 0;
    $scope.distros[i].planner_settings = $scope.distros[i].planner_settings || {};
    $scope.distros[i].planner_settings.version = $scope.distros[i].planner_settings.version || 'legacy';
    $scope.distros[i].planner_settings.minimum_hosts = $scope.distros[i].planner_settings.minimum_hosts || 0;
    $scope.distros[i].planner_settings.maximum_hosts = $scope.distros[i].planner_settings.maximum_hosts || 0;
    $scope.distros[i].planner_settings.target_time = $scope.distros[i].planner_settings.target_time || 0;
    $scope.distros[i].planner_settings.acceptable_host_idle_time = $scope.distros[i].planner_settings.acceptable_host_idle_time || 0;
    $scope.distros[i].planner_settings.patch_factor = $scope.distros[i].planner_settings.patch_factor || 0;
    $scope.distros[i].planner_settings.time_in_queue_factor = $scope.distros[i].planner_settings.time_in_queue_factor || 0;
    $scope.distros[i].planner_settings.expected_runtime_factor = $scope.distros[i].planner_settings.expected_runtime_factor || 0;
    // Convert from nanoseconds (time.Duration) to seconds (UI display units) for the relevant planner_settings' fields.
    if ($scope.distros[i].planner_settings.target_time > 0) {
      $scope.distros[i].planner_settings.target_time /= 1e9;
    }
    if ($scope.distros[i].planner_settings.acceptable_host_idle_time > 0) {
      $scope.distros[i].planner_settings.acceptable_host_idle_time /= 1e9;
    }
    $scope.distros[i].planner_settings.group_versions = $scope.distros[i].planner_settings.group_versions;
    $scope.distros[i].planner_settings.task_ordering = $scope.distros[i].planner_settings.task_ordering || 'interleave';
    $scope.distros[i].finder_settings = $scope.distros[i].finder_settings || {};
    $scope.distros[i].finder_settings.version = $scope.distros[i].finder_settings.version || 'legacy';
    $scope.distros[i].bootstrap_settings.method = $scope.distros[i].bootstrap_settings.method || 'legacy-ssh';
    $scope.distros[i].bootstrap_settings.communication = $scope.distros[i].bootstrap_settings.communication || 'legacy-ssh';
    $scope.distros[i].clone_method = $scope.distros[i].clone_method || 'legacy-ssh';
  }

  $scope.plannerVersions = [{
    'id': "legacy",
    'display': 'Legacy '
  }, {
    'id': "revised",
    'display': 'Revised '
  }, {
    'id': "tunable",
    'display': 'Tunable '
  }];

  $scope.taskOrderings = [{
    'id': "",
    'display': ' '
  }, {
   'id': "interleave",
    'display': 'Interleave '
  }, {
    'id': "mainlinefirst",
    'display': 'Mainline First '
  }, {
    'id': "patchfirst",
    'display': 'Patch First '
  }];

  $scope.providers = [{
    'id': 'ec2-auto',
    'display': 'EC2 Auto'
  }, {
    'id': 'ec2-ondemand',
    'display': 'EC2 On-Demand'
  }, {
    'id': 'ec2-spot',
    'display': 'EC2 Spot'
  }, {
    'id': 'ec2-fleet',
    'display': 'EC2 Fleet'
  }, {
    'id': 'static',
    'display': 'Static IP/VM'
  }, {
    'id': 'docker',
    'display': 'Docker'
  }, {
    'id': 'openstack',
    'display': 'OpenStack'
  }, {
    'id': 'gce',
    'display': 'Google Compute'
  }, {
    'id': 'vsphere',
    'display': 'VMware vSphere'
  }];

  $scope.architectures = [{
    'id': 'windows_amd64',
    'display': 'Windows 64-bit'
  }, {
    'id': 'linux_ppc64le',
    'display': 'Linux PowerPC 64-bit'
  }, {
    'id': 'linux_s390x',
    'display': 'Linux zSeries'
  }, {
    'id': 'linux_arm64',
    'display': 'Linux ARM 64-bit'
  }, {
    'id': 'windows_386',
    'display': 'Windows 32-bit'
  }, {
    'id': 'darwin_amd64',
    'display': 'OSX 64-bit'
  }, {
    'id': 'linux_amd64',
    'display': 'Linux 64-bit'
  }, {
    'id': 'linux_386',
    'display': 'Linux 32-bit'
  }];

  $scope.bootstrapMethods = [{
      'id': 'legacy-ssh',
      'display': 'Legacy SSH'
    }, {
      'id': 'ssh',
      'display': 'SSH'
    }, {
      'id': 'preconfigured-image',
      'display': 'Preconfigured Image'
    }, {
      'id': 'user-data',
      'display': 'User Data'
    }, {
  }];

  $scope.communicationMethods = [{
      'id': 'legacy-ssh',
      'display': 'Legacy SSH'
  }, {
      'id': 'ssh',
      'display': 'SSH'
  }, {
      'id': 'rpc',
      'display': 'RPC'
  }]

    $scope.cloneMethods = [{
      'id': 'legacy-ssh',
      'display': 'Legacy SSH'
    }, {
      'id':  'oauth',
      'display': 'OAuth Token'
    }]

  $scope.ids = [];

  $scope.keys = [];

  $scope.modalOpen = false;

  $scope.$on('$locationChangeStart', function(event) {
    $scope.hashLoad();
  });

  $scope.hashLoad = function() {
    var distroHash = $location.hash();
    if (distroHash) {
      // If the distro exists, load it.
      var distro = $scope.getDistroById(distroHash);
      if (distro) {
    $scope.activeDistro = distro;
    return;
      }
    }
    // Default to the first distro.
    $scope.setActiveDistro($scope.distros[0]);
  };

  $scope.setActiveDistro = function(distro) {
    $scope.activeDistro = distro;
    $location.hash(distro._id);
  };

  $scope.getDistroById = function(id) {
    return _.find($scope.distros, function(distro) {
    return distro._id === id;
    });
  };

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

  $scope.addInstanceSSHKey = function(ssh_key) {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.ssh_keys == null) {
      $scope.activeDistro.settings.ssh_keys = [];
    }
    $scope.activeDistro.settings.ssh_keys.push({});
    $scope.scrollElement('#ssh-keys-table');
  }

  $scope.removeInstanceSSHKey = function(ssh_key) {
    var index = $scope.activeDistro.settings.ssh_keys.indexOf(ssh_key);
    $scope.activeDistro.settings.ssh_keys.splice(index, 1);
  }

  $scope.addNetworkTag = function(tag) {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.network_tags == null) {
      $scope.activeDistro.settings.network_tags = [];
    }
    $scope.activeDistro.settings.network_tags.push('');
    $scope.scrollElement('#network-tags-table');
  }

  $scope.removeNetworkTag = function(tag) {
    var index = $scope.activeDistro.settings.network_tags.indexOf(tag);
    $scope.activeDistro.settings.network_tags.splice(index, 1);
  }

  $scope.addSSHOption = function() {
    if ($scope.activeDistro.ssh_options == null) {
      $scope.activeDistro.ssh_options = [];
    }
    $scope.activeDistro.ssh_options.push('');
    $scope.scrollElement('#ssh-options-table');
  }

  $scope.addDistroAlias = function() {
    if ($scope.activeDistro.aliases == null) {
      $scope.activeDistro.aliases = [];
    }
    $scope.activeDistro.aliases.push('');
    $scope.scrollElement('#distro-aliases-table');
  }

  $scope.removeDistroAlias = function(alias_name) {
    var index = $scope.activeDistro.aliases.indexOf(alias_name);
    $scope.activeDistro.aliases.splice(index, 1);
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
    // Convert from UI display units (seconds) to nanoseconds (time.Duration) for relevant planner_settings' fields.
    if($scope.activeDistro.planner_settings.target_time > 0) {
      $scope.activeDistro.planner_settings.target_time *= 1e9
    }
    if($scope.activeDistro.planner_settings.acceptable_host_idle_time > 0) {
      $scope.activeDistro.planner_settings.acceptable_host_idle_time *= 1e9
    }
    if ($scope.activeDistro.new) {
      mciDistroRestService.addDistro(
      $scope.activeDistro, {
      success: function(resp) {
        $window.location.reload(true);
      },
      error: function(resp) {
        $window.location.reload(true);
        console.log(resp.data.error);
        }
      }
      );
    } else {
      mciDistroRestService.modifyDistro(
      $scope.activeDistro._id,
      $scope.activeDistro,
      $scope.shouldDeco,
    {
      success: function(resp) {
        $window.location.reload(true);
      },
      error: function(resp) {
        $window.location.reload(true);
        console.log(resp.data.error);
      }
    }
      );
    }
    // this will reset the location hash to the new one in case the _id is changed.
    $scope.setActiveDistro($scope.activeDistro)
  };

  $scope.removeConfiguration = function() {
    mciDistroRestService.removeDistro(
      $scope.activeDistro._id,
      $scope.shouldDeco,
      {
    success: function(resp) {
      $window.location.reload(true);
    },
    error: function(resp) {
      $window.location.reload(true);
      console.log(resp.data.error);
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
        'bootstrap_settings': {
            'method': 'legacy-ssh',
            'communication': 'legacy-ssh'
        },
        'clone_method': 'legacy-ssh',
        'settings': {},
        'planner_settings': {
          'version': 'legacy',
          'minimum_hosts': 0
        },
        'finder_settings': {
          'version': 'legacy'
        },
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
    $anchorScroll();
  };

  $scope.copyDistro = function(){
    if (!$scope.hasNew) {
      var newDistro = {
        'arch': $scope.activeDistro.arch,
        'work_dir': $scope.activeDistro.work_dir,
        'provider': $scope.activeDistro.provider,
        'new': true,
        'user': $scope.activeDistro.user,
        'ssh_key': $scope.activeDistro.ssh_key,
        'ssh_options': $scope.activeDistro.ssh_options,
        'setup': $scope.activeDistro.setup,
        'setup': $scope.activeDistro.teardown,
        'setup': $scope.activeDistro.user_data,
        'pool_size': $scope.activeDistro.pool_size,
        'setup_as_sudo' : $scope.activeDistro.setup_as_sudo,
        'bootstrap_settings': $scope.activeDistro.bootstrap_settings,
        'clone_method': $scope.activeDistro.clone_method,
      };
      newDistro.settings = _.clone($scope.activeDistro.settings);
      newDistro.expansions = _.clone($scope.activeDistro.expansions);
      newDistro.planner_settings = _.clone($scope.activeDistro.planner_settings);
      newDistro.finder_settings = _.clone($scope.activeDistro.finder_settings);

      $scope.distros.unshift(newDistro);
      $scope.hasNew = true;
      $scope.setActiveDistro($scope.distros[0]);
      $('#distros-list-container').animate({ scrollTop: 0 }, 'slow');
      $anchorScroll();
      }
    }


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
    if ($scope.form.portRange.minPort.$invalid || $scope.form.portRange.maxPort.$invalid) {
    return false
    }
    return (!min && !max) || (min >= 0 && min <= max);
  }

  $scope.checkPoolID = function(id) {
    if ($scope.form.poolID.$invalid) {
      return false
    }
    return $scope.containerPoolIds.includes(id)
  }

  $scope.displayContainerPool = function(id){
    return ($filter('filter')($window.containerPools, {'id':id}))[0];
  };

  // checks that the form is valid for the given active distro
  $scope.validForm = function() {
    if (!$scope.validBootstrapAndCommunication()) {
      return false;
    }
    if ($scope.activeDistro.provider.startsWith('ec2')) {
      return $scope.validSecurityGroup() && $scope.validSubnetId();
    }
    return true;
  }

  $scope.validBootstrapAndCommunication = function() {
    if ($scope.activeDistro) {
      return ($scope.activeDistro.bootstrap_settings.method == 'legacy-ssh' && $scope.activeDistro.bootstrap_settings.communication == 'legacy-ssh') ||
             ($scope.activeDistro.bootstrap_settings.method != 'legacy-ssh' && $scope.activeDistro.bootstrap_settings.communication != 'legacy-ssh');
    }
    return true;
  };


  // if a security group is in a vpc it needs to be the id which starts with 'sg-'
  $scope.validSecurityGroup = function(){
    if ($scope.activeDistro){
      if ($scope.activeDistro.settings.is_vpc && $scope.activeDistro.settings.security_group_ids){
        for (var i = 0; i < $scope.activeDistro.settings.security_group_ids.length; i++) {
          if ($scope.activeDistro.settings.security_group_ids[i].substring(0,3) !== "sg-") {
            return false
          }
        }
     }
   }
   return true
 };

    // if a security group is in a vpc it needs to be the id which starts with 'subnet-'
    $scope.validSubnetId = function(){
      if ($scope.activeDistro){
    if ($scope.activeDistro.settings.is_vpc) {
      return $scope.activeDistro.settings.subnet_id.substring(0,7) == 'subnet-';
    }
      }
      return true
    };

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
      '     Are you sure you want to remove [[activeDistro._id]]?' +
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

mciModule.filter("versionDisplay", function() {
  return function(version, scope) {
    return scope.getKeyDisplay('plannerVersions', version);
  }
});

mciModule.filter('taskOrderingDisplay', function() {
  return function(taskOrdering, scope) {
    return scope.getKeyDisplay('taskOrderings', taskOrdering);
  }
});

mciModule.filter('bootstrapMethodDisplay', function() {
  return function(bootstrapMethod, scope) {
    return scope.getKeyDisplay('bootstrapMethods', bootstrapMethod);
  }
});

mciModule.filter('communicationMethodDisplay', function() {
  return function(communicationMethod, scope) {
    return scope.getKeyDisplay('communicationMethods', communicationMethod);
  }
});

mciModule.filter('cloneMethodDisplay', function() {
  return function(cloneMethod, scope) {
    return scope.getKeyDisplay('cloneMethods', cloneMethod);
  }
})

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
