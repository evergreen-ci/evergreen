mciModule.controller('AdminSettingsController', ['$scope', '$window', '$http', 'mciAdminRestService', 'notificationService', '$mdpTimePicker', function ($scope, $window, $http, mciAdminRestService, notificationService) {
  $scope.can_clear_tokens = $window.can_clear_tokens;
  $scope.show_overrides_section = $window.show_overrides_section;
  $scope.validDefaultHostAllocatorRoundingRules = $window.validDefaultHostAllocatorRoundingRules;
  $scope.validDefaultHostAllocatorFeedbackRules = $window.validDefaultHostAllocatorFeedbackRules;
  $scope.validDefaultHostsOverallocatedRules = $window.validDefaultHostsOverallocatedRules;

  $scope.load = function () {
    $scope.Settings = {};

    $scope.getSettings();
    $scope.disableRestart = false;
    $scope.disableSubmit = false;
    $scope.restartRed = true;
    $scope.restartPurple = true;
    $scope.restartLavender = true;
    $scope.ValidThemes = ["ANNOUNCEMENT", "INFORMATION", "WARNING", "IMPORTANT"];
    $scope.validAuthKinds = ["okta", "naive", "only_api", "allow_service_users", "github"];
    $scope.validECSOSes = ["linux", "windows"];
    $scope.validECSArches = ["amd64", "arm64"];
    $scope.validECSWindowsVersions = {
      "Server 2016": "windows_server_2016",
      "Server 2019": "windows_server_2019",
      "Server 2022": "windows_server_2022"
    };
    $("#restart-modal").on("hidden.bs.modal", $scope.enableSubmit);
  }

  $scope.getSettings = function () {
    var successHandler = function (resp) {
      if (resp.data.slack && resp.data.slack.options) {
        var fields = resp.data.slack.options.fields;
        var fieldsSet = [];
        for (var field in fields) {
          fieldsSet.push(field);
        }
        resp.data.slack.options.fields = fieldsSet;
      }

      $scope.tempCredentials = [];
      _.each(resp.data.credentials, function (val, key) {
        var obj = {};
        obj[key] = val;
        $scope.tempCredentials.push(obj);
      });

      $scope.tempExpansions = [];
      _.each(resp.data.expansions, function (val, key) {
        var obj = {};
        obj[key] = val;
        $scope.tempExpansions.push(obj);
      });

      if (!resp.data.auth) {
        resp.data.auth = {};
      }
      if (resp.data.auth.multi) {
        if (resp.data.auth.multi.read_write) {
          $scope.tempMultiAuthReadWrite = _.clone(resp.data.auth.multi.read_write);
        } else {
          $scope.tempMultiAuthReadWrite = [];
        }
        if (resp.data.auth.multi.read_only) {
          $scope.tempMultiAuthReadOnly = _.clone(resp.data.auth.multi.read_only);
        } else {
          $scope.tempMultiAuthReadOnly = [];
        }
      }

      $scope.tempPlugins = resp.data.plugins ? jsyaml.safeDump(resp.data.plugins) : ""
      $scope.tempContainerPools = resp.data.container_pools.pools ? jsyaml.safeDump(resp.data.container_pools.pools) : ""

      $scope.Settings = resp.data;
      $scope.Settings.jira_notifications = $scope.Settings.jira_notifications;
      $scope.Settings.jira_notifications.custom_fields = $scope.Settings.jira_notifications.custom_fields || {};
      $scope.Settings.providers.aws.ec2_keys = $scope.Settings.providers.aws.ec2_keys || [];
      $scope.Settings.providers.aws.allowed_regions = $scope.Settings.providers.aws.allowed_regions || [];
    }
    var errorHandler = function (resp) {
      notificationService.pushNotification("Error loading settings: " + resp.data.error, "errorHeader");
    }
    mciAdminRestService.getSettings({
      success: successHandler,
      error: errorHandler
    });
  }

  $scope.saveSettings = function () {
    var successHandler = function (resp) {
      window.location.href = "/admin";
    }
    var errorHandler = function (resp) {
      notificationService.pushNotification("Error saving settings: " + resp.data.message, "errorHeader");
    }

    if ($scope.Settings.slack && $scope.Settings.slack.options) {
      var fields = $scope.Settings.slack.options.fields;
      var fieldsSet = {};
      for (var i = 0; i < fields.length; i++) {
        fieldsSet[fields[i]] = true;
      }
      $scope.Settings.slack.options.fields = fieldsSet;
    }

    $scope.Settings.credentials = {};
    _.map($scope.tempCredentials, function (elem, index) {
      for (var key in elem) {
        $scope.Settings.credentials[key] = elem[key];
      }
    });

    if (!$scope.Settings.auth) {
      $scope.Settings.auth = {};
    }
    if (!$scope.Settings.auth.multi) {
      $scope.Settings.auth.multi = {};
    }
    if (!$scope.Settings.auth.multi.read_write) {
      $scope.Settings.auth.read_write = [];
    }
    if ($scope.tempMultiAuthReadWrite) {
      $scope.Settings.auth.multi.read_write = $scope.tempMultiAuthReadWrite;
    }
    if ($scope.tempMultiAuthReadOnly) {
      $scope.Settings.auth.multi.read_only = $scope.tempMultiAuthReadOnly;
    }

    $scope.Settings.expansions = {};
    _.map($scope.tempExpansions, function (elem, index) {
      for (var key in elem) {
        $scope.Settings.expansions[key] = elem[key];
      }
    });

    try {
      $scope.Settings.plugins = jsyaml.safeLoad($scope.tempPlugins);
    } catch (e) {
      notificationService.pushNotification("Error parsing plugin yaml: " + e, "errorHeader");
      return;
    }

    try {
      var parsedContainerPools = jsyaml.safeLoad($scope.tempContainerPools);
    } catch (e) {
      notificationService.pushNotification("Error parsing container pools yaml: " + e, "errorHeader");
      return;
    }

    if (!$scope.tempContainerPools) {
      parsedContainerPools = [];
    }

    // do not save settings if any container pool field is null
    // or if duplicate container pool IDs found
    var uniqueIds = {}
    for (var i = 0; i < parsedContainerPools.length; i++) {
      var p = parsedContainerPools[i]
      // check fields
      if (!p.distro || !p.id || !p.max_containers) {
        notificationService.pushNotification("Error saving settings: container pool field cannot be null", "errorHeader");
        return
      }

      // check uniqueness
      if (p.id in uniqueIds) {
        notificationService.pushNotification("Error saving settings: found duplicate container pool ID: " + p.id, "errorHeader");
        return;
      }
      uniqueIds[p.id] = true
    }

    $scope.Settings.container_pools.pools = parsedContainerPools;

    if ($scope.tempPlugins === null || $scope.tempPlugins === undefined || $scope.tempPlugins == "") {
      $scope.Settings.plugins = {};
    }
    if (!$scope.tempCredentials || $scope.tempCredentials.length === 0) {
      $scope.Settings.credentials = {};
    }
    if (!$scope.tempExpansions || $scope.tempExpansions.length === 0) {
      $scope.Settings.expansions = {};
    }

    mciAdminRestService.saveSettings($scope.Settings, {
      success: successHandler,
      error: errorHandler
    });
  }

  $scope.validEC2Credentials = function (item) {
    return item && item.region && item.key && item.secret;
  }

  $scope.addEC2Credential = function () {
    if ($scope.Settings.providers == null || $scope.Settings.providers == undefined) {
      $scope.Settings.providers = {
        "aws": {
          "ec2_keys": []
        }
      };
    }
    if ($scope.Settings.providers.aws.ec2_keys === undefined || $scope.Settings.providers.aws.ec2_keys === null) {
      $scope.Settings.providers.aws.ec2_keys = []
    }
    if (!$scope.validEC2Credentials($scope.new_item)) {
      $scope.invalidCredential = "EC2 Region, Key, and Secret required.";
      return
    }
    $scope.Settings.providers.aws.ec2_keys.push($scope.new_item);
    $scope.new_item = {};
    $scope.invalidCredential = "";
  }

  $scope.deleteEC2Credential = function (index) {
    $scope.Settings.providers.aws.ec2_keys.splice(index, 1);
  }

  $scope.addSubnet = function () {
    if ($scope.Settings.providers == null) {
      $scope.Settings.providers = {
        "aws": {
          "subnets": []
        }
      };
    }
    if ($scope.Settings.providers.aws.subnets == null) {
      $scope.Settings.providers.aws.subnets = []
    }

    if (!$scope.validSubnet($scope.new_subnet)) {
      $scope.invalidSubnet = "Availability zone and subnet ID are required.";
      return
    }

    $scope.Settings.providers.aws.subnets.push($scope.new_subnet);
    $scope.new_subnet = {};
    $scope.invalidSubnet = "";
  }

  $scope.addAccountRoleMapping = function () {
    if ($scope.Settings.providers == null) {
      $scope.Settings.providers = {
        "aws": {
          "account_roles": []
        }
      };
    }
    if ($scope.Settings.providers.aws == null) {
      $scope.Settings.providers.aws = {
        "account_roles": []
      }
    }
    if ($scope.Settings.providers.aws.account_roles == null) {
      $scope.Settings.providers.aws.account_roles = []
    }

    if (!$scope.validAccountRoleMapping($scope.new_account_role_mapping)) {
      $scope.invalidAccountRoleMapping = "Account and role are required.";
      return
    }

    $scope.Settings.providers.aws.account_roles.push($scope.new_account_role_mapping);
    $scope.new_account_role_mapping = {};
    $scope.invalidAccountRoleMapping = "";
  }

  $scope.addProjectToPrefixMapping = function () {
    if ($scope.Settings.buckets == null) {
      $scope.Settings.buckets = {
        "project_to_prefix_mappings": []
      };
    }
    if ($scope.Settings.buckets.project_to_prefix_mappings == null) {
      $scope.Settings.buckets.project_to_prefix_mappings = [];
    }

    if (!$scope.validProjectToPrefixMapping($scope.new_project_to_prefix_mapping)) {
      $scope.invalidProjectToPrefixMapping = "Project and prefix are required.";
      return
    }

    $scope.Settings.buckets.project_to_prefix_mappings.push($scope.new_project_to_prefix_mapping);
    $scope.new_project_to_prefix_mapping = {};
    $scope.invalidProjectToPrefixMapping = "";
  }

  $scope.addProjectToBucketMapping = function () {
    if ($scope.Settings.buckets == null) {
      $scope.Settings.buckets = {
        "project_to_bucket_mappings": []
      };
    }
    if ($scope.Settings.buckets.project_to_bucket_mappings == null) {
      $scope.Settings.buckets.project_to_bucket_mappings = [];
    }

    if (!$scope.validProjectToBucketMapping($scope.new_project_to_bucket_mapping)) {
      $scope.invalidProjectToBucketMapping = "Project and bucket are required.";
      return
    }

    $scope.Settings.buckets.project_to_bucket_mappings.push($scope.new_project_to_bucket_mapping);
    $scope.new_project_to_bucket_mapping = {};
    $scope.invalidProjectToBucketMapping = "";
  }

  $scope.addInternalBucket = function () {
    if ($scope.Settings.buckets == null) {
      $scope.Settings.buckets = {
        "internal_buckets": []
      };
    }
    if ($scope.Settings.buckets.internal_buckets == null) {
      $scope.Settings.buckets.internal_buckets = [];
    }

    if (!$scope.validInternalBucket($scope.new_internal_bucket)) {
      $scope.invalidInternalBucket = "Bucket name cannot be empty.";
      return
    }

    $scope.Settings.buckets.internal_buckets.push($scope.new_internal_bucket);
    $scope.new_internal_bucket = "";
    $scope.invalidInternalBucket = "";
  }

  $scope.deleteSubnet = function (index) {
    $scope.Settings.providers.aws.subnets.splice(index, 1);
  }

  $scope.validSubnet = function (subnet) {
    return subnet && subnet.az && subnet.subnet_id;
  }

  $scope.deleteAccountRoleMapping = function (index) {
    $scope.Settings.providers.aws.account_roles.splice(index, 1);
  }

  $scope.validAccountRoleMapping = function (mapping) {
    return mapping && mapping.account && mapping.role;
  }

  $scope.deleteProjectToPrefixMapping = function (index) {
    $scope.Settings.buckets.project_to_prefix_mappings.splice(index, 1);
  }

  $scope.validProjectToPrefixMapping = function (mapping) {
    return mapping && mapping.project_id && mapping.prefix;
  }

  $scope.deleteProjectToBucketMapping = function (index) {
    $scope.Settings.buckets.project_to_bucket_mappings.splice(index, 1);
  }

  $scope.validProjectToBucketMapping = function (mapping) {
    // We do not check mapping.prefix since it is optional.
    return mapping && mapping.project_id && mapping.bucket;
  }

  $scope.deleteInternalBucket = function (index) {
    $scope.Settings.buckets.internal_buckets.splice(index, 1);
  }

  $scope.validInternalBucket = function (bucket) {
    return bucket && bucket != "";
  }

  $scope.addAWSVPCSubnet = function () {
    if ($scope.Settings.providers == null) {
      $scope.Settings.providers = {
        "aws": {
          "pod": {
            "ecs": {
              "awsvpc": {
                "subnets": []
              }
            }
          }
        }
      };
    }
    if ($scope.Settings.providers.aws == null) {
      $scope.Settings.providers.aws = {
        "pod": {
          "ecs": {
            "awsvpc": {
              "subnets": []
            }
          }
        }
      };
    }
    if ($scope.Settings.providers.aws.pod == null) {
      $scope.Settings.providers.aws.pod = {
        "ecs": {
          "awsvpc": {
            "subnets": []
          }
        }
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs == null) {
      $scope.Settings.providers.aws.pod.ecs = {
        "awsvpc": {
          "subnets": []
        }
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs.awsvpc == null) {
      $scope.Settings.providers.aws.pod.ecs.awsvpc = {
        "subnets": []
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs.awsvpc.subnets == null) {
      $scope.Settings.providers.aws.pod.ecs.awsvpc.subnets = [];
    }

    if (!$scope.validAWSVPCSubnet($scope.new_awsvpc_subnet)) {
      $scope.invalidAWSVPCSubnet = "Subnet ID cannot be empty.";
      return
    }

    $scope.Settings.providers.aws.pod.ecs.awsvpc.subnets.push($scope.new_awsvpc_subnet);
    $scope.new_awsvpc_subnet = "";
    $scope.invalidAWSVPCSubnet = "";
  }

  $scope.validAWSVPCSubnet = function (subnet) {
    return subnet;
  }

  $scope.deleteAWSVPCSubnet = function (index) {
    $scope.Settings.providers.aws.pod.ecs.awsvpc.subnets.splice(index, 1);
  }

  $scope.addAWSVPCSecurityGroup = function () {
    if ($scope.Settings.providers == null) {
      $scope.Settings.providers = {
        "aws": {
          "pod": {
            "ecs": {
              "awsvpc": {
                "security_groups": []
              }
            }
          }
        }
      };
    }
    if ($scope.Settings.providers.aws == null) {
      $scope.Settings.providers.aws = {
        "pod": {
          "ecs": {
            "awsvpc": {
              "security_groups": []
            }
          }
        }
      };
    }
    if ($scope.Settings.providers.aws.pod == null) {
      $scope.Settings.providers.aws.pod = {
        "ecs": {
          "awsvpc": {
            "security_groups": []
          }
        }
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs == null) {
      $scope.Settings.providers.aws.pod.ecs = {
        "awsvpc": {
          "security_groups": []
        }
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs.awsvpc == null) {
      $scope.Settings.providers.aws.pod.ecs.awsvpc = {
        "security_groups": []
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs.awsvpc.security_groups == null) {
      $scope.Settings.providers.aws.pod.ecs.awsvpc.security_groups = [];
    }

    if (!$scope.validAWSVPCSecurityGroup($scope.new_awsvpc_security_group)) {
      $scope.invalidAWSVPCSecurityGroup = "Security group cannot be empty.";
      return
    }

    $scope.Settings.providers.aws.pod.ecs.awsvpc.security_groups.push($scope.new_awsvpc_security_group);
    $scope.new_awsvpc_security_group = "";
    $scope.invalidAWSVPCSecurityGroup = "";
  }

  $scope.validAWSVPCSecurityGroup = function (sg) {
    return sg;
  }

  $scope.deleteAWSVPCSecurityGroup = function (index) {
    $scope.Settings.providers.aws.pod.ecs.awsvpc.security_groups.splice(index, 1);
  }

  $scope.addECSCluster = function () {
    if ($scope.Settings.providers == null) {
      $scope.Settings.providers = {
        "aws": {
          "pod": {
            "ecs": {
              "clusters": []
            }
          }
        }
      };
    }
    if ($scope.Settings.providers.aws == null) {
      $scope.Settings.providers.aws = {
        "pod": {
          "ecs": {
            "clusters": []
          }
        }
      };
    }
    if ($scope.Settings.providers.aws.pod == null) {
      $scope.Settings.providers.aws.pod = {
        "ecs": {
          "clusters": []
        }
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs == null) {
      $scope.Settings.providers.aws.pod.ecs = {
        "clusters": []
      };
    }

    if ($scope.Settings.providers.aws.pod.ecs.clusters == null) {
      $scope.Settings.providers.aws.pod.ecs.clusters = [];
    }

    if (!$scope.validECSCluster($scope.new_ecs_cluster)) {
      $scope.invalidECSCluster = "ECS cluster name and OS are required.";
      return
    }

    $scope.Settings.providers.aws.pod.ecs.clusters.push($scope.new_ecs_cluster);
    $scope.new_ecs_cluster = {};
    $scope.invalidECSCluster = "";
  }

  $scope.validECSCluster = function (item) {
    return item && item.name && item.os;
  }

  $scope.deleteECSCluster = function (index) {
    $scope.Settings.providers.aws.pod.ecs.clusters.splice(index, 1);
  }

  $scope.addECSCapacityProvider = function () {
    if ($scope.Settings.providers == null) {
      $scope.Settings.providers = {
        "aws": {
          "pod": {
            "ecs": {
              "capacity_providers": []
            }
          }
        }
      };
    }
    if ($scope.Settings.providers.aws == null) {
      $scope.Settings.providers.aws = {
        "pod": {
          "ecs": {
            "capacity_providers": []
          }
        }
      };
    }
    if ($scope.Settings.providers.aws.pod == null) {
      $scope.Settings.providers.aws.pod = {
        "ecs": {
          "capacity_providers": []
        }
      };
    }
    if ($scope.Settings.providers.aws.pod.ecs == null) {
      $scope.Settings.providers.aws.pod.ecs = {
        "capacity_providers": []
      };
    }

    if ($scope.Settings.providers.aws.pod.ecs.capacity_providers == null) {
      $scope.Settings.providers.aws.pod.ecs.capacity_providers = [];
    }

    if (!$scope.validECSCapacityProvider($scope.new_ecs_capacity_provider)) {
      $scope.invalidECSCapacityProvider = "ECS capacity provider name, OS (and Windows version if Windows), and arch are required.";
      return
    }

    $scope.Settings.providers.aws.pod.ecs.capacity_providers.push($scope.new_ecs_capacity_provider);
    $scope.new_ecs_capacity_provider = {};
    $scope.invalidECSCapacityProvider = "";
  }

  $scope.validECSCapacityProvider = function (item) {
    if (!item || !item.name || !item.os || !item.arch) {
      return false
    }
    if (item.os == "windows" && !item.windows_version) {
      return false
    }
    return true
  }

  $scope.deleteECSCapacityProvider = function (index) {
    $scope.Settings.providers.aws.pod.ecs.capacity_providers.splice(index, 1);
  }

  $scope.addAmboyNamedQueue = function () {
    if ($scope.Settings.amboy == null) {
      $scope.Settings.amboy = {
        "named_queues": []
      };
    }
    if ($scope.Settings.amboy.named_queues == null) {
      $scope.Settings.amboy.named_queues = [];
    }

    if (!$scope.validAmboyNamedQueue($scope.new_amboy_named_queue)) {
      $scope.invalidAmboyNamedQueue = "Amboy queue name or regexp is required.";
      return
    }

    $scope.Settings.amboy.named_queues.push($scope.new_amboy_named_queue);
    $scope.new_amboy_named_queue = {};
    $scope.invalidAmboyNamedQueue = "";
  }

  $scope.validAmboyNamedQueue = function (queue) {
    return queue && (queue.name || queue.regexp);
  }

  $scope.deleteAmboyNamedQueue = function (index) {
    $scope.Settings.amboy.named_queues.splice(index, 1);
  }

  $scope.clearAllUserTokens = function () {
    if (!confirm("This will log out all users from all existing sessions. Continue?"))
      return

    $http.post('/admin/cleartokens').then(
      function (resp) {
        window.location.reload();
      },
      function (resp) {
        notificationService.pushNotification("Failed to clear user tokens: " + resp.data.error, 'errorHeader');
      });
  }

  $scope.clearCommitQueues = function () {
    if (!confirm("This will clear the contents of all commit queues. Continue?")) {
      return
    }

    var successHandler = function (resp) {
      notificationService.pushNotification("Operation successful: cleared " + resp.data.cleared_count + " queues", 'notifyHeader', 'success');
    };
    var errorHandler = function (resp) {
      notificationService.pushNotification("Failed to clear commit queues: " + resp.data.error, 'errorHeader');
    };

    mciAdminRestService.clearCommitQueues({
      success: successHandler,
      error: errorHandler
    });
  }

  timestamp = function (ts) {
    return "[" + moment(ts, "YYYY-MM-DDTHH:mm:ss").format("lll") + "] ";
  }

  $scope.restartItems = function (dryRun) {
    if (!$scope.fromDate || !$scope.toDate || !$scope.toTime || !$scope.fromTime) {
      alert("The from/to date and time must be populated to restart tasks");
      return;
    }
    var from = combineDateTime($scope.fromDate, $scope.fromTime);
    var to = combineDateTime($scope.toDate, $scope.toTime);
    if (to < from) {
      alert("From time cannot be after to time");
      $scope.disableSubmit = false;
      return;
    }

    if (!$scope.restartRed && !$scope.restartPurple && !$scope.restartLavender) {
      alert("No tasks selected to restart");
      return;
    }
    if (dryRun === false) {
      $scope.disableRestart = true;
      var successHandler = function (resp) {
        $("#divMsg").text("The below tasks have been queued to restart. Feel free to close this popup or inspect the tasks listed.");
        $scope.disableSubmit = false;
      }
    } else {
      $scope.disableSubmit = true;
      $scope.disableRestart = false;
      $("#divMsg").text("");
      dryRun = true;
      var successHandler = function (resp) {
        $scope.items = resp.data.items_restarted;
        $scope.modalTitle = "Restart Tasks";
        $("#restart-modal").modal("show");
      }
    }
    var errorHandler = function (resp) {
      notificationService.pushNotification("Error restarting tasks: " + resp.data.error, "errorHeader");
    }

    mciAdminRestService.restartItems(from, to, dryRun, $scope.restartRed, $scope.restartPurple, $scope.restartLavender, {
      success: successHandler,
      error: errorHandler
    });
  }

  combineDateTime = function (date, time) {
    date.setHours(time.getHours());
    date.setMinutes(time.getMinutes());

    return date;
  }

  $scope.enableSubmit = function () {
    $scope.disableSubmit = false;
    $scope.$apply();
  }

  $scope.jumpToItem = function (itemId) {
    window.open(/task/ + itemId);
  }


  $scope.scrollTo = function (section) {
    var offset = $('#' + section).offset();
    var scrollto = offset.top - 55; //position of the element - header height(ish)
    $('html, body').animate({
      scrollTop: scrollto
    }, 0);
  }

  $scope.clearSection = function (section, subsection) {
    if (!subsection) {
      $scope.Settings[section] = {};
    } else {
      $scope.Settings[section][subsection] = {};
    }
  }

  $scope.chipToUserJSON = function (chip) {
    var user = {};
    try {
      var user = JSON.parse(chip);
    } catch (e) {
      alert("Unable to parse json: " + e);
      return null;
    }
    if (!user.username || user.username === "") {
      alert("You must enter a username");
      return null;
    }

    return user;
  }

  $scope.addMultiAuthReadWrite = function () {
    if (!$scope.tempMultiAuthReadWrite) {
      $scope.tempMultiAuthReadWrite = [];
    }
    $scope.tempMultiAuthReadWrite.push("");
  }

  $scope.removeMultiAuthReadWrite = function (index) {
    $scope.tempMultiAuthReadWrite.splice(index, 1);
  }

  $scope.addMultiAuthReadOnly = function () {
    if (!$scope.tempMultiAuthReadOnly) {
      $scope.tempMultiAuthReadOnly = [];
    }
    $scope.tempMultiAuthReadOnly.push("");
  }

  $scope.removeMultiAuthReadOnly = function (index) {
    $scope.tempMultiAuthReadOnly.splice(index, 1);
  }

  $scope.invalidAuth = function (kind) {
    return ($scope.validAuthKinds.indexOf(kind) < 0);
  }

  $scope.addOktaScope = function () {
    if ($scope.Settings.auth === null || $scope.Settings.auth === undefined) {
      $scope.Settings.auth = {
        "okta": {
          "scopes": []
        }
      };
    }
    if ($scope.Settings.auth.okta === null || $scope.Settings.auth.okta === undefined) {
      $scope.Settings.auth.okta = {
        "scopes": []
      };
    }
    if ($scope.Settings.auth.okta.scopes === null || $scope.Settings.auth.okta.scopes === undefined) {
      $scope.Settings.auth.okta.scopes = [];
    }

    if (!$scope.validOktaScope($scope.new_okta_scope)) {
      $scope.invalidScope = "Scope cannot be empty.";
      return
    }

    $scope.Settings.auth.okta.scopes.push($scope.new_okta_scope);
    $scope.new_okta_scope = "";
    $scope.invalidOktaScope = "";
  }

  $scope.deleteOktaScope = function (index) {
    $scope.Settings.auth.okta.scopes.splice(index, 1);
  }

  $scope.validOktaScope = function (scope) {
    return scope;
  }

  $scope.addExpansion = function (chip) {
    var obj = {};
    pieces = chip.split(":");
    if (pieces.length !== 2) {
      alert("Input must be in the format of key:value");
      return null;
    }
    var key = pieces[0];
    if ($scope.tempExpansions[key]) {
      alert("Duplicate expansion: " + key);
      return null;
    }
    obj[key] = pieces[1];
    $scope.tempExpansions[key] = pieces[1];
    return obj;
  }

  $scope.addOwnerRepo = function () {
    if ($scope.Settings.project_creation === null || $scope.Settings.project_creation === undefined) {
      $scope.Settings.project_creation = {
        "repo_exceptions": [],
      };
    }

    if ($scope.Settings.project_creation.repo_exceptions === null || $scope.Settings.project_creation.repo_exceptions === undefined) {
      $scope.Settings.project_creation.repo_exceptions = [];
    }

    if (!$scope.validOwnerRepo($scope.new_owner_repo)) {
      $scope.invalidOwnerRepo = "Owner and Repo cannot be empty.";
      return
    }

    $scope.Settings.project_creation.repo_exceptions.push($scope.new_owner_repo);
    $scope.new_owner_repo = {};
    $scope.invalidOwnerRepo = "";
  }

  $scope.deleteOwnerRepo = function (index) {
    $scope.Settings.project_creation.repo_exceptions.splice(index, 1);
  }

  $scope.validOwnerRepo = function (owner_repo) {
    return owner_repo && owner_repo.owner && owner_repo.repo;
  }

  $scope.addOverride = function () {
    if ($scope.Settings.overrides == null) {
      $scope.Settings.overrides = {};
    }
    if ($scope.Settings.overrides.overrides == null) {
      $scope.Settings.overrides.overrides = [];
    }

    if (!$scope.validOverride($scope.new_override)) {
      $scope.invalidOverride = "section id, field, and value are required.";
      return
    }

    $scope.Settings.overrides.overrides.push($scope.new_override);
    $scope.new_override = {};
    $scope.invalidOverride = "";
  }

  $scope.deleteOverride = function (index) {
    $scope.Settings.overrides.overrides.splice(index, 1);
  }

  $scope.validOverride = function (override) {
    return override && override.section_id && override.field && override.value;
  }

  $scope.deleteJIRAProject = function (key) {
    if (!key) {
      return;
    }
    delete $scope.Settings.jira_notifications.custom_fields[key];
  }
  $scope.addJIRAProject = function () {
    var value = $scope.jiraMapping.newProject.toUpperCase();
    if (!value) {
      return;
    }
    if (!$scope.Settings.jira_notifications.custom_fields[value]) {
      $scope.Settings.jira_notifications.custom_fields[value] = {
        fields: {},
        components: [],
        labels: []
      };
    }
    delete $scope.jiraMapping.newProject;
  }
  $scope.addDisabledGQLQuery = function () {
    var value = $scope.queryMapping.newQuery
    if (!value) {
      return;
    }
    $scope.Settings.disabled_gql_queries = [...($scope.Settings.disabled_gql_queries || []), value]
    delete $scope.queryMapping.newQuery;
  }
  $scope.removeDisabledGQLQuery = function (queryName) {
    $scope.Settings.disabled_gql_queries = ($scope.Settings.disabled_gql_queries || []).filter(v => v !== queryName)
  }
  $scope.addJIRAFieldToProject = function (project) {
    var field = $scope.jiraMapping.newField[project];

    if ($scope.Settings.jira_notifications.custom_fields[project].fields == null) {
      $scope.Settings.jira_notifications.custom_fields[project].fields = {}
    }

    if (!field || $scope.Settings.jira_notifications.custom_fields[project].fields[field]) {
      return;
    }

    $scope.Settings.jira_notifications.custom_fields[project].fields[field] = "{FIXME}";
    delete $scope.jiraMapping.newField[project];
  }
  $scope.deleteJIRAFieldFromProject = function (project, field) {
    if (!field) {
      return;
    }
    delete $scope.Settings.jira_notifications.custom_fields[project].fields[field];
  }
  $scope.removeComponent = function (project, component) {
    var index = $scope.Settings.jira_notifications.custom_fields[project].components.indexOf(component);
    $scope.Settings.jira_notifications.custom_fields[project].components.splice(index, 1);
  }
  $scope.removeLabel = function (project, label) {
    var index = $scope.Settings.jira_notifications.custom_fields[project].labels.indexOf(label);
    $scope.Settings.jira_notifications.custom_fields[project].labels.splice(index, 1);
  }
  $scope.addComponent = function (project) {
    if ($scope.Settings.jira_notifications.custom_fields[project].components == null) {
      $scope.Settings.jira_notifications.custom_fields[project].components = [];
    }
    $scope.Settings.jira_notifications.custom_fields[project].components.push("");
  }
  $scope.addLabel = function (project) {
    if ($scope.Settings.jira_notifications.custom_fields[project].labels == null) {
      $scope.Settings.jira_notifications.custom_fields[project].labels = [];
    }
    $scope.Settings.jira_notifications.custom_fields[project].labels.push("");
  }

  $scope.jiraMapping = {};
  $scope.queryMapping = {}

  $scope.load();
}]);
