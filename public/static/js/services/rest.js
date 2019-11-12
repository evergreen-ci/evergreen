var mciServices = mciServices || {};

mciServices.rest = angular.module('mciServices.rest', []);

mciServices.rest.RestV2Resource = function(resource) {
  return 'rest/v2/' + resource;
}

mciServices.rest.factory('mciBaseRestService', ['$http', function($http) {
    // private vars
    var baseUrl = '';
    var resources = {};

    // the service that will be returned
    var service = {};

    var httpCall = function(method, resource, idents, config, callbacks) {
        if (!$http[method] || typeof $http[method] !== 'function') {
            alert('invalid http method: ' + method);
            return;
        }

        config.method = method;
        config.url = [baseUrl, resource].concat(idents).join('/');
        var csrfElem = document.getElementsByName("gorilla.csrf.Token");
        if (csrfElem && csrfElem.length > 0) {
          config.headers = {
             'X-CSRF-Token': csrfElem[0].value
          };
        };

        $http(config).then(callbacks.success || function() {}, callbacks.error || function() {});
    };

    ['delete', 'get', 'post', 'put'].forEach(function(method) {
        service[method + 'Resource'] = function(resource, idents, config, callbacks) {
            httpCall(method, resource, idents, config, callbacks);
        };
    });

    return service;
}]);

mciServices.rest.factory('historyDrawerService', ['mciBaseRestService',
    function(baseSvc) {
        var resource = 'history';
        var service = {};
        var defaultRadius = 10;

        // modelType could be either "tasks" or "versions"
        var historyFetcher = function(modelType) {
            return function(modelId, historyType, radius, callbacks) {
                var config = {
                    params: {
                        radius: radius || defaultRadius
                    }
                };
                baseSvc.getResource(resource, [modelType, modelId, historyType], config, callbacks);
            }
        }

        service.fetchVersionHistory = historyFetcher("versions");

        return service;
    }
]);

mciServices.rest.factory('taskHistoryDrawerService', ['mciBaseRestService',
    function(baseSvc) {
        var resource = 'history';
        var service = {};
        var defaultRadius = 10;

        service.fetchTaskHistory = function(versionId, taskVariant, taskName, sortType, radius, callbacks) {
            var config = {
                params: {
                    radius: radius || defaultRadius
                }
            };
            baseSvc.getResource(resource, ["tasks", "2", versionId, sortType, taskVariant, taskName], config, callbacks);
        }

        return service;
    }
]);

