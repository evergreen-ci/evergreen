mciModule.controller('SpawnedHostsCtrl', ['$scope', '$window', '$timeout', '$q', '$location', 'mciSpawnRestService', 'notificationService', function ($scope, $window, $timeout, $q, $location, mciSpawnRestService, notificationService) {
    $scope.userTz = $window.userTz;
    $scope.defaultRegion = $window.defaultRegion;
    $scope.hosts = [];
    $scope.modalOpen = false;
    $scope.spawnTask = $window.spawnTask;
    $scope.spawnDistro = $window.spawnDistro;

    // variables for spawning a new host
    $scope.spawnableDistros = [];
    $scope.selectedDistro = {};
    $scope.userKeys = [];
    $scope.selectedKey = {};
    $scope.spawnInfo = {};
    $scope.curHostData;
    $scope.curVolumeData;
    $scope.maxHostsPerUser = $window.maxHostsPerUser;
    $scope.maxUnexpirableHostsPerUser = $window.maxUnexpirableHostsPerUser;
    $scope.maxUnexpirableVolumesPerUser = $window.maxUnexpirableVolumesPerUser;
    $scope.maxVolumeSizePerUser = $window.maxVolumeSizePerUser;
    $scope.setupScriptPath = $window.setupScriptPath;
    $scope.spawnReqSent = false;
    $scope.volumeReqSent = false;
    $scope.useTaskConfig = false;
    $scope.isVirtualWorkstation = false;
    $scope.noExpiration = false;
    $scope.volumeSizeDefault = 500;
    $scope.homeVolumeSize = $scope.volumeSizeDefault;
    $scope.homeVolumeID;
    $scope.volumeAttachHost;
    $scope.createVolumeInfo = {
        'type': 'gp2',
        'zone': 'us-east-1a',
    };
    $scope.allowedInstanceTypes = [];
    $scope.volumes = [];

    // max of 7 days time to expiration
    $scope.maxHoursToExpiration = 24 * 7;
    $scope.saveKey = false;
    $scope.currKeyName = '';
    $scope.newKey = {
      'name': '',
      'key': '',
    };

    $scope.resourceTypes = ["hosts", "volumes"];

    $scope.currentResourceType = function() {
      var params = $location.search()
      if ($scope.resourceTypes.includes(params["resourcetype"])) {
        return params["resourcetype"];
      }

      $location.search("resourcetype", $scope.resourceTypes[0])
      return $scope.resourceTypes[0];
    }()

    var epochTime = moment('Jan 1, 1970');

    $scope.hostSortOrders = [{
        name: 'Status',
        by: 'status'
      },
      {
        name: 'Uptime',
        by: 'uptime'
      },
      {
        name: 'Create Time',
        by: 'creation_time',
        reverse: false
      },
      {
        name: 'Distro',
        by: 'distro'
      }
    ];

    $scope.hostSortBy = $scope.hostSortOrders[0];

    $scope.setHostSortBy = function (order) {
      $scope.hostSortBy = order;
    };

    $scope.volumeSortOrders = [{
      name: 'Status',
      by: 'status'
    },
    {
      name: 'Expires In',
      by: 'expiration',
      reverse: false
    }
    ];

    $scope.volumeSortBy = $scope.volumeSortOrders[0];

    $scope.setVolumeSortBy = function (order) {
      $scope.volumeSortBy = order;
    };

    $scope.setResourceType = function (rtype) {
      $location.search("resourcetype", rtype);
      var id = null;
      if (rtype == "hosts" && $scope.curHostData) {
        id = $scope.curHostData.id;
      }
      if (rtype == "volumes" && $scope.curVolumeData) {
        id = $scope.curVolumeData.volume_id;
      }
      $location.search("id", id);

      $scope.currentResourceType = rtype;
    }

    $scope.isHomeVolume = function(volume) {
      for (var i = 0; i < $scope.hosts.length; i++) {
        if ($scope.hosts[i].home_volume_id === volume.volume_id) {
          return true;
        }
      }
      return false;
    };

    // Spawn REST API calls
    $scope.fetchSpawnedHosts = function () {
      mciSpawnRestService.getSpawnedHosts(
        'hosts', {}, {
          success: function (resp) {
            var hosts = resp.data;
            _.each(hosts, function (host) {
              $scope.computeUptime(host);
              $scope.computeHostExpirationTimes(host);
              host.selectedInstanceType = host.instance_type;
              if (host.display_name == "") {
                host.display_name = host.id;
              }
              host.originalDisplayName = host.display_name;
              if ($location.search()["id"] == host.id) {
                $scope.setSelectedHost(host)
              }
            });
            $scope.hosts = hosts
          },
          error: function (resp) {
            // Avoid errors when leaving the page because of a background refresh
            if ($scope.hosts == null && !$scope.errorFetchingHosts) {
              notificationService.pushNotification('Error fetching spawned hosts: ' + resp.data, 'errorHeader');
              $scope.errorFetchingHosts = true;
            }
          }
        }
      );
    };

    $scope.updateSpawnedHosts = function () {
      mciSpawnRestService.getSpawnedHosts(
        'hosts', {}, {
          success: function (resp) {
            var hosts = resp.data;
            _.each(hosts, function (host) {
              for (var i = 0; i < $scope.hosts.length; i++) {
                if ($scope.hosts[i].id !== host.id && $scope.hosts[i].id !== host.tag) {
                  continue;
                }
                $scope.computeUptime(host);
                $scope.computeHostExpirationTimes(host);
                $scope.hosts[i].uptime = host.uptime;
                $scope.hosts[i].expires_in = host.expires_in;
                $scope.hosts[i].status = host.status;
                $scope.hosts[i].id = host.id;
                $scope.hosts[i].host = host.host;
                $scope.hosts[i].start_time = host.start_time;
                if ($scope.hosts[i].home_volume_id !== host.home_volume_id) {
                    $scope.addNewVolumes();
                    $scope.hosts[i].home_volume_id = host.home_volume_id;
                }
                if ($scope.hosts[i].instance_type === undefined || $scope.hosts[i].instance_type === "") {
                  $scope.hosts[i].instance_type = host.instance_type;
                }
                if ($scope.hosts[i].selectedInstanceType === undefined || $scope.hosts[i].selectedInstanceType === "") {
                  $scope.hosts[i].selectedInstanceType = host.instance_type;
                }
              }
            });
          }
        }
      )
    }

    $scope.fetchVolumes = function () {
      mciSpawnRestService.getVolumes(
        {
          success: function (resp) {
            var volumes = resp.data;
            _.each(volumes, function (volume) {
              $scope.initialVolumeSetup(volume);
              if ($location.search()["id"] === volume.volume_id) {
                $scope.setSelectedVolume(volume)
              }
            });
            $scope.volumes = volumes;
            $scope.homeVolumeSize = Math.min($scope.volumeSizeDefault, $scope.availableVolumeSize());
            $scope.createVolumeInfo.size = $scope.homeVolumeSize;
          },
          error: function (resp) {
            // Avoid errors when leaving the page because of a background refresh
            if ($scope.volumes == null && !$scope.errorFetchingVolumes) {
              notificationService.pushNotification('Error fetching volumes: ' + resp.data, 'errorHeader');
              $scope.errorFetchingVolumes = true;
            }
          }
        }
      );
    };

    $scope.addNewVolumes = function () {
        mciSpawnRestService.getVolumes(
            {
                success: function (resp) {
                    var volumes = resp.data;
                    _.each(volumes, function (volume) {
                        var volumeExists = $scope.volumes.some(v => v.id === volume.id);
                        if (!volumeExists) {
                            $scope.initialVolumeSetup(volume);
                            $scope.volumes.push(volume);
                        }
                    });
                },
            },
        )
    }

    $scope.computeHostExpirationTimes = function (host) {
      if (!host.isTerminated && new Date(host.expiration_time) > new Date("0001-01-01T00:00:00Z")) {
        if (host.no_expiration) {
          host.expires_in = "never";
          host.original_expiration = null;
          host.current_expiration = null;
          host.modified_expiration = new Date();
        } else {
          var expiretime = moment().diff(host.expiration_time, 'seconds');
          host.expires_in = moment.duration(expiretime, 'seconds').humanize();

          host.original_expiration = new Date(host.expiration_time);
          host.current_expiration = new Date(host.expiration_time);
          host.modified_expiration = new Date(host.expiration_time);
        }
      }
    }

    $scope.computeVolumeExpirationTimes = function (volume) {
      if (volume.no_expiration) {
        volume.expires_in = "never";
      } else {
        var expiretime = moment().diff(volume.expiration, 'seconds');
        volume.expires_in = moment.duration(expiretime, 'seconds').humanize();
      }

      volume.original_expiration = new Date(volume.expiration);
      volume.current_expiration = new Date(volume.expiration);
      volume.original_no_expiration = volume.no_expiration;
    }

    $scope.initialVolumeSetup = function (volume) {
        $scope.computeVolumeExpirationTimes(volume);
        if (volume.display_name === "") {
            volume.display_name = volume.volume_id;
        }
        volume.originalDisplayName = volume.display_name;
        if (volume.host_id) {
            volume.status = "mounted";
        } else {
            volume.status = "free";
        }
    }

    $scope.computeUptime = function (host) {
      host.isTerminated = host.status === 'terminated';
      host.isStarted = true;
      const terminateTime = moment(host.termination_time);
      const startTime = moment(host.start_time);
      // check if the host is terminated to determine uptime
      if (host.isTerminated && terminateTime > epochTime) {
        const uptime = terminateTime.diff(startTime, 'seconds');
        host.uptime = moment.duration(uptime, 'seconds').humanize();
      } else if (host.status === 'stopped' || host.start_time == "0001-01-01T00:00:00Z") {
        host.uptime = "";
        host.isStarted = false;
      } else {
        const uptime = moment().diff(startTime, 'seconds');
        host.uptime = moment.duration(uptime, 'seconds').humanize();
      }
    }

    $scope.setCurrentExpirationOnClick = function () {
      // host previously had an expiration
      if ($scope.curHostData.current_expiration != null) {
        $scope.curHostData.modified_expiration = new Date($scope.curHostData.current_expiration);
        $scope.curHostData.current_expiration = null;
      } else {
        $scope.curHostData.current_expiration = new Date($scope.curHostData.modified_expiration);
      }
    }

    // Load immediately, load again in 5 seconds to pick up any slow
    // spawns / terminates from the previous post since they are async, and
    // every 60 seconds after that to pick up changes.
    $timeout($scope.fetchSpawnedHosts, 1);

    $timeout($scope.updateSpawnedHosts, 5000);
    setInterval(function () {
      $scope.updateSpawnedHosts();
    }, 60000);

    $timeout($scope.fetchVolumes, 1);
    // Returns true if the user can spawn another host. If hosts have not been initialized it
    // assumes true.
    $scope.availableHosts = function () {
      return ($scope.hosts == null) || ($scope.hosts.length < $scope.maxHostsPerUser)
    };

    $scope.availableUnexpirableHosts = function () {
      return $scope.maxUnexpirableHostsPerUser - _.where($scope.hosts, {
        original_expiration: null
      }).length;
    }

    $scope.unexpirableHostEnabled = function () {
      return $scope.availableUnexpirableHosts() > 0 || $scope.curHostData && ($scope.curHostData.no_expiration || $scope.curHostData.original_expiration == null)
    };

    $scope.availableVolumeSize = function () {
      var totalSize = $scope.maxVolumeSizePerUser;
      return _.reduce($scope.volumes, function(availableSize, v){ return availableSize - v.size;}, totalSize);
    };

    $scope.invalidHostOptions = function() {
      if ($scope.isVirtualWorkstation && ($scope.homeVolumeID === undefined || $scope.homeVolumeID === "")) {
        return $scope.invalidVolumeSize($scope.homeVolumeSize);
      }
      return false;
    };

    $scope.invalidVolumeSize = function(size) {
        return size <= 0 || $scope.availableVolumeSize() - size < 0;
    };

    $scope.availableUnexpirableVolumes = function () {
      return $scope.maxUnexpirableVolumesPerUser - _.where($scope.volumes, {
        no_expiration: true
      }).length;
    }

    $scope.unexpirableVolumeEnabled = function () {
      return $scope.availableUnexpirableVolumes() > 0 || ($scope.curVolumeData && $scope.curVolumeData.no_expiration)
    }

    $scope.generatePassword = function () {
      $scope.curHostData.password = _.shuffle(
        SHA1(document.cookie + _.now()).slice(0, 9).concat(
          _.take(
            _.shuffle('~!@#$%^&*_-+=:,.?/'),
            3
          ).join('')
        )
      ).join('')
    }

    $scope.copyPassword = function () {
      var el = $('#password-input');
      el.focus();
      el.select();
      document.execCommand('copy');
    }

    $scope.fetchSpawnableDistros = function (selectDistro, cb) {
      mciSpawnRestService.getSpawnableDistros(
        'distros', {}, {
          success: function (resp) {
            var distros = resp.data;
            $scope.setSpawnableDistros(distros, selectDistro);
            // If there is a callback to run after the distros were fetched,
            // execute it.
            if (cb) {
              cb();
            }
          },
          error: function (resp) {
            notificationService.pushNotification('Error fetching spawnable distros: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.fetchAllowedInstanceTypes = function () {
      mciSpawnRestService.getAllowedInstanceTypes(
        'types', $scope.curHostData.id,{}, {
          success: function (resp) {
            $scope.allowedInstanceTypes = resp.data;
          },
          error: function (resp) {
            notificationService.pushNotification('Error fetching allowed instance types: ' + resp.data, 'errorHeader')
          }
        }
      )
    }

    $scope.fetchUserKeys = function () {
      mciSpawnRestService.getUserKeys(
        'keys', {}, {
          success: function (resp) {
            $scope.setUserKeys(resp.data);
          },
          error: function (resp) {
            notificationService.pushNotification('Error fetching user keys: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.setHostDisplayName = function (host) {
      mciSpawnRestService.updateHostDisplayName(host.id, host.display_name,
        {
          success: function (resp) {
            host.originalDisplayName = host.display_name;
          },
          error: function (resp) {
            notificationService.pushNotification('Error setting host display name: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.resetHostDisplayName = function (host) {
      host.display_name = host.originalDisplayName;
    };

    $scope.setVolumeDisplayName = function (volume) {
      mciSpawnRestService.updateVolume("changeVolumeDisplayName", volume.volume_id, {"new_name": volume.display_name},
        {
          success: function (resp) {
            volume.originalDisplayName = volume.display_name;
          },
          error: function (resp) {
            notificationService.pushNotification('Error setting volume display name: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.resetVolumeDisplayName = function (volume) {
      volume.display_name = volume.originalDisplayName;
    };

    $scope.getHomeVolumeDisplayName = function () {
      if (!$scope.homeVolumeID) {
        return "New Volume";
      }

      return $scope.getVolumeDisplayName($scope.homeVolumeID);
    }

    $scope.getVolumeDisplayName = function(volumeID) {
      volume = $scope.volumes.find(volume => {return volume.volume_id == volumeID})
      if (volume) {
        return $scope.concatName(volume.volume_id, volume.display_name);
      }

      return volumeID;
    }

    $scope.getHostDisplayName = function(hostID) {
      host = $scope.hosts.find(host => {return host.id == hostID})
      if (host) {
        return $scope.concatName(host.id, host.display_name);
      }

      return hostID;
    }

    $scope.concatName = function(id, displayName) {
      var name = id;
      if (displayName && displayName != id) {
        name += " (" + displayName + ")";
      }

      return name;
    };

    $scope.setHomeVolume = function (volume) {
      if (volume) {
        $scope.homeVolumeID = volume.volume_id;
      } else {
        $scope.homeVolumeID = "";
      }
    }

    $scope.setAttachToHost = function (host) {
      $scope.volumeAttachHost = host;
    }

    $scope.spawnHost = function () {
      $scope.spawnReqSent = true;
      $scope.spawnInfo.spawnKey = $scope.selectedKey;
      $scope.spawnInfo.saveKey = $scope.saveKey;
      $scope.spawnInfo.userData = $scope.userdata;
      $scope.spawnInfo.use_project_setup_script = $scope.use_project_setup_script;
      if (!$scope.use_project_setup_script) { // don't set both scripts
          $scope.spawnInfo.setup_script = $scope.setup_script;
      }
      $scope.spawnInfo.is_virtual_workstation = $scope.isVirtualWorkstation;
      if ($scope.isVirtualWorkstation) {
          $scope.spawnInfo.no_expiration = $scope.noExpiration;
          $scope.spawnInfo.home_volume_size = $scope.homeVolumeSize;
          $scope.spawnInfo.home_volume_id = $scope.homeVolumeID;
      }
      $scope.spawnInfo.useTaskConfig = $scope.useTaskConfig;
      $scope.spawnInfo.region = $scope.selectedRegion;
      if ($scope.spawnTaskChecked && !!$scope.spawnTask) {
        $scope.spawnInfo.task_id = $scope.spawnTask.id;
      }
      mciSpawnRestService.spawnHost(
        $scope.spawnInfo, {}, {
          success: function (resp) {
            // we don't use reload here because we need to clear query parameters
            // in the case of spawning a host from a task
            $window.location.href = "/spawn";
          },
          error: function (resp) {
            $scope.spawnReqSent = false;
            notificationService.pushNotification('Error spawning host: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.createVolume = function () {
      $scope.volumeReqSent = true;
      mciSpawnRestService.createVolume(
        $scope.createVolumeInfo, {}, {
          success: function (resp) {
            $window.location.reload();
          },
          error: function (resp) {
              $scope.volumeReqSent = false;
            notificationService.pushNotification('Error creating volume: ' + resp.data.error, 'errorHeader');
          }
        }
      );
    };

    $scope.updateRDPPassword = function () {
      mciSpawnRestService.updateRDPPassword(
        'updateRDPPassword',
        $scope.curHostData.id,
        $scope.curHostData.password, {}, {
          success: function (resp) {
            $window.location.reload();
          },
          error: function (resp) {
            notificationService.pushNotification('Error setting host RDP password: ' + resp.data.error, 'errorHeader');
          }
        }
      );
    };

    $scope.updateHost = function () {
      let promises = [];
      // update expiration if it changed
      if (!moment($scope.curHostData.original_expiration).startOf("second").isSame(moment($scope.curHostData.current_expiration).startOf("second"))) {
        let new_expiration = null;
        if (!$scope.curHostData.no_expiration) {
          new_expiration = new Date($scope.curHostData.current_expiration);
        }

        promises.push(mciSpawnRestService.extendHostExpiration(
          'extendHostExpiration',
          $scope.curHostData.id, new_expiration, {}, {
            error: function (resp) {
              notificationService.pushNotification('Error extending host expiration: ' + resp.data.error, 'errorHeader');
            }
          }
        ));
      }

      // update tags if they changed
      if (($scope.curHostData.tags_to_add && Object.entries($scope.curHostData.tags_to_add).length > 0) || ($scope.curHostData.tags_to_delete && $scope.curHostData.tags_to_delete.length > 0)) {
        let tags_to_add = [];
        for (key in $scope.curHostData.tags_to_add) {
          let new_tag = key + "=" + $scope.curHostData.tags_to_add[key];
          tags_to_add.push(new_tag);
        }
        promises.push(mciSpawnRestService.updateHostTags(
          'updateHostTags',
          $scope.curHostData.id, tags_to_add, $scope.curHostData.tags_to_delete, {}, {
            error: function (resp) {
              notificationService.pushNotification('Error updating host tags: ' + resp.data, 'errorHeader');
            }
          }
        ));
      }

      // update instance type if it changed
      if ($scope.curHostData.selectedInstanceType && $scope.curHostData.instance_type !== $scope.curHostData.selectedInstanceType) {
        // Do nothing if host is not stopped
        if ($scope.curHostData.status != "stopped") {
          notificationService.pushNotification('Host must be stopped before modifying instance type', 'errorHeader');
          return
        }
        promises.push(mciSpawnRestService.updateInstanceType(
          'updateInstanceType',
          $scope.curHostData.id, $scope.curHostData.selectedInstanceType, {}, {
            error: function (resp) {
              notificationService.pushNotification('Error setting new instance type: ' + resp.data, 'errorHeader')
            }
          }
        ));
      }

      $q.all(promises).then(() => {
        $window.location.reload();
      })
    }

    $scope.updateHostStatus = function (action) {
      mciSpawnRestService.updateHostStatus(
        action,
        $scope.curHostData.id, {}, {
          success: function (resp) {
            $window.location.reload();
          },
          error: function (resp) {
            notificationService.pushNotification('Error changing host status: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.updateVolume = function (action) {
      data = {};
      if (action === "attachVolume" && $scope.volumeAttachHost) {
        data.host_id = $scope.volumeAttachHost.id;
      }
      if (action === "extendVolumeExpiration") {
        if ($scope.curVolumeData.no_expiration != $scope.curVolumeData.original_no_expiration) {
          action = $scope.curVolumeData.no_expiration? "setVolumeNoExpiration" : "setVolumeHasExpiration";
        } else if (!$scope.curVolumeData.no_expiration && $scope.curVolumeData.current_expiration != $scope.curVolumeData.original_expiration) {
          data.expiration = $scope.curVolumeData.current_expiration;
        }
      }
    
      mciSpawnRestService.updateVolume(
        action,
        $scope.curVolumeData.volume_id, data, {
          success: function (resp) {
            $window.location.reload();
          },
          error: function (resp) {
            notificationService.pushNotification('Error editing volume: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

    $scope.updateVolumeExpirationEnabled = function() {
      if (!$scope.curVolumeData) {
        return false;
      }

      if ($scope.curVolumeData.no_expiration != $scope.curVolumeData.original_no_expiration) {
        return true;
      }
      if (!$scope.curVolumeData.no_expiration && moment($scope.curVolumeData.original_expiration).startOf("second").isBefore(moment($scope.curVolumeData.current_expiration).startOf("second"))) {
        return true;
      }

      return false;
    }

    // API helper methods
    $scope.setSpawnableDistros = function (distros, selectDistroId) {
      if (distros.length == 0) {
        distros = [];
      }
      distros.forEach(function (spawnableDistro) {
        $scope.spawnableDistros.push({
          'distro': spawnableDistro
        });
      });
      if (distros.length > 0) {
        $scope.spawnableDistros.sort(function (a, b) {
          if (a.distro.name < b.distro.name) return -1;
          if (a.distro.name > b.distro.name) return 1;
          return 0;
        });
        $scope.setSpawnableDistro($scope.spawnableDistros[0].distro);
        $scope.spawnInfo.spawnKey = $scope.newKey;
        if (selectDistroId) {
          var selectedIndex = _.findIndex($scope.spawnableDistros,
            function (x) {
              return x.distro.name == selectDistroId
            }
          )
          if (selectedIndex >= 0) {
            $scope.setSpawnableDistro($scope.spawnableDistros[selectedIndex].distro);
          }
        }
      };
    };

    $scope.setUserKeys = function (publicKeys) {
      if (publicKeys == 'null') {
        publicKeys = [];
      }
      $scope.userKeys = [];
      _.each(publicKeys, function (publicKey) {
        $scope.userKeys.push({
          'name': publicKey.name,
          'key': publicKey.key
        });
      });
      if (publicKeys.length > 0) {
        $scope.userKeys.sort(function (a, b) {
          if (a.name < b.name) return -1;
          if (a.name > b.name) return 1;
          return 0;
        });
        $scope.updateSelectedKey($scope.userKeys[0]);
      } else {
        $scope.updateSelectedKey($scope.newKey);
      }
      // disable key name text field by default
      $('#input-key-name').attr('disabled', true);
    };


    // User Interface helper functions
    // set the spawn request distro based on user selection
    $scope.setSpawnableDistro = function (spawnableDistro) {
      $scope.selectedDistro = spawnableDistro;
      $scope.spawnInfo.distroId = spawnableDistro.name;
      $scope.selectedRegion = "us-east-1";
      // if multiple regions, preference the user region
      if ($scope.selectedDistro.regions.length > 1) {
        if ($scope.defaultRegion !== "") {
          for (let i=0; i < $scope.selectedDistro.regions.length; i++) {
            // valid region
            if ($scope.defaultRegion === $scope.selectedDistro.regions[i]) {
              $scope.selectedRegion = $scope.defaultRegion;
              break;
            }
          }
        }
      }
      // if preferred region not configured for this distro, default to the first region in this list
      if ($scope.selectedRegion === "" && $scope.selectedDistro.regions.length === 1) {
        $scope.selectedRegion = $scope.selectedDistro.regions[0];
      }

      // clear home volume settings when switching between distros
      $scope.isVirtualWorkstation = $scope.selectedDistro.virtual_workstation_allowed && !$scope.spawnTaskChecked;
      $scope.noExpiration = $scope.selectedDistro.virtual_workstation_allowed && ($scope.availableUnexpirableHosts() > 0);
      $scope.homeVolumeSize = Math.min($scope.volumeSizeDefault, $scope.availableVolumeSize());
    };

    $scope.setRegion = function(region) {
      $scope.selectedRegion = region;
    }

    // set the spawn host update instance type based on user selection
    $scope.setInstanceType = function (instanceType) {
      $scope.curHostData.selectedInstanceType = instanceType
    }

    // toggle spawn key based on user selection
    $scope.updateSelectedKey = function (selectedKey) {
      $scope.selectedKey.name = selectedKey.name;
      $scope.selectedKey.key = selectedKey.key;
      $scope.currKeyName = $scope.selectedKey.name;
    };

    // indicates if user wishes to save this key
    $scope.toggleSaveKey = function () {
      $scope.saveKey = !$scope.saveKey;
    }

    $scope.getSpawnStatusLabel = function (host) {
      if (host) {
        switch (host.status) {
          case 'running':
            return 'label success';
            break;
          case 'initializing':
          case 'provisioning':
          case 'building':
          case 'starting':
          case 'stopping':
          case 'stopped':
            return 'label block-status-started';
            break;
          case 'decommissioned':
          case 'unreachable':
          case 'quarantined':
          case 'provision failed':
            return 'block-status-cancelled';
            break;
          case 'terminated':
            return 'block-status-failed';
            break;
          default:
            return '';
        }
      }
    }

    $scope.getVolumeStatusLabel = function (volume) {
      if (volume && !volume.host_id) {
        return 'label block-status-inactive'
      }
      if (volume && volume.host_id) {
        return 'label success'
      }
      return ''
    }

    // populate for selected host; highlight selected host
    $scope.setSelectedHost = function (host) {
      if ($scope.lastSelectedHost) {
        $scope.lastSelectedHost.selected = '';
      }
      host.selected = 'active-host';
      host.password = '';
      $scope.lastSelectedHost = host;
      $location.search("id", host.id);
      $scope.curHostData = host;
      // check if this is a windows host
      $scope.curHostData.isWinHost = false;
      // XXX: if this is-windows check is updated, make sure to also update
      // model/distro/distro.go as well
      if (host.distro.arch.indexOf('win') != -1) {
        $scope.curHostData.isWinHost = true;
      }
      $scope.getTags();
      $scope.fetchAllowedInstanceTypes();
    };

    // populate for selected volume; highlight selected volume
    $scope.setSelectedVolume = function (volume) {
      if ($scope.lastSelectedVolume) {
        $scope.lastSelectedVolume.selected = '';
      }
      volume.selected = 'active-host';
      $scope.lastSelectedVolume = volume;
      $location.search("id", volume.volume_id);

      $scope.curVolumeData = volume;
    };

    $scope.invalidDelete = function (data) {
      if (data && data.no_expiration && data.checkDelete !== "delete") {
        return true;
      };
      return false;
    };

    $scope.getTags = function () {
      $scope.curHostData.tags_to_delete = [];
      $scope.curHostData.tags_to_add = {};
      $scope.curHostData.userTags = {};
      if ($scope.curHostData.instance_tags === undefined) {
        return;
      }
      for (var i = 0; i < $scope.curHostData.instance_tags.length; i++) {
        let curTag = $scope.curHostData.instance_tags[i];
        if (curTag.can_be_modified) {
          $scope.curHostData.userTags[curTag.key] = curTag.value;
        }
      }
    }

    $scope.removeTag = function (key) {
      delete $scope.curHostData.userTags[key];
      // only add to delete list if key isn't dirty
      if ($scope.curHostData.instance_tags !== undefined) {
        for (var i = 0; i < $scope.curHostData.instance_tags.length; i++) {
          if ($scope.curHostData.instance_tags[i].key === key) {
            $scope.curHostData.tags_to_delete.push(key);
            return;
          }
        }
      }
    }

    $scope.addTag = function () {
      if ($scope.new_tag.key && $scope.new_tag.value) {
        $scope.curHostData.tags_to_add[$scope.new_tag.key] = $scope.new_tag.value;
        $scope.curHostData.userTags[$scope.new_tag.key] = $scope.new_tag.value;
        $scope.new_tag = {};
      }
    }

    $scope.validTag = function (key, value) {
      if (!key) {
        return false;
      }

      if (!value) {
        return false;
      }

      return true;
    }

    $scope.goToVolume = function (volume_id) {
      _.each($scope.volumes, function (volume) {
        if (volume.volume_id == volume_id) {
          $scope.setSelectedVolume(volume);
          $scope.setResourceType("volumes");
        }
      });
    }

    $scope.goToHost = function (host_id) {
      _.each($scope.hosts, function (host) {
        if (host.id == host_id) {
          $scope.setSelectedHost(host);
          $scope.setResourceType("hosts");
        }
      });
    }

    $scope.goToPage = function (resource_type) {
      $scope.setResourceType(resource_type);
      if ($scope.modalOpen) {
        $('#spawn-modal').modal('hide');
      }
    }

    initializeModal = function (modal, title, action) {
      $scope.modalTitle = title;
      modal.on('shown.bs.modal', function () {
        $scope.modalOpen = true;
      });

      modal.on('hide.bs.modal', function () {
        $scope.modalOpen = false;
      });
    }

    attachEnterHandler = function (action) {
      $(document).keyup(function (ev) {
        if ($scope.modalOpen && ev.keyCode === 13) {
          if ($scope.currentResourceType == "hosts") {
            $scope.updateHostStatus(action);
          } else {
            $scope.updateVolume(action);
          }

          $('#spawn-modal').modal('hide');
        }
      });
    }

    $scope.openSpawnModal = function (opt) {
      $scope.modalOption = opt;
      $scope.modalOpen = true;

      var modal = $('#spawn-modal').modal('show');
      switch (opt) {
        case 'spawnHost':
          $scope.fetchUserKeys();
          if ($scope.spawnableDistros.length == 0) {
            $scope.fetchSpawnableDistros();
          }
          initializeModal(modal, 'Spawn Host');
          break;
        case 'terminateHost':
          $scope.curHostData.checkDelete = "";
          initializeModal(modal, 'Terminate Host');
          break;
        case 'stopHost':
          initializeModal(modal, 'Stop Host');
          attachEnterHandler('stop');
          break;
        case 'startHost':
          initializeModal(modal, 'Start Host');
          attachEnterHandler('start');
          break;
        case 'updateRDPPassword':
          initializeModal(modal, 'Set RDP Password')
          modal.on('shown.bs.modal', function () {
            $('#password-input').focus();
          })
          break;
      }
    };

    $scope.openVolumeModal = function (opt) {
      $scope.modalOption = opt;
      $scope.modalOpen = true;
      var modal = $('#spawn-modal').modal('show');
      switch (opt) {
        case 'createVolume':
          initializeModal(modal, 'Create Volume');
          break;
        case 'deleteVolume':
          $scope.curVolumeData.checkDelete = "";
          initializeModal(modal, 'Delete Volume');
          attachEnterHandler('deleteVolume');
          break;
        case 'attachVolume':
          if ($scope.hosts.length > 0) {
            $scope.setAttachToHost($scope.hosts[0])
          }
          initializeModal(modal, 'Attach Volume');
          attachEnterHandler('attachVolume');
          break;
        case 'detachVolume':
          initializeModal(modal, 'Detach Volume');
          attachEnterHandler('detachVolume');
          break;
      }
    };

    if ($scope.spawnTask && $scope.spawnDistro) {
      // find the spawn distro in the spawnable distros list, if it's there,
      // pre-select it in the modal.
      $scope.spawnTaskChecked = true
      setTimeout(function () {
        $scope.fetchSpawnableDistros($scope.spawnDistro._id, function () {
          $scope.openSpawnModal('spawnHost')
        });
      }, 0)
    }
  }

]);
