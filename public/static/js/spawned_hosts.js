mciModule.controller('SpawnedHostsCtrl', ['$scope', '$window', '$timeout', '$q', 'mciSpawnRestService', 'notificationService', function ($scope, $window, $timeout, $q, mciSpawnRestService, notificationService) {
    $scope.userTz = $window.userTz;
    $scope.hosts = null;
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
    $scope.maxHostsPerUser = $window.maxHostsPerUser;
    $scope.maxUnexpirableHostsPerUser = $window.maxUnexpirableHostsPerUser;
    $scope.spawnReqSent = false;
    $scope.useTaskConfig = false;
    $scope.allowedInstanceTypes = [];

    // max of 7 days time to expiration
    $scope.maxHoursToExpiration = 24 * 7;
    $scope.saveKey = false;
    $scope.currKeyName = '';
    $scope.newKey = {
      'name': '',
      'key': '',
    };

    var epochTime = moment('Jan 1, 1970');

    $scope.sortOrders = [{
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

    $scope.sortBy = $scope.sortOrders[0];

    $scope.setSortBy = function (order) {
      $scope.sortBy = order;
    };

    // Spawn REST API calls
    $scope.fetchSpawnedHosts = function () {
      mciSpawnRestService.getSpawnedHosts(
        'hosts', {}, {
          success: function (resp) {
            var hosts = resp.data;
            _.each(hosts, function (host) {
              $scope.computeUptime(host);
              $scope.computeExpirationTimes(host);

              host.selectedInstanceType = host.instance_type;
              if ($scope.lastSelected && $scope.lastSelected.id == host.id) {
                $scope.setSelected(host);
              }
            });
            $scope.hosts = hosts
          },
          error: function (resp) {
            // Avoid errors when leaving the page because of a background refresh
            if ($scope.hosts == null && !$scope.errorFetchingHosts) {
              notificationService.pushNotification('Error fetching spawned hosts: ' + resp.data.error, 'errorHeader');
              $scope.errorFetchingHosts = true;
            }
          }
        }
      );
    }

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
                $scope.computeExpirationTimes(host);
                $scope.hosts[i].uptime = host.uptime;
                $scope.hosts[i].expires_in = host.expires_in;
                $scope.hosts[i].status = host.status;
                $scope.hosts[i].id = host.id;
                $scope.hosts[i].host = host.host;
                $scope.hosts[i].start_time = host.start_time;
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

    $scope.computeExpirationTimes = function (host) {
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

    $scope.computeUptime = function (host) {
      host.isTerminated = host.status == 'terminated';
      const terminateTime = moment(host.termination_time);
      const startTime = moment(host.start_time);
      // check if the host is terminated to determine uptime
      if (host.isTerminated && terminateTime > epochTime) {
        const uptime = terminateTime.diff(startTime, 'seconds');
        host.uptime = moment.duration(uptime, 'seconds').humanize();
      } else if (host.status == 'stopped' || host.start_time == "0001-01-01T00:00:00Z") {
        host.uptime = "";
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

    // Returns true if the user can spawn another host. If hosts have not been initialized it
    // assumes true.
    $scope.availableHosts = function () {
      return ($scope.hosts == null) || ($scope.hosts.length < $scope.maxHostsPerUser)
    }

    $scope.availableUnexpirableHosts = function () {
      var available = $scope.maxUnexpirableHostsPerUser;
      for (host of $scope.hosts) {
        if (host.original_expiration == null) {
          available--
        }
      }
      return available
    }

    $scope.unexpirableEnabled = function () {
      return $scope.availableUnexpirableHosts() > 0 || $scope.curHostData.no_expiration || $scope.curHostData.original_expiration == null
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
            notificationService.pushNotification('Error fetching spawnable distros: ' + resp.data.error, 'errorHeader');
          }
        }
      );
    };

    $scope.fetchAllowedInstanceTypes = function () {
      mciSpawnRestService.getAllowedInstanceTypes(
        'types', $scope.curHostData.host_type, {}, {
          success: function (resp) {
            $scope.allowedInstanceTypes = resp.data;
          },
          error: function (resp) {
            notificationService.pushNotification('Error fetching allowed instance types: ' + resp.data.error, 'errorHeader')
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
            notificationService.pushNotification('Error fetching user keys: ' + resp.data.error, 'errorHeader');
          }
        }
      );
    };

    $scope.spawnHost = function () {
      $scope.spawnReqSent = true;
      $scope.spawnInfo.spawnKey = $scope.selectedKey;
      $scope.spawnInfo.saveKey = $scope.saveKey;
      $scope.spawnInfo.userData = $scope.userdata;
      $scope.spawnInfo.useTaskConfig = $scope.useTaskConfig;
      if ($scope.spawnTaskChecked && !!$scope.spawnTask) {
        $scope.spawnInfo.task_id = $scope.spawnTask.id;
      }
      mciSpawnRestService.spawnHost(
        $scope.spawnInfo, {}, {
          success: function (resp) {
            window.location.href = "/spawn";
          },
          error: function (resp) {
            $scope.spawnReqSent = false;
            notificationService.pushNotification('Error spawning host: ' + resp.data.error, 'errorHeader');
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
            window.location.href = "/spawn";
          },
          error: function (resp) {
            notificationService.pushNotification('Error setting host RDP password: ' + resp.data.error, 'errorHeader');
          }
        }
      );
    };

    $scope.update = function () {
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
        window.location.href = "/spawn";
      })
    }

    $scope.updateHostStatus = function (action) {
      mciSpawnRestService.updateHostStatus(
        action,
        $scope.curHostData.id, {}, {
          success: function (resp) {
            window.location.href = "/spawn";
          },
          error: function (resp) {
            notificationService.pushNotification('Error changing host status: ' + resp.data, 'errorHeader');
          }
        }
      );
    };

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
        $scope.selectedDistro = $scope.spawnableDistros[0].distro;
        $scope.spawnInfo = {
          'distroId': $scope.selectedDistro.name,
          'spawnKey': $scope.newKey,
        };
        if (selectDistroId) {
          var selectedIndex = _.findIndex($scope.spawnableDistros,
            function (x) {
              return x.distro.name == selectDistroId
            }
          )
          if (selectedIndex >= 0) {
            $scope.selectedDistro = $scope.spawnableDistros[selectedIndex].distro;
            $scope.spawnInfo.distroId = $scope.selectedDistro.name;
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
      $scope.selectedDistro = spawnableDistro
      $scope.spawnInfo.distroId = spawnableDistro.name;
    };

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

    // populate for selected host; highlight selected host
    $scope.setSelected = function (host) {
      if ($scope.lastSelected) {
        $scope.lastSelected.selected = '';
      }
      host.selected = 'active-host';
      host.password = '';
      $scope.lastSelected = host;
      $scope.curHostData = host;
      $scope.curHostData.isTerminated = host.isTerminated;
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
          $scope.updateHostStatus(action);
          $('#spawn-modal').modal('hide');
        }
      });
    }

    $scope.openSpawnModal = function (opt) {
      $scope.modalOption = opt;
      $scope.modalOpen = true;

      var modal = $('#spawn-modal').modal('show');
      switch ($scope.modalOption) {
        case 'spawnHost':
          $scope.fetchUserKeys();
          if ($scope.spawnableDistros.length == 0) {
            $scope.fetchSpawnableDistros();
          }
          initializeModal(modal, 'Spawn Host');
          break;
        case 'terminateHost':
          initializeModal(modal, 'Terminate Host');
          attachEnterHandler('terminate');
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