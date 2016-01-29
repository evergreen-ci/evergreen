mciModule
// top-level controller for the waterfall
  .controller('WaterfallCtrl', function($scope, $window, $location, $locationHash) {

    // load in the build variants, sorting
    $scope.buildVariants = $window.serverData.build_variants.sort();

    //variable/function to handle expanding the header message
    $scope.showFullMessage = false;
    $scope.flipExpand = function() {
      $scope.showFullMessage = !$scope.showFullMessage;
    };


    // load in the versions
    $scope.versions = $window.serverData.versions;
    var versionsOnPage = 0;
    _.each($scope.versions, function(version) {

      // keep a running total of how many versions are on the page
      versionsOnPage += version.ids.length;

      // if there are no builds (the version is rolled up), create
      // placeholder builds
      if (!version.builds) {
        version.builds = [];
        for (var i = 0; i < $scope.buildVariants.length; i++) {
          version.builds.push({
            build_variant: $scope.buildVariants[i]
          })
        }
        return;
      }

      // sort the builds within the version by build variant
      version.builds.sort(function(a, b) {
        if (a.build_variant > b.build_variant) {
          return 1;
        } else if (a.build_variant < b.build_variant) {
          return -1;
        }
        return 0;
      });

      // iterate over all of the build variants - if the version is
      // missing a build for the variant, insert a blank one
      for (var i = 0, l = $scope.buildVariants.length; i < l; i++) {
        var buildVariantName = $scope.buildVariants[i];
        if (!version.rolled_up &&
          (!version.builds[i] ||
            version.builds[i].build_variant !== buildVariantName)) {
          version.builds.splice(i, 0, {
            build_variant: buildVariantName
          });
        }
      }


    });

    // load in the total number of versions
    var totalVersions = $window.serverData.total_versions;

    // the current skip value
    var currentSkip = $window.serverData.current_skip;

    // how many versions the next button should skip to
    var nextSkip = currentSkip + versionsOnPage;
    $scope.disableNextLink = false;
    if (nextSkip >= totalVersions) {
      $scope.disableNextLink = true;
    }

    // the number of versions on the previous page
    var previousPageCount = $window.serverData.previous_page_count;
    // where the previous button should skip to / if it should be disabled
    var previousSkip = currentSkip - previousPageCount;
    $scope.disablePreviousLink = (currentSkip == 0);

    // get the current url, stripping off the hash route and query params
    function getUrl() {
      var url = $location.absUrl();
      var hashIndex = url.indexOf('#');
      if (hashIndex != -1) {
        url = url.substr(0, hashIndex);
      }
      var queryIndex = url.indexOf('?');
      if (queryIndex != -1) {
        url = url.substr(0, queryIndex);
      }
      return url
    };
    // refs for the next and previous page of the waterfall
    $scope.previousPage = function() {
      return getUrl() + '?skip=' + previousSkip + '#/filter/' + $scope.filter.variant +
        '/' + $scope.filter.task;
    };
    $scope.nextPage = function() {
      return getUrl() + '?skip=' + nextSkip + '#/filter/' + $scope.filter.variant +
        '/' + $scope.filter.task;
    };

    // initialize the filter
    var filterOpts = $location.path().split('/');
    $scope.filter = {
      variant: filterOpts[2] || '',
      task: filterOpts[3] || ''
    };

    // tie the filter and the location hash together
    $scope.$watch('filter.variant', function() {
      $location.path('filter/' + $scope.filter.variant + '/' + $scope.filter.task);
    });
    $scope.$watch('filter.task', function() {
      $location.path('filter/' + $scope.filter.variant + '/' + $scope.filter.task);
    });
  })
  // directive to make the popover that is placed onclick above the rolled up content
  .directive('popoverSection', function ($filter) {
    function createPopoverInfo(id, revision, author, message, create_time, error, warning) {
      var errorIcon = "";
      if (error.messages && error.messages.length != 0) {
        errorIcon = '<span><i class="fa fa-ban error-text" data-element-tooltip="body">&nbsp;</i></span>';
      }
      if (warning.messages && warning.messages.length != 0) {
        errorIcon = '<span><i class="fa fa-warning warning-text" data-element-tooltip="body">&nbsp;</i></span>';
      }
      return '<div class="commit-meta"><div class="commit-date">' + (create_time ? create_time : '') + '</div>' +
        '<a href="/version/' + $filter('escapeHtml')(id) + '">' + errorIcon +
        '<span class="monospace">' + $filter('escapeHtml')(revision.substring(0, 10)) + '</span></a>' +
        ' - ' + '<strong>' + $filter('escapeHtml')(author) + '</strong></div>' +
        '<p>' + $filter('escapeHtml')(message) + '</p>';
    }

    return {
      restrict: 'E',
      scope: false,
      replace: true,
      link: function(scope, element, attrs) {
        // create the content of the popover that will appear over the version
        var popoverContent = '<ul class="githash-popover list-unstyled">';
        for (var i = 0; i < scope.version.messages.length; i++) {
          popoverContent += '<li>';
          popoverContent += createPopoverInfo(scope.version.ids[i],
            scope.version.revisions[i], scope.version.authors[i],
            scope.version.messages[i], $filter('date')(scope.version.create_times[i], 'short'),
            scope.version.errors[i], scope.version.warnings[i]);
          popoverContent += '</li>';
        }
        popoverContent += '</ul>';

        // init the popover
        $(element).popover({
          placement: 'bottom',
          content: popoverContent,
          html: true,
          container: 'body',
          template: '<div class="popover popover-wide" role="tooltip"><div class="arrow"></div><h3 class="popover-title"></h3><div class="popover-content"></div></div>'
        });
      }
    };
  })
  //directive to make the version header for each section of the table
  .directive('versionHeader', function ($filter) {
    return {
      restrict: 'E',
      replace: true,
      scope: false,
      link: function(scope, element, attr) {
        if (scope.version.rolled_up) {
          scope.version.rolled_header = scope.version.messages.length + ' inactive ' + $filter('pluralize')(scope.version.messages.length, 'version');
        } else {
          scope.version.create_time = $filter('date')(scope.version.create_times[0], 'short');
          scope.version.author = scope.version.authors[0];
          scope.version.id_link = "/version/"+scope.version.ids[0];
          scope.version.commit = scope.version.revisions[0].substring(0, 5);
          scope.version.message = scope.version.messages[0].substring(0,35);
        }
      }

    }
  })
  // directive for a single cell representing the result of a build
  .directive('buildResult', function() {
    return {
      restrict: 'E',
      scope: false,
      transclude: true, // so the task result directives can be put in the template
      link: function(scope, element, attrs) {},
      replace: true,
      template: '<div ng-transclude class="build-result"></div>'
    };
  })
  // directive for a slice of a waterfall cell representing the result of a task
  .directive('taskResult', function($filter) {
    return {
      restrict: 'E',
      scope: false,
      link: function(scope, element, attrs) {

        // create the tooltip title
        var title = scope.task.display_name + ' - ' + $filter('statusLabel')(scope.task);
        if (scope.task.status == 'success' ||
          scope.task.status == 'failed') {
          title += ' - ' + $filter('stringifyNanoseconds')(scope.task.time_taken);
        }

        // init the element's tooltip
        $(element).tooltip({
          title: title,
          placement: 'top',
          container: $('#wrapper'),
          animation: false,
        });

      },
      replace: true,
      template: '<a href="/task/[[task.id]]"' +
        ' class="task-result [[task | statusFilter]]"></a>'
    }
  })
  // directive for a smaller, mobile-friendly waterfall cell representing a build
  // outcome
  .directive('buildSummary', function() {
    return {
      restrict: 'E',
      scope: false,
      replace: true,
      link: function(scope, element, attrs) {
        scope.failed = 0;
        scope.succeeded = 0;
        if (scope.build.tasks) {
            // compute the number of failed and succeeded tasks
          for (var i = 0; i < scope.build.tasks.length; i++) {
            switch (scope.build.tasks[i].status) {
              case "failed":
                scope.failed++;
                break;
              case "success":
                scope.succeeded++;
                break;
            }
          }

          // don't display zero values
          ['failed', 'succeeded'].
          forEach(function(status) {
            if (!scope[status]) {
              scope[status] = '';
            }
          });
        }
      },
      templateUrl: '/static/partials/build_summary.html'
    };
  });
