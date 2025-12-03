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
  },
  // Extracts referenced element from the obj
  // For obj = {a: {b: 3}} and reference = 'a.b'
  // Returns 3
  // Returns undefined if referenced element does not exist
  dereference: function(obj, reference) {
    return _.reduce(reference.split('.'), function(m, ref) {
      return m ? m[ref] : undefined
    }, obj)
  },
})

var mciModule = angular.module('MCI', [
  'directives.admin',
  'directives.badges',
  'directives.confirm',
  'directives.drawer',
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
  'ngMaterialDateRangePicker',
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
}).factory('confirmDialogFactory', function($mdDialog) {
  return function(text_or_function) {
    const formatter = _.isFunction(text_or_function) ? text_or_function : function() { return text_or_function};
    return function(items) {
      return $mdDialog.show(
        $mdDialog.confirm()
          .ok('Ok')
          .cancel('Cancel')
          .title('Confirm')
          .textContent(formatter(items))
      );
    }
  }
})