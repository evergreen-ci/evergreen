mciModule.controller('DistrosCtrl', function ($scope, $window, $http, $location, $anchorScroll, $filter, mciDistroRestService) {

  $scope.createDistro = $window.canCreateDistro;
  $scope.distroIds = $window.distroIds;
  $scope.containerPoolDistros = $window.containerPoolDistros;
  $scope.containerPoolIds = $window.containerPoolIds;
  $scope.validHostAllocatorRoundingRules = $window.validHostAllocatorRoundingRules;
  $scope.validHostAllocatorFeedbackRules = $window.validHostAllocatorFeedbackRules;
  $scope.validHostsOverallocatedRules = $window.validHostsOverallocatedRules;

  let newId = "new distro"

  $scope.hostAllocatorVersions = [{
    'id': 'utilization',
    'display': 'Utilization '
  }];

  $scope.finderVersions = [{
    'id': 'legacy',
    'display': 'Legacy '
  }, {
    'id': 'parallel',
    'display': 'Parallel '
  }, {
    'id': 'pipeline',
    'display': 'Pipeline '
  }, {
    'id': 'alternate',
    'display': 'Alternate '
  }];

  $scope.plannerVersions = [{
    'id': 'legacy',
    'display': 'Legacy '
  }, {
    'id': 'tunable',
    'display': 'Tunable '
  }];

  $scope.dispatcherVersions = [{
    'id': 'revised',
    'display': 'Revised '
  }, {
    'id': 'revised-with-dependencies',
    'display': 'Revised w/Dependencies '
  }];

  $scope.providers = [{
    'id': 'ec2-ondemand',
    'display': 'EC2 On-Demand'
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

  $scope.bootstrapMethods = [{
    'id': 'legacy-ssh',
    'display': 'Legacy SSH'
  }, {
    'id': 'ssh',
    'display': 'SSH'
  }, {
    'id': 'user-data',
    'display': 'User Data'
  }, {}];

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
    'id': 'oauth',
    'display': 'OAuth Token'
  }]

  $scope.fleetInstanceTypes = [{
    'id': 'spot',
    'display': 'Spot'
  }, {
    'id': 'fallback',
    'display': 'Spot with on-demand fallback'
  }, {
    'id': 'on-demand',
    'display': 'On-demand'
  }]

  $scope.ids = [];
  $scope.keys = [];
  $scope.architectures = [];

  $scope.modalOpen = false;

  $scope.$on('$locationChangeStart', function (event) {
    $scope.hashLoad();
  });

  $scope.hashLoad = function () {
    var distroHash = $location.hash();
    if (distroHash) {
      // If the distro exists, load it.
      if (distroHash === newId && $scope.tempDistro) {
        $scope.activeDistro = $scope.tempDistro;
      } else if (distroHash != newId) {
        $scope.getDistroById(distroHash).then(function (distro) {
          if (distro) {
            $scope.regions = distro.regions;
            $scope.readOnly = distro.permissions.distro_settings < 20
            $scope.remove = distro.permissions.distro_settings >= 30
            $scope.activeDistro = distro.distro;
          }
        });
      }
    } else {
      // default to first id
      $scope.setActiveDistroId(distroIds[0]);
    }
  };

  $scope.setFleetInstanceType = function(instanceType) {
    if ($scope.activeDistro.settings.fleet_options === undefined) {
      $scope.activeDistro.settings.fleet_options = {};
    }
    switch(instanceType) {
      case "spot":
        $scope.activeDistro.settings.fleet_options.use_on_demand = false;
        $scope.activeDistro.settings.fallback = false;
        break;
      case "fallback":
        $scope.activeDistro.settings.fleet_options.use_on_demand = false;
        $scope.activeDistro.settings.fallback = true;
        break;
      case "on-demand":
        $scope.activeDistro.settings.fleet_options.use_on_demand = true;
        $scope.activeDistro.settings.fallback = false;
        break;
    }
  }

  $scope.getFleetInstanceType = function(settings) {
    if (!(settings && settings.fleet_options && settings.fleet_options.use_on_demand) && (settings && settings.fallback)) {
      return "fallback";
    }

    if ((settings && settings.fleet_options && settings.fleet_options.use_on_demand) && !(settings && settings.fallback)) {
      return "on-demand";
    }

    return "spot";
  }

  $scope.setActiveDistroId = function (id) {
    $location.hash(id);
  };

  $scope.getDistroById = function (id) {
    if ($scope.tempDistro && $scope.tempDistro._id === id) {
      return new Promise(function (resolve) {
        resolve();
      });
    }
    return $http.get('/distros/' + id).then(
      function (resp) {
        var distro = resp.data
        if (distro.distro) {
          // Host Allocator Settings
          distro.distro.host_allocator_settings = distro.distro.host_allocator_settings || {};
          distro.distro.host_allocator_settings.version = distro.distro.host_allocator_settings.version || 'utilization';
          distro.distro.host_allocator_settings.minimum_hosts = distro.distro.host_allocator_settings.minimum_hosts || 0;
          distro.distro.host_allocator_settings.maximum_hosts = distro.distro.host_allocator_settings.maximum_hosts || 0;
          distro.distro.host_allocator_settings.acceptable_host_idle_time = distro.distro.host_allocator_settings.acceptable_host_idle_time || 0;
          // Convert from nanoseconds (time.Duration) to seconds (UI display units) for the relevant host_allocator_settings' fields.
          if (distro.distro.host_allocator_settings.acceptable_host_idle_time > 0) {
            distro.distro.host_allocator_settings.acceptable_host_idle_time /= 1e9;
          }
          // Planner Settings
          distro.distro.planner_settings = distro.distro.planner_settings || {};
          distro.distro.planner_settings.version = distro.distro.planner_settings.version || 'legacy';
          distro.distro.planner_settings.target_time = distro.distro.planner_settings.target_time || 0;
          distro.distro.planner_settings.patch_factor = distro.distro.planner_settings.patch_factor || 0;
          distro.distro.planner_settings.time_in_queue_factor = distro.distro.planner_settings.time_in_queue_factor || 0;
          distro.distro.planner_settings.expected_runtime_factor = distro.distro.planner_settings.expected_runtime_factor || 0;
          distro.distro.planner_settings.generate_task_factor = distro.distro.planner_settings.generate_task_factor || 0;

          // Convert from nanoseconds (time.Duration) to seconds (UI display units) for the relevant planner_settings' fields.
          if (distro.distro.planner_settings.target_time > 0) {
            distro.distro.planner_settings.target_time /= 1e9;
          }
          distro.distro.planner_settings.group_versions = distro.distro.planner_settings.group_versions;
          // Finder Settings
          distro.distro.finder_settings = distro.distro.finder_settings || {};
          distro.distro.finder_settings.version = distro.distro.finder_settings.version || 'legacy';
          // Dispatcher Settings
          distro.distro.dispatcher_settings = distro.distro.dispatcher_settings || {};
          distro.distro.dispatcher_settings.version = distro.distro.dispatcher_settings.version || 'revised';
          distro.distro.bootstrap_settings.method = distro.distro.bootstrap_settings.method || 'legacy-ssh';
          distro.distro.bootstrap_settings.communication = distro.distro.bootstrap_settings.communication || 'legacy-ssh';
          distro.distro.clone_method = distro.distro.clone_method || 'legacy-ssh';
          distro.distro.icecream_settings = distro.distro.icecream_settings || {};

          // current settings from provider settings
          if (distro.distro.provider_settings === undefined || distro.distro.provider_settings.length === 0) {
            distro.distro.provider_settings = [$scope.getNewProviderSettings(distro.distro.provider)];
          }
          distro.distro.settings = distro.distro.provider_settings[0];
          $scope.currentIdx = 0;
        }
        return distro
      },
      function (resp) {
        console.log(resp.status)
      });
  };

  $scope.updateSettingsList = function() {
    // don't save if there aren't any populated fields
    for (let [key, val] of Object.entries($scope.activeDistro.settings)) {
      // If using a static host list, unset any invalid SSH ports (i.e. null, undefined) because the back-end will treat
      // a conversion from nil to an integer as invalid.
      if ($scope.activeDistro.settings.hosts) {
        for (var i = 0; i < $scope.activeDistro.settings.hosts.length; i++) {
          if (!$scope.activeDistro.settings.hosts[i].ssh_port) {
            delete($scope.activeDistro.settings.hosts[i].ssh_port);
          }
        }
      }
      if (key !== "region" && val !== "" && val !== undefined) {
        $scope.activeDistro.provider_settings[$scope.currentIdx] = $scope.activeDistro.settings;
        return;
      }
    }
    // if the settings object is cleared, then we should remove from the list for now
    $scope.activeDistro.provider_settings.splice($scope.currentIdx);
  };

  $scope.switchRegion = function(region) {
    // save current settings to the overall provider settings list
    $scope.updateSettingsList();

    for (var i = 0; i < $scope.activeDistro.provider_settings.length; i++) {
      if ($scope.activeDistro.provider_settings[i].region === region) {
        $scope.activeDistro.settings = $scope.activeDistro.provider_settings[i];
        $scope.currentIdx = i;
        return;
      }
    }

    // if we didn't find the region in the provider settings list, then we must be adding a new one
    $scope.activeDistro.settings = $scope.getNewProviderSettings($scope.activeDistro.provider, region);
    $scope.activeDistro.provider_settings.push($scope.activeDistro.settings);
    $scope.currentIdx = $scope.activeDistro.provider_settings.length - 1;
  };

  $scope.getNewProviderSettings = function(provider, region) {
      if (provider.startsWith('ec2')) {
        if (region !== "") {
          return {"region": region};
        }
        return {"region": "us-east-1"};
      }
      return {};
  };

  // clear every field but region for the current settings object
  $scope.clearSettings = function() {
    $scope.activeDistro.settings = $scope.getNewProviderSettings($scope.activeDistro.provider, $scope.activeDistro.settings.region)
  };

  $scope.initOptions = function () {
    var keys = [];

    if ($window.keys !== null) {
      keys = Object.keys($window.keys);
    }

    for (var i = 0; i < $scope.distroIds.length; i++) {
      $scope.ids.push($scope.distroIds[i]);
    }

    for (var i = 0; i < keys.length; i++) {
      $scope.keys.push({
        name: keys[i],
        location: $window.keys[keys[i]],
      });
    }

    if ($window.architectures != null) {
      keys = Object.keys($window.architectures);
      for (var i = 0; i < keys.length; i++) {
        $scope.architectures.push({
          'id': keys[i],
          'display': $window.architectures[keys[i]],
        })
      };
    }
  };

  $scope.isUnique = function (id) {
    return $scope.ids.indexOf(id) == -1;
  };

  $scope.setKeyValue = function (key, value) {
    $scope.activeDistro[key] = value;
  };

  $scope.getKeyDisplay = function (key, display) {
    for (var i = 0; i < $scope[key].length; i++) {
      if ($scope[key][i].id === display) {
        return $scope[key][i].display;
      }
    }
    return display;
  };

  $scope.addHost = function () {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.hosts == null) {
      $scope.activeDistro.settings.hosts = [];
    }
    $scope.activeDistro.settings.hosts.push({});
    $scope.scrollElement('#hosts-table');
  }

  $scope.removeHost = function (host) {
    var index = $scope.activeDistro.settings.hosts.indexOf(host);
    $scope.activeDistro.settings.hosts.splice(index, 1);
  }

  $scope.addMount = function (mount_point) {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.mount_points == null) {
      $scope.activeDistro.settings.mount_points = [];
    }
    $scope.activeDistro.settings.mount_points.push({});
    $scope.scrollElement('#mounts-table');
  }

  $scope.scrollElement = function (elt) {
    $(elt).animate({
      scrollTop: $(elt)[0].scrollHeight
    }, 'slow');
  }

  $scope.removeMount = function (mount_point) {
    var index = $scope.activeDistro.settings.mount_points.indexOf(mount_point);
    $scope.activeDistro.settings.mount_points.splice(index, 1);
  }

  $scope.addInstanceSSHKey = function (ssh_key) {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.ssh_keys == null) {
      $scope.activeDistro.settings.ssh_keys = [];
    }
    $scope.activeDistro.settings.ssh_keys.push({});
    $scope.scrollElement('#ssh-keys-table');
  }

  $scope.removeInstanceSSHKey = function (ssh_key) {
    var index = $scope.activeDistro.settings.ssh_keys.indexOf(ssh_key);
    $scope.activeDistro.settings.ssh_keys.splice(index, 1);
  }

  $scope.addNetworkTag = function (tag) {
    if ($scope.activeDistro.settings == null) {
      $scope.activeDistro.settings = {};
    }
    if ($scope.activeDistro.settings.network_tags == null) {
      $scope.activeDistro.settings.network_tags = [];
    }
    $scope.activeDistro.settings.network_tags.push('');
    $scope.scrollElement('#network-tags-table');
  }

  $scope.removeNetworkTag = function (tag) {
    var index = $scope.activeDistro.settings.network_tags.indexOf(tag);
    $scope.activeDistro.settings.network_tags.splice(index, 1);
  }

  $scope.addSSHOption = function () {
    if ($scope.activeDistro.ssh_options == null) {
      $scope.activeDistro.ssh_options = [];
    }
    $scope.activeDistro.ssh_options.push('');
    $scope.scrollElement('#ssh-options-table');
  }

  $scope.addDistroAlias = function () {
    if ($scope.activeDistro.aliases == null) {
      $scope.activeDistro.aliases = [];
    }
    $scope.activeDistro.aliases.push('');
    $scope.scrollElement('#distro-aliases-table');
  }

  $scope.addMountpoint = function () {
    if ($scope.activeDistro.mountpoints == null) {
      $scope.activeDistro.mountpoints = [];
    }
    $scope.activeDistro.mountpoints.push('');
    $scope.scrollElement('#mountpoints-table');
  }

  $scope.removeDistroAlias = function (alias_name) {
    var index = $scope.activeDistro.aliases.indexOf(alias_name);
    $scope.activeDistro.aliases.splice(index, 1);
  }

  $scope.removeSSHOption = function (ssh_option) {
    var index = $scope.activeDistro.ssh_options.indexOf(ssh_option);
    $scope.activeDistro.ssh_options.splice(index, 1);
  }

  $scope.removeMountpoint = function (mountpoint) {
    var index = $scope.activeDistro.mountpoints.indexOf(mountpoint);
    $scope.activeDistro.mountpoints.splice(index, 1);
  }

  $scope.addPreconditionScript = function () {
    if ($scope.activeDistro.bootstrap_settings == null) {
      $scope.activeDistro.bootstrap_settings = {};
    }
    if ($scope.activeDistro.bootstrap_settings.precondition_scripts == null) {
      $scope.activeDistro.bootstrap_settings.precondition_scripts = [];
    }
    $scope.activeDistro.bootstrap_settings.precondition_scripts.push({});
    $scope.scrollElement('#precondition-scripts-table')
  }

  $scope.removePreconditionScript = function (index) {
    $scope.activeDistro.bootstrap_settings.precondition_scripts.splice(index, 1);
  }

  $scope.addEnvVar = function () {
    if ($scope.activeDistro.bootstrap_settings == null) {
      $scope.activeDistro.bootstrap_settings = {};
    }
    if ($scope.activeDistro.bootstrap_settings.env == null) {
      $scope.activeDistro.bootstrap_settings.env = [];
    }
    $scope.activeDistro.bootstrap_settings.env.push({
      "key": "",
      "value": ""
    });
  }

  $scope.removeEnvVar = function (envVar) {
    var index = $scope.activeDistro.bootstrap_settings.env.indexOf(envVar);
    $scope.activeDistro.bootstrap_settings.env.splice(index, 1);
  }

  $scope.addExpansion = function (expansion) {
    if ($scope.activeDistro.expansions == null) {
      $scope.activeDistro.expansions = [];
    }
    $scope.activeDistro.expansions.push({});
    $scope.scrollElement('#expansions-table');
  }

  $scope.removeExpansion = function (expansion) {
    var index = $scope.activeDistro.expansions.indexOf(expansion);
    $scope.activeDistro.expansions.splice(index, 1);
  }

  $scope.saveConfiguration = function () {
    // Convert from UI display units (seconds) to nanoseconds (time.Duration) for relevant *_settings' fields.
    if ($scope.activeDistro.planner_settings.target_time > 0) {
      $scope.activeDistro.planner_settings.target_time *= 1e9
    }
    if ($scope.activeDistro.host_allocator_settings.acceptable_host_idle_time > 0) {
      $scope.activeDistro.host_allocator_settings.acceptable_host_idle_time *= 1e9
    }
    $scope.updateSettingsList();
    $scope.activeDistro.settings = null;
    if ($scope.activeDistro.new) {
      mciDistroRestService.addDistro(
        $scope.activeDistro, {
          success: function (resp) {
            $window.location.reload(true);
          },
          error: function (resp) {
            $window.location.reload(true);
            console.log(resp.data.error);
          }
        }
      );
    } else {
      mciDistroRestService.modifyDistro(
        $scope.activeDistro._id,
        $scope.activeDistro,
        $scope.shouldDeco, $scope.shouldRestartJasper, $scope.shouldReprovisionToNew, {
          success: function (resp) {
            $window.location.reload(true);
          },
          error: function (resp) {
            $window.location.reload(true);
            console.log(resp.data.error);
          }
        }
      );
    }
    // this will reset the location hash to the new one in case the _id is changed.
    $scope.setActiveDistroId($scope.activeDistro._id)
  };

  $scope.removeConfiguration = function () {
    mciDistroRestService.removeDistro(
      $scope.activeDistro._id,
      $scope.shouldDeco, {
        success: function (resp) {
          $window.location.reload(true);
        },
        error: function (resp) {
          $window.location.reload(true);
          console.log(resp.data.error);
        }
      }
    );
  };

  $scope.newDistro = function () {
    if (!$scope.tempDistro) {
      var defaultOptions = {
        '_id': newId,
        'arch': 'linux_amd64',
        'provider': 'ec2-fleet',
        'bootstrap_settings': {
          'method': 'legacy-ssh',
          'communication': 'legacy-ssh'
        },
        'clone_method': 'legacy-ssh',
        'provider_settings': [$scope.getNewProviderSettings('ec2-fleet', "")], // empty list with one new object
        'finder_settings': {
          'version': 'legacy'
        },
        'planner_settings': {
          'version': 'legacy',
        },
        'dispatcher_settings': {
          'version': 'revised'
        },
        'host_allocator_settings': {
          'version': 'utilization'
        },
        'new': true,
      };

      if ($scope.keys.length != 0) {
        defaultOptions.ssh_key = $scope.keys[0].name;
      }
      $scope.tempDistro = defaultOptions;
      $scope.distroIds.unshift(defaultOptions._id);
      $scope.setActiveDistroId(defaultOptions._id);
    }
    $scope.setActiveDistroId($scope.distroIds[0]);
    $('#distros-list-container').animate({
      scrollTop: 0
    }, 'slow');
    $anchorScroll();
  };

  $scope.copyDistro = function () {
    if (!$scope.tempDistro) {
      var newDistro = {
        '_id': newId,
        'arch': $scope.activeDistro.arch,
        'work_dir': $scope.activeDistro.work_dir,
        'provider': $scope.activeDistro.provider,
        'new': true,
        'user': $scope.activeDistro.user,
        'ssh_key': $scope.activeDistro.ssh_key,
        'ssh_options': $scope.activeDistro.ssh_options,
        'mountpoints': $scope.activeDistro.mountpoints,
        'authorized_keys_file': $scope.activeDistro.authorized_keys_file,
        'setup': $scope.activeDistro.setup,
        'spawn_allowed': $scope.activeDistro.spawn_allowed,
        'user_data': $scope.activeDistro.user_data,
        'iam_instance_profile_arn': $scope.activeDistro.iam_instance_profile_arn,
        'setup_as_sudo': $scope.activeDistro.setup_as_sudo,
        'clone_method': $scope.activeDistro.clone_method,
        'is_virtual_workstation': $scope.activeDistro.is_virtual_workstation,
        'is_cluster': $scope.activeDistro.is_cluster,
        'disable_shallow_clone': $scope.activeDistro.disable_shallow_clone,
        'disabled': $scope.activeDistro.disabled,
        'merge_user_data': $scope.activeDistro.merge_user_data_parts,
      };
      newDistro.settings = _.clone($scope.activeDistro.settings);
      newDistro.provider_settings = _.clone($scope.activeDistro.provider_settings);
      newDistro.expansions = _.clone($scope.activeDistro.expansions);
      newDistro.bootstrap_settings = _.clone($scope.activeDistro.bootstrap_settings);
      newDistro.planner_settings = _.clone($scope.activeDistro.planner_settings);
      newDistro.finder_settings = _.clone($scope.activeDistro.finder_settings);
      newDistro.dispatcher_settings = _.clone($scope.activeDistro.dispatcher_settings);
      newDistro.host_allocator_settings = _.clone($scope.activeDistro.host_allocator_settings);
      newDistro.home_volume_settings = _.clone($scope.activeDistro.home_volume_settings);
      newDistro.icecream_settings = _.clone($scope.activeDistro.icecream_settings);

      $scope.distroIds.unshift(newDistro._id);
      $scope.tempDistro = newDistro;
      $scope.setActiveDistroId(newDistro._id);
      $('#distros-list-container').animate({
        scrollTop: 0
      }, 'slow');
      $anchorScroll();
    }
  }

  $scope.openConfirmationModal = function (option) {
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

    $(document).keyup(function (ev) {
      if ($scope.modalOpen && ev.keyCode === 13) {
        if ($scope.confirmationOption === 'removeDistro') {
          $scope.removeConfiguration();
          $('#admin-modal').modal('hide');
        }
      }
    });
  }

  $scope.checkPortRange = function (min, max) {
    if ($scope.form.portRange.minPort.$invalid || $scope.form.portRange.maxPort.$invalid) {
      return false
    }
    return (!min && !max) || (min >= 0 && min <= max);
  }

  $scope.checkPoolID = function (id) {
    if ($scope.form.poolID.$invalid) {
      return false
    }
    return $scope.containerPoolIds.includes(id)
  }

  $scope.displayContainerPool = function (id) {
    return ($filter('filter')($window.containerPools, {
      'id': id
    }))[0];
  };

  // checks that the form is valid for the given active distro
  $scope.validForm = function () {
    if (!$scope.validBootstrapAndCommunication()) {
      return false;
    }
    if ($scope.activeDistro.provider.startsWith('ec2')) {
      if ($scope.activeDistro) {
          $scope.updateSettingsList(); // remove any empty settings before validating
      }
      return $scope.validSecurityGroup() && $scope.validSubnetId();
    }
    return true;
  }

  $scope.isDefaultRegion = function() {
    // default region cannot be empty
    if (!$scope.activeDistro || !$scope.activeDistro.settings || $scope.activeDistro.settings.region === "us-east-1") {
      return true;
    }
    return false;
  }

  $scope.isWindows = function () {
    if ($scope.activeDistro && $scope.activeDistro.arch) {
      return $scope.activeDistro.arch.includes('windows');
    }
    return false
  }

  $scope.isLinux = function () {
    if ($scope.activeDistro && $scope.activeDistro.arch) {
      return $scope.activeDistro.arch.includes('linux');
    }
    return false;
  }

  $scope.isStatic = function () {
    if ($scope.activeDistro) {
      return $scope.activeDistro.provider === 'static'
    }
  }

  $scope.isNonLegacyProvisioning = function () {
    if ($scope.activeDistro && $scope.activeDistro.bootstrap_settings) {
      return $scope.activeDistro.bootstrap_settings.method != 'legacy-ssh' ||
        $scope.activeDistro.bootstrap_settings.communication != 'legacy-ssh'
    }
    return false;
  }

  $scope.validBootstrapAndCommunication = function () {
    if ($scope.activeDistro && $scope.activeDistro.bootstrap_settings) {
      return ($scope.activeDistro.bootstrap_settings.method == 'legacy-ssh' && $scope.activeDistro.bootstrap_settings.communication == 'legacy-ssh') ||
        ($scope.activeDistro.bootstrap_settings.method != 'legacy-ssh' && $scope.activeDistro.bootstrap_settings.communication != 'legacy-ssh');
    }
    return true;
  };


  // if a security group is in a vpc it needs to be the id which starts with 'sg-'
  $scope.validSecurityGroup = function () {
    if ($scope.activeDistro) {
      for (var i = 0; i < $scope.activeDistro.provider_settings; i++) {
        if ($scope.activeDistro.provider_settings[i].is_vpc && $scope.activeDistro.provider_settings[i].security_group_ids) {
          for (var i = 0; i < $scope.activeDistro.provider_settings[i].security_group_ids.length; i++) {
            if ($scope.activeDistro.provider_settings[i].security_group_ids[i].substring(0, 3) !== "sg-") {
                return false;
            }
          }
        }
      }
    }
    return true
  };

  // if a security group is in a vpc it needs to be the id which starts with 'subnet-'
  $scope.validSubnetId = function () {
    if ($scope.activeDistro) {
      for (var i = 0; i < $scope.activeDistro.provider_settings; i++) {
        if ($scope.activeDistro.provider_settings[i].is_vpc) {
            return $scope.activeDistro.provider_settings[i].subnet_id.substring(0, 7) == 'subnet-';
        }
      }
    }
    return true
  };

  // scroll to top of window on page reload
  $(window).on('beforeunload', function () {
    $(window).scrollTop(0);
  });

});