mciServices.rest.factory('mciTasksRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'tasks';

    var service = {};

    service.getResource = function() {
        return resource;
    };

    service.takeActionOnTask = function(taskId, action, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        baseSvc.putResource(resource, [taskId], config, callbacks);
    };

    service.getTask = function(taskId, callbacks) {
        baseSvc.getResource(resource, [taskId], {}, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciBuildsRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'builds';

    var service = {};

    service.takeActionOnBuild = function(buildId, action, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        baseSvc.putResource(resource, [buildId], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciHostRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'host';

    var service = {};

    service.updateStatus = function(hostId, action, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        baseSvc.putResource(resource, [hostId], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciHostsRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'hosts';

    var service = {};

    service.updateStatus = function(hostIds, action, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        config.data['host_ids'] = hostIds;
        baseSvc.putResource(resource, [], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciVersionsRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'version';

    var service = {};

    service.takeActionOnVersion = function(versionId, action, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        baseSvc.putResource(resource, [versionId], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciBuildVariantHistoryRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'build_variant';

    var service = {};

    service.getBuildVariantHistory = function(project, buildVariant, params, callbacks) {
        var _project = encodeURIComponent(project);
        var _buildVariant = encodeURIComponent(buildVariant);

        var config = {
            params: params
        };
        baseSvc.getResource(resource, [_project, _buildVariant], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciTaskHistoryRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'task_history';

    var service = {};

    service.getTaskHistory = function(project, taskName, params, callbacks) {
        var _project = encodeURIComponent(project);
        var _taskName = encodeURIComponent(taskName);

        var config = {
            params: params
        };
        baseSvc.getResource(resource, [_project, _taskName], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciLoginRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'login';

    var service = {};

    service.authenticate = function(username, password, data, callbacks) {
        var config = {
            data: data
        };
        config.data['username'] = username;
        config.data['password'] = password;
        baseSvc.postResource(resource, [], config, callbacks);
    };

    return service;
}]);

mciServices.rest.factory('mciSpawnRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'spawn';

    var service = {};

    service.getSpawnedHosts = function(action, params, callbacks) {
        baseSvc.getResource(resource, action, params, callbacks);
    }

    service.getSpawnableDistros = function(action, params, callbacks) {
        var config = {
            params: params
        };
        baseSvc.getResource(resource, action, config, callbacks);
    };

    service.getUserKeys = function(action, params, callbacks) {
        var config = {
            params: params
        };
        baseSvc.getResource(resource, action, config, callbacks);
    };

    service.getAllowedInstanceTypes = function(action, provider, params, callbacks) {
        var config = {
            params: params
        }
        config.params['provider'] = provider
        baseSvc.getResource(resource, action, config, callbacks);
    }

    service.spawnHost = function(spawnInfo, data, callbacks) {
        var config = {
            data: data
        };
        config.data['task_id'] = spawnInfo.task_id;
        config.data['distro'] = spawnInfo.distroId;
        config.data['save_key'] = spawnInfo.saveKey;
        config.data['key_name'] = spawnInfo.spawnKey.name;
        config.data['public_key'] = spawnInfo.spawnKey.key;
        config.data['userdata'] = spawnInfo.userData;
        config.data['use_task_config'] = spawnInfo.useTaskConfig;
        baseSvc.putResource(resource, [], config, callbacks);
    };

    service.updateHostStatus = function(action, hostId, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        config.data['host_id'] = hostId;
        baseSvc.postResource(resource, [], config, callbacks);
    };

    service.updateRDPPassword = function(action, hostId, rdpPassword, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        config.data['host_id'] = hostId;
        config.data['rdp_pwd'] = rdpPassword;
        baseSvc.postResource(resource, [], config, callbacks);
    };

    service.extendHostExpiration = function(action, hostId, newExpiration, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        config.data['host_id'] = hostId;
        config.data['expiration'] = newExpiration;
        baseSvc.postResource(resource, [], config, callbacks);
    };

    service.updateHostTags = function(action, hostId, tagsToAdd, tagsToDelete, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        config.data['host_id'] = hostId;
        config.data['tags_to_add'] = tagsToAdd;
        config.data['tags_to_delete'] = tagsToDelete;
        baseSvc.postResource(resource, [], config, callbacks);
    }

    service.updateInstanceType = function(action, hostId, newInstanceType, data, callbacks) {
        var config = {
            data: data
        };
        config.data['action'] = action;
        config.data['host_id'] = hostId;
        config.data['instance_type'] = newInstanceType;
        baseSvc.postResource(resource, [], config, callbacks);
    }

    return service;
}]);

mciServices.rest.factory('mciTaskStatisticsRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'task_stats';
    var service = {};

    service.getTimeStatistics = function getTimeStatistics(field1, field2, groupByField, days, callbacks) {
        baseSvc.getResource(resource, [field1, field2, groupByField, days], {}, callbacks);
    };

    return service;
}]);



mciServices.rest.factory('mciDistroRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = 'distros';

    var service = {};

    service.addDistro = function(data, callbacks) {
        var config = {
            data: data
        };
        baseSvc.putResource(resource, [], config, callbacks);
    }

    service.modifyDistro = function(distroId, data, shouldDeco, callbacks) {
        var config = {
            data: data,
            params: {
              deco: shouldDeco
            }
        };
        baseSvc.postResource(resource, [distroId], config, callbacks);
    }

    service.removeDistro = function(distroId, shouldDeco, callbacks) {
        var config = {
            params: {
              deco: shouldDeco
            }
        };
        baseSvc.deleteResource(resource, [distroId], config, callbacks);
    }

    return service;
}]);

mciServices.rest.factory('mciAdminRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = mciServices.rest.RestV2Resource("admin");

    var service = {};

    service.getSettings = function(callbacks) {
      baseSvc.getResource(resource + "/settings", [], {}, callbacks);
    }

    service.saveSettings = function(settings, callbacks) {
      var config = {
          data: settings
      };
      baseSvc.postResource(resource + "/settings", [], config, callbacks);
    }

    service.restartItems = function(from, to, isDryRun, restartType, restartRed, restartPurple, restartLavender, callbacks) {
      var config = {}
      config.data = {
        start_time: from,
        end_time: to,
        dry_run: isDryRun,
        include_test_failed: restartRed,
        include_sys_failed: restartPurple,
        include_setup_failed: restartLavender,
      };
      baseSvc.postResource(resource + "/restart/" + restartType, [], config, callbacks);
    }

    service.getEvents = function(timestamp, limit, callbacks) {
      if (!limit || limit === 0) {
        limit = 15;
      }
      var url = resource + "/events" + "?limit=" + limit;
      if (timestamp && timestamp !== "") {
        url += "&ts=" + timestamp;
      }
      baseSvc.getResource(url, [], {}, callbacks);
    }

    service.revertEvent = function(guid, callbacks) {
      var config = {};
      config.data = {
        guid: guid,
      };
      baseSvc.postResource(resource + "/revert", [], config, callbacks);
    }

    service.clearCommitQueues = function(callbacks) {
        baseSvc.deleteResource(resource + "/commit_queues", [], {}, callbacks);
    }

    return service;
}]);

mciServices.rest.factory('mciProjectRestService', ['mciBaseRestService', function(baseSvc) {
    var resource = mciServices.rest.RestV2Resource("projects");

    var service = {};

    service.getEvents = function(timestamp, limit, callbacks, project_id) {
      if (!limit || limit === 0) {
        limit = 15;
      }

      var _project = encodeURIComponent(project_id)
      var url = [resource, _project, "events"].join('/') + "?limit=" + limit;
      if (timestamp && timestamp !== "") {
        url += "&ts=" + timestamp;
      }

      baseSvc.getResource(url, [], {}, callbacks);
    }

    return service;
}]);

mciServices.rest.factory('mciUserSettingsService', ['mciBaseRestService', function(baseSvc) {
    var resource = mciServices.rest.RestV2Resource("user/settings");

    var service = {};

    service.getUserSettings = function(callbacks) {
      baseSvc.getResource(resource, [], {}, callbacks);
    }

    service.saveUserSettings = function(settings, callbacks) {
      var config = {
          data: settings
      };
      baseSvc.postResource(resource, [], config, callbacks);
    }
    return service;
}]);

mciServices.rest.factory('mciSubscriptionsService', ['mciBaseRestService', function(baseSvc) {
    var resource = mciServices.rest.RestV2Resource("subscriptions");

    var service = {};

    service.get = function(owner, type, callbacks) {
      var queryString = "?owner="+owner+"&type="+type
      baseSvc.getResource(resource+queryString, [], {}, callbacks);
    }

    service.post = function(subscriptions, callbacks) {
      baseSvc.postResource(resource, [], {data: subscriptions}, callbacks);
    }

    service.delete = function(id, callbacks) {
      var queryString = "?id="+id;
      baseSvc.deleteResource(resource+queryString, [], {}, callbacks);
    }

    return service;
}]);
