mciModule.controller('SpawnedHostsCtrl', ['$scope','$window', '$timeout', 'mciSpawnRestService', 'notificationService', function($scope, $window, $timeout, mciSpawnRestService, notificationService) {
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
    $scope.userData = {};
    $scope.spawnInfo = {};
    $scope.extensionLength = {};
    $scope.curHostData;
    $scope.hostExtensionLengths = {};
    $scope.maxHostsPerUser = $window.maxHostsPerUser;
    $scope.spawnReqSent = false;

    // max of 7 days time to expiration
    $scope.maxHoursToExpiration = 24*7;
    $scope.saveKey = false;
    $scope.currKeyName = '';
    $scope.newKey = {
      'name': 'New Key...',
      'key': '',
    };

    var epochTime = moment('Jan 1, 1970');

    $scope.sortOrders = [
      { name: 'Status', by: 'status' },
      { name: 'Uptime', by: 'uptime' },
      { name: 'Create Time', by: 'creation_time', reverse: false },
      { name: 'Distro', by: 'distro' }
    ];

    $scope.sortBy = $scope.sortOrders[0];

    $scope.extensionLengths = [
      {display: "1 hour", hours: 1},
      {display: "2 hours", hours: 2},
      {display: "4 hours", hours: 4},
      {display: "6 hours", hours: 6},
      {display: "8 hours", hours: 8},
      {display: "10 hours", hours: 10},
      {display: "12 hours", hours: 12},
      {display: "1 day", hours: 24},
      {display: "2 days", hours: 24*2},
      {display: "3 days", hours: 24*3},
      {display: "4 days", hours: 24*4},
      {display: "5 days", hours: 24*5},
      {display: "6 days", hours: 24*6},
      {display: "7 days", hours: 24*7},
    ];

    $scope.setSortBy = function(order) {
      $scope.sortBy = order;
    };

    // Spawn REST API calls
    $scope.fetchSpawnedHosts = function() {
      mciSpawnRestService.getSpawnedHosts(
        'hosts', {}, {
          success: function(hosts, status) {
            _.each(hosts, function(host) {
              host.isTerminated = host.status == 'terminated';
              var terminateTime = moment(host.termination_time);
              // check if the host is terminated to determine uptime
              if (terminateTime > epochTime) {
                var uptime = terminateTime.diff(host.creation_time, 'seconds');
                host.uptime = moment.duration(uptime, 'seconds').humanize();
              } else {
                var uptime = moment().diff(host.creation_time, 'seconds');
                host.uptime = moment.duration(uptime, 'seconds').humanize();
                var expiretime = moment().diff(host.expiration_time, 'seconds');
                if(+new Date(host.expiration_time) > +new Date("0001-01-01T00:00:00Z")){
                  host.expires_in = moment.duration(expiretime, 'seconds').humanize();
                }
              }
              if ($scope.lastSelected && $scope.lastSelected.id == host.id) {
                $scope.setSelected(host);
              }
           });
            $scope.hosts = hosts
          },
          error: function(jqXHR, status, errorThrown) {
            // Avoid errors when leaving the page because of a background refresh
            if ($scope.hosts == null && !$scope.errorFetchingHosts) {
                notificationService.pushNotification('Error fetching spawned hosts: ' + jqXHR.error, 'errorHeader');
                $scope.errorFetchingHosts = true;
            }
          }
        }
      );
    }

    // Load immediately, load again in 5 seconds to pick up any slow
    // spawns / terminates from the pervious post since they are async, and
    // every 60 seconds after that to pick up changes.
    $timeout($scope.fetchSpawnedHosts, 1);
    $timeout($scope.fetchSpawnedHosts, 5000);
    setInterval(function(){$scope.fetchSpawnedHosts();}, 60000);

    // Returns true if the user can spawn another host. If hosts has not been initialized it
    // assumes true.
    $scope.availableHosts = function() {
      return ($scope.hosts == null) || ($scope.hosts.length < $scope.maxHostsPerUser)
    }

    $scope.fetchSpawnableDistros = function(selectDistro, cb) {
      mciSpawnRestService.getSpawnableDistros(
        'distros', {}, {
          success: function(distros, status) {
            $scope.setSpawnableDistros(distros, selectDistro);
            // If there is a callback to run after the distros were fetched,
            // execute it.
            if(cb){
              cb();
            }
          },
          error: function(jqXHR, status, errorThrown) {
            notificationService.pushNotification('Error fetching spawnable distros: ' + jqXHR.error,'errorHeader');
          }
        }
      );
    };

    $scope.fetchUserKeys = function() {
      mciSpawnRestService.getUserKeys(
        'keys', {}, {
          success: function(keys, status) {
            $scope.setUserKeys(keys);
          },
          error: function(jqXHR, status, errorThrown) {
            notificationService.pushNotification('Error fetching user keys: ' + jqXHR.error,'errorHeader');
          }
        }
      );
    };

    $scope.spawnHost = function() {
      $scope.spawnReqSent = true;
      $scope.spawnInfo.spawnKey = $scope.selectedKey;
      $scope.spawnInfo.saveKey = $scope.saveKey;
      $scope.spawnInfo.userData = $scope.userData.text;
      if($scope.spawnTaskChecked && !!$scope.spawnTask){
        $scope.spawnInfo.task_id = $scope.spawnTask.id;
      }
      mciSpawnRestService.spawnHost(
        $scope.spawnInfo, {}, {
          success: function(data, status) {
            window.location.href = "/spawn";
          },
          error: function(jqXHR, status, errorThrown) {
            $scope.spawnReqSent = false;
            notificationService.pushNotification('Error spawning host: ' + jqXHR.error,'errorHeader');
          }
        }
      );
    };

    $scope.updateRDPPassword = function () {
      mciSpawnRestService.updateRDPPassword(
        'updateRDPPassword',
        $scope.curHostData.id,
        $scope.curHostData.password, {}, {
          success: function (data, status) {
            window.location.href = "/spawn";
          },
          error: function (jqXHR, status, errorThrown) {
            notificationService.pushNotification('Error setting host RDP password: ' + jqXHR.error,'errorHeader');
          }
        }
      );
    };

    $scope.updateHostExpiration = function() {
      mciSpawnRestService.extendHostExpiration(
        'extendHostExpiration',
        $scope.curHostData.id,
        $scope.extensionLength.hours.toString(), {}, {
          success: function (data, status) {
            window.location.href = "/spawn";
          },
          error: function (jqXHR, status, errorThrown) {
            notificationService.pushNotification('Error extending host expiration: ' + jqXHR.error,'errorHeader');
          }
        }
      );
    };

    $scope.terminateHost = function() {
      mciSpawnRestService.terminateHost(
        'terminate',
        $scope.curHostData.id, {}, {
          success: function(data, status) {
            window.location.href = "/spawn";
          },
          error: function(jqXHR, status, errorThrown) {
            notificationService.pushNotification('Error terminating host: ' + jqXHR.error,'errorHeader');
          }
        }
      );
    };

    // API helper methods
    $scope.setSpawnableDistros = function(distros, selectDistroId) {
      if (distros.length == 0) {
        distros = [];
      }
      distros.forEach(function(spawnableDistro) {
        $scope.spawnableDistros.push({
          'distro': spawnableDistro
        });
      });
      if (distros.length > 0) {
        $scope.spawnableDistros.sort(function(a, b) {
          if (a.distro.name < b.distro.name) return -1;
          if (a.distro.name > b.distro.name) return 1;
          return 0;
        });
        $scope.selectedDistro = $scope.spawnableDistros[0].distro;
        $scope.spawnInfo = {
          'distroId': $scope.selectedDistro.name,
          'spawnKey': $scope.newKey,
        };
        if(selectDistroId){
          var selectedIndex = _.findIndex($scope.spawnableDistros,
            function(x){return x.distro.name == selectDistroId}
          )
          if(selectedIndex>=0){
            $scope.selectedDistro = $scope.spawnableDistros[selectedIndex].distro;
            $scope.spawnInfo.distroId = $scope.selectedDistro.name;
          }

        }
      };
    };

    $scope.setExtensionLength = function(extensionLength) {
      $scope.extensionLength = extensionLength;
    };

    $scope.setUserKeys = function(publicKeys) {
      if (publicKeys == 'null') {
        publicKeys = [];
      }
      $scope.userKeys = [];
      _.each(publicKeys, function(publicKey) {
        $scope.userKeys.push({ 'name': publicKey.name, 'key': publicKey.key });
      });
      if (publicKeys.length > 0) {
        $scope.userKeys.sort(function(a, b) {
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
    $scope.setSpawnableDistro = function(spawnableDistro) {
      $scope.selectedDistro = spawnableDistro
      $scope.spawnInfo.distroId = spawnableDistro.name;
    };

    // toggle spawn key based on user selection
    $scope.updateSelectedKey = function(selectedKey) {
      $scope.selectedKey.name = selectedKey.name;
      $scope.selectedKey.key = selectedKey.key;
      $scope.currKeyName = $scope.selectedKey.name;
    };

    // indicates if user wishes to save this key
    $scope.toggleSaveKey = function() {
      $scope.saveKey = !$scope.saveKey;
    }

    $scope.getSpawnStatusLabel = function(host) {
      if (host) {
        switch (host.status) {
        case 'running':
          return 'label success';
          break;
        case 'initializing':
        case 'provisioning':
        case 'starting':
          return 'label block-status-started';
          break;
        case 'decommissioned':
        case 'unreachable':
        case 'quarantined':
        case 'provision failed':
          return 'label block-status-cancelled';
          break;
        case 'terminated':
          return 'label block-status-failed';
          break;
        default:
          return '';
        }
      }
    }

    // populate for selected host; highlight selected host
    $scope.setSelected = function(host) {
      if ($scope.lastSelected) {
        $scope.lastSelected.selected = '';
      }
      host.selected = 'active-host';
      host.password = '';
      host.cPassword = '';
      $scope.lastSelected = host;
      $scope.curHostData = host;
      $scope.curHostData.isTerminated = host.isTerminated;
      // check if this is a windows host
      $scope.curHostData.isWinHost = false;
      if (host.distro.arch.indexOf('win') != -1) {
        $scope.curHostData.isWinHost = true;
      }
      $scope.updateExtensionOptions($scope.curHostData);
    };

    $scope.updateExtensionOptions = function(host) {
      $scope.hostExtensionLengths = [];
      _.each($scope.extensionLengths, function(extensionLength) {
        var remainingTimeSec = moment(host.expiration_time).diff(moment(), 'seconds');
        var remainingTimeDur = moment.duration(remainingTimeSec, 'seconds');
        remainingTimeDur.add(extensionLength.hours, 'hours');

        // you should only be able to extend duration for a max of 7 days
        if (remainingTimeDur.as('hours') < $scope.maxHoursToExpiration) {
          $scope.hostExtensionLengths.push(extensionLength);
        }
      });
      if ($scope.hostExtensionLengths.length > 0) {
        $scope.setExtensionLength($scope.hostExtensionLengths[0]);
      }
    }

    $scope.setExtensionLength = function(extensionLength) {
      $scope.extensionLength = extensionLength;
    }

    $scope.openSpawnModal = function(opt) {
      $scope.modalOption = opt;
      $scope.modalOpen = true;

      var modal = $('#spawn-modal').modal('show');
      if ($scope.modalOption === 'spawnHost') {
        $scope.fetchUserKeys();
        if ($scope.spawnableDistros.length == 0) {
          $scope.fetchSpawnableDistros();
        }
        $scope.modalTitle = 'Spawn Host';
        modal.on('shown.bs.modal', function() {
          $scope.modalOpen = true;
          $scope.$apply();
        });

        modal.on('hide.bs.modal', function() {
          $scope.modalOpen = false;
          $scope.$apply();
        });
      } else if ($scope.modalOption === 'terminateHost') {
        $scope.modalTitle = 'Terminate Host';
        modal.on('shown.bs.modal', function() {
          $scope.modalOpen = true;
          $scope.$apply();
        });

        modal.on('hide.bs.modal', function() {
          $scope.modalOpen = false;
          $scope.$apply();
        });
      } else if ($scope.modalOption === 'updateRDPPassword') {
        $scope.modalTitle = 'Set RDP Password';
        modal.on('shown.bs.modal', function () {
          $('#password-input').focus();
          $scope.modalOpen = true;
          $scope.$apply();
        });

        modal.on('hide.bs.modal', function () {
          $scope.modalOpen = false;
          $scope.$apply();
        });
      }

    };

    $(document).keyup(function(ev) {
      if ($scope.modalOpen && ev.keyCode === 13) {
        if ($scope.modalOption === 'terminateHost') {
          $scope.terminateHost();
          $('#spawn-modal').modal('hide');
        }
      }
    });

    if($scope.spawnTask && $scope.spawnDistro){
      // find the spawn distro in the spawnable distros list, if it's there,
      // pre-select it in the modal.
      $scope.spawnTaskChecked = true
      setTimeout(function(){
        $scope.fetchSpawnableDistros($scope.spawnDistro._id, function(){
          $scope.openSpawnModal('spawnHost')
        });
      }, 0)
    }
  }

]);