mciModule.directive('removeDistro', function () {
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

mciModule.filter("providerDisplay", function () {
  return function (provider, scope) {
    return scope.getKeyDisplay('providers', provider);
  }
});

mciModule.filter("fleetInstanceTypeDisplay", function () {
  return function (instanceType, scope) {
    return scope.getKeyDisplay('fleetInstanceTypes', instanceType);
  }
});

mciModule.filter("archDisplay", function () {
  return function (arch, scope) {
    return scope.getKeyDisplay('architectures', arch);
  }
});

mciModule.filter("finderVersionDisplay", function () {
  return function (version, scope) {
    return scope.getKeyDisplay('finderVersions', version);
  }
});

mciModule.filter("plannerVersionDisplay", function () {
  return function (version, scope) {
    return scope.getKeyDisplay('plannerVersions', version);
  }
});

mciModule.filter("hostAllocatorVersionDisplay", function () {
  return function (version, scope) {
    return scope.getKeyDisplay('hostAllocatorVersions', version);
  }
});

mciModule.filter("dispatcherVersionDisplay", function () {
  return function (version, scope) {
    return scope.getKeyDisplay('dispatcherVersions', version);
  }
});

mciModule.filter('bootstrapMethodDisplay', function () {
  return function (bootstrapMethod, scope) {
    return scope.getKeyDisplay('bootstrapMethods', bootstrapMethod);
  }
});

mciModule.filter('communicationMethodDisplay', function () {
  return function (communicationMethod, scope) {
    return scope.getKeyDisplay('communicationMethods', communicationMethod);
  }
});

mciModule.filter('cloneMethodDisplay', function () {
  return function (cloneMethod, scope) {
    return scope.getKeyDisplay('cloneMethods', cloneMethod);
  }
})

mciModule.directive('unique', function () {
  return {
    require: 'ngModel',
    link: function (scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function (value) {
        var valid = scope.isUnique(value);
        ctrl.$setValidity('unique', valid);
        return value;
      });
      ctrl.$formatters.unshift(function (value) {
        var valid = scope.isUnique(value);
        ctrl.$setValidity('unique', valid);
        return value;
      });
    }
  };
});
