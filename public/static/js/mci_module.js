// Use '{name}' pattern for underscore string interpolation
// (For consistency with GoLang routes)
_.templateSettings = {
  interpolate: /\{(.+?)\}/g
}

_.mixin({
  sum: function(coll) {
    return _.reduce(coll, function(m, d) {
      return m + d
    }, 0)
  },
  patch: function(coll, patch) {
    _.each(coll, function(d) {
      _.extend(d, patch)
    })
  }
})

var mciModule = angular.module('MCI', [
  'directives.admin',
  'directives.badges',
  'directives.confirm',
  'directives.drawer',
  'directives.eventLogs',
  'directives.events',
  'directives.github',
  'directives.patch',
  'directives.spawn',
  'directives.svg',
  'directives.tristateCheckbox',
  'directives.visualization',
  'filters.common',
  'mciServices.locationHash',
  'mciServices.rest',
  'md.time.picker',
  'md5',
  'ngMaterial',
  'ngRoute',
  'ngSanitize',
  'ui.grid',
  'ui.grid.autoResize',
  'ui.grid.emptyBaseLayer',
  'ui.grid.grouping',
  'ui.grid.moveColumns',
  'ui.grid.pinning',
  'ui.grid.resizeColumns',
  'ui.grid.selection',
  'ui.select',
], function($interpolateProvider, $locationProvider) {
  // Use [[ ]] to delimit AngularJS bindings, because using {{ }} confuses go
  $interpolateProvider.startSymbol('[[');
  $interpolateProvider.endSymbol(']]');
  $locationProvider.hashPrefix('!')
}).factory('$now', [function() {
  return {
    now: function() {
      return new Date();
    }
  };
}]).filter('taskStatusLabel', function() {
  return function(status, type) {
    switch (status) {
      case 'started':
        return type + '-info';
      case 'undispatched':
        return type + '-default';
      case 'dispatched':
        return type + '-info';
      case 'failed':
        return type + '-danger';
      case 'cancelled':
        return type + '-warning';
      case 'success':
        return type + '-success';
      default:
        return type + '-default';
    }
  }
}).directive('bsPopover', function($parse, $compile, $templateCache, $q, $http) {
  // This is adapted from https://github.com/mgcrea/angular-strap/blob/master/src/directives/popover.js
  // but hacked to allow passing in the 'container' option
  // and fix weirdness by wrapping element with $()

  // Hide popovers when pressing esc
  $('body').on('keyup', function(ev) {
    if (ev.keyCode === 27) {
      $('.popover.in').popover('hide');
    }
  });
  var type = 'popover',
    dataPrefix = !!$.fn.emulateTransitionEnd ? 'bs.' : '',
    evSuffix = dataPrefix ? '.' + dataPrefix + type : '';

  return {
    restrict: 'A',
    scope: true,
    link: function postLink(scope, element, attr, ctrl) {
      var getter = $parse(attr.bsPopover),
        setter = getter.assign,
        value = getter(scope),
        options = {};

      if (angular.isObject(value)) {
        options = value;
      }

      $q.when(options.content || $templateCache.get(value) || $http.get(value, {
        cache: true
      })).then(function onSuccess(template) {

        // Handle response from $http promise
        if (angular.isObject(template)) {
          template = template.data;
        }

        // Handle data-placement and data-trigger attributes
        _.forEach(['placement', 'trigger', 'container'], function(name) {
          if (!!attr[name]) {
            options[name] = attr[name];
          }
        });

        // Handle data-unique attribute
        if (!!attr.unique) {
          $(element).on('show' + evSuffix, function(ev) { // requires bootstrap 2.3.0+
            // Hide any active popover except self
            $('.popover.in').not(element).popover('hide');
          });
        }

        // Handle data-hide attribute to toggle visibility
        if (!!attr.hide) {
          scope.$watch(attr.hide, function(newValue, oldValue) {
            if (!!newValue) {
              popover.hide();
            } else if (newValue !== oldValue) {
              $timeout(function() {
                popover.show();
              });
            }
          });
        }

        if (!!attr.show) {
          scope.$watch(attr.show, function(newValue, oldValue) {
            if (!!newValue) {
              $timeout(function() {
                popover.show();
              });
            } else if (newValue !== oldValue) {
              popover.hide();
            }
          });
        }

        // Initialize popover
        $(element).popover(angular.extend({}, options, {
          content: template,
          html: true
        }));

        // Bootstrap override to provide tip() reference & compilation
        var popover = $(element).data(dataPrefix + type);
        popover.hasContent = function() {
          return this.getTitle() || template; // fix multiple $compile()
        };
        popover.getPosition = function() {
          var r = $.fn.popover.Constructor.prototype.getPosition.apply(this, arguments);

          // Compile content
          $compile(this.$tip)(scope);
          scope.$digest();

          // Bind popover to the tip()
          this.$tip.data(dataPrefix + type, this);

          return r;
        };

        // Provide scope display functions
        scope.$popover = function(name) {
          popover(name);
        };
        _.forEach(['show', 'hide'], function(name) {
          scope[name] = function() {
            popover[name]();
          };
        });
        scope.dismiss = scope.hide;

        // Emit popover events
        _.forEach(['show', 'shown', 'hide', 'hidden'], function(name) {
          $(element).on(name + evSuffix, function(ev) {
            scope.$emit('popover-' + name, ev);
          });
        });

      });
    }
  }
}).directive('elementTooltip', function() {
  return {
    scope: true,
    link: function(scope, element, attrs) {
      scope.$watch(attrs.elementTooltip, function(tip) {
        var obj = {
          title: tip
        };
        if (attrs.elementTooltipContainer) {
          obj.container = attrs.elementTooltipContainer;
        }

        $(element).elementTooltip = $(element).tooltip(obj);
        $(element).attr('title', tip).tooltip('fixTitle');
      });
    }
  }
}).directive('buildTasksResultsBar', function() {
  return function(scope, element, attrs) {
    // Progress bar to display the state of tasks for a given uiBuild
    scope.$watch(attrs.buildTasksResultsBar, function(build) {
      if (build) {
        var numSuccess = 0;
        var numFailed = 0;
        var numStarted = 0;
        var numNeither = 0;

        for (var i = 0; i < build.Tasks.length; ++i) {
          switch (build.Tasks[i].Task.Status) {
            case 'success':
              ++numSuccess;
              break;
            case 'failed':
              ++numFailed;
              break;
            case 'started':
              ++numStarted;
              break;
            default:
              ++numNeither;
              break;
          }
        }

        var successTitle = numSuccess + " task" + (numSuccess == 1 ? "" : "s") + " succeeded";
        var failedTitle = numFailed + " task" + (numFailed == 1 ? "" : "s") + " failed";
        var startedTitle = numStarted + " task" + (numStarted == 1 ? "" : "s") + " in progress";
        var neitherTitle = numNeither + " task" + (numNeither == 1 ? "" : "s") + " not started or cancelled";
        element.html('<div class="progress-bar progress-bar-success" role="progressbar" style="width: ' + (numSuccess / build.Tasks.length * 100) + '%" data-toggle="tooltip" data-animation="" title="' + successTitle + '"></div>' +
          '<div class="progress-bar progress-bar-danger" role="progressbar" style="width: ' + (numFailed / build.Tasks.length * 100) + '%" data-toggle="tooltip" data-animation="" title="' + failedTitle + '"></div>' +
          '<div class="progress-bar progress-bar-warning" role="progressbar" style="width: ' + (numStarted / build.Tasks.length * 100) + '%" data-toggle="tooltip" data-animation="" title="' + startedTitle + '"></div>' +
          '<div class="progress-bar progress-bar-default" role="progressbar" style="width: ' + (numNeither / build.Tasks.length * 100) + '%" data-toggle="tooltip" data-animation="" title="' + neitherTitle + '"></div>');

        $(element.children('*[data-toggle="tooltip"]')).each(function(i, el) {
          $(el).tooltip();
        });
      }
    });
  };
}).filter('statusFilter', function() {
  return function(task) {
    // for task test results, return the status passed in
    if (task !== Object(task)) {
      return task;
    }
    var cls = task.status;
    if (task.status == 'undispatched' || (task.display_only && task.task_waiting)) {
      if (!task.activated) {
        cls = 'inactive';
      } else {
        cls = 'unstarted';
      }
    } else if (task.status == 'started') {
      cls = 'started';
    } else if (task.status == 'success') {
      cls = 'success';
    } else if (task.status == 'failed') {
      cls = 'failed';
      // Get a property name where task details are stored
      // Name may differe from API to API
      let detailsProp = _.find(['task_end_details', 'status_details'],  d => d in task)
      if (detailsProp) {
        let details = task[detailsProp]
        if ('type' in details) {
          if (details.type == 'system' && details.status != 'success') {
            cls = 'system-failed';
          }
          if (details.type == 'setup' && details.status != 'success') {
            cls = 'setup-failed';
          }
        }
        if ('timed_out' in details) {
          if (details.timed_out && 'desc' in details && details.desc == 'heartbeat') {
            cls = 'system-failed';
          }
        }
      }
    }
    return cls;
  }
}).filter('statusLabel', function() {
  return function(task) {
    if (task.task_waiting) {
      return task.task_waiting;
    }
    if (task.status == 'started') {
      return 'started';
    } else if (task.status == 'undispatched' && task.activated) {
      if (task.task_waiting) {
        return task.task_waiting;
      }
      return 'scheduled';
    } else if (task.status == 'undispatched' && !task.activated){
      // dispatch_time could be a string or a number. to check when the dispatch_time
      // is a real value, this if-statement accounts for cases where
      // dispatch_time is 0, "0" or even new Date(0) or older.
      if (!task.dispatch_time || task.dispatch_time == 0 || (typeof task.dispatch_time === "string" && +new Date(task.dispatch_time) <= 0)) {
        return "not scheduled"
      }
      return 'aborted';
    } else if (task.status == 'success') {
      return 'success';
    } else if (task.status == 'failed') {
      // Get a property name where task details are stored
      // Name may differe from API to API
      let detailsProp = _.find(['task_end_details', 'status_details'],  d => d in task)
      if (detailsProp) {
        let details = task[detailsProp]
        if ('timed_out' in details) {
          if (details.timed_out && 'desc' in details && details.desc == 'heartbeat') {
            return 'system unresponsive';
          }
          if (details.type == 'system') {
            return 'system timed out';
          }
          return 'test timed out';
        }
        if (details.type == 'system' && details.status != 'success') {
          return 'system failure';
        }
        if (details.type == 'setup' && details.status != 'success') {
          return 'setup failure';
        }
        return 'failed';
      }
    }
    return task.status;
  }
}).filter('endOfPath', function() {
  return function(input) {
    var lastSlash = input.lastIndexOf('/');
    if (lastSlash === -1 || lastSlash === input.length - 1) {
      // try to find the index using windows-style filesystem separators
      lastSlash = input.lastIndexOf('\\');
      if (lastSlash === -1 || lastSlash === input.length - 1) {
        return input;
      }
    }
    return input.substring(lastSlash + 1);
  }
}).filter('buildStatus', function() {
  // given a list of tasks, returns the status of the overall build.
  return function(tasks) {
    var countSuccess = 0;
    var countInProgress = 0;
    var countActive = 0;
    for(i=0;i<tasks.length;i++){
      // if any task is failed, the build status is "failed"
      if(tasks[i].status == "failed"){
        return "block-status-failed";
      }
      if(tasks[i].status == "success"){
        countSuccess++;
      } else if(tasks[i].status == "dispatched" || tasks[i].status=="started"){
        countInProgress++;
      } else if(tasks[i].status == "undispatched") {
        countActive += tasks[i].activated ? 1 : 0;
      }
    }
    if(countSuccess == tasks.length){
      // all tasks are passing
      return "block-status-success";
    }else if(countInProgress>0){
      // no failures yet, but at least 1 task in still progress
      return "block-status-started";
    }else if(countActive>0){
      // no failures yet, but at least 1 task still active
      return "block-status-created";
    }
    // no active tasks pending
    return "block-status-inactive";
  }
}).factory('mciTime', [function() {
  var $time = {
    now: function() {
      return new Date();
    },
    // Some browsers, e.g. Safari, don't handle things like new Date(undefined)
    // particularly well, so this is to avoid that headache
    fromNanoseconds: function(nano) {
      if (nano) {
        return new Date(Math.ceil(nano / (1000 * 1000)));
      }
      return null;
    },
    fromMilliseconds: function(ms) {
      if (ms) {
        return new Date(ms);
      }
      return null;
    },
    finishConditional: function(start, finish, now) {
      // Pretty common calculation - if start is undefined, return 0.
      // If start is defined and finish isn't, return now - start in millis,
      // and if start and finish are both defined, return finish - start in millis

      if (!start || isNaN(start.getTime()) || start.getTime() <= 0) {
        return 0;
      } else if (!finish || isNaN(finish.getTime()) || finish.getTime() <= 0) {
        return (now || $time.now()).getTime() - start.getTime();
      } else {
        return finish.getTime() - start.getTime();
      }
    }
  };

  return $time;
}]).config(['$compileProvider', function ($compileProvider) {
  //$compileProvider.debugInfoEnabled(false);
}]).config(['$locationProvider', function($locationProvider) {
  $locationProvider.hashPrefix('');
}]).config(function($mdThemingProvider) {
  $mdThemingProvider.theme('default')
    .primaryPalette('green')
}).run(function($window) {
  // JSDOM fails while executing these lines for some reason
  if (!$window.document) return
  if (!$window.nav) return

  let currentNavHeight; 
  let navEl = $window.nav
  let bodyEl = $window.document.body

  function onResize() {
    if (navEl.clientHeight != currentNavHeight) {
      bodyEl.style.paddingTop = navEl.clientHeight + 'px'
      currentNavHeight = navEl.clientHeight
    }
  }

  // Make navbar/content height responsive
  angular.element($window).bind('resize', onResize)

  // Trigger on app start
  onResize()
})
