var directives = directives || {};

directives.visualization = angular.module('directives.visualization', ['md5']);

directives.visualization.directive('progressBar', function($filter) {
  return function(scope, element, attrs) {
    var lastVal = 0;
    var lastClass = "progress-bar-success";
    var lastMax = 1;

    element.html('<div class="progress-bar progress-bar-success" role="progressbar" style="width: 0%"></div>');

    scope.$watch(attrs.progressBar, function(val) {
      if (isNaN(val)) {} else {
        lastVal = val;
        element.children('.progress-bar').css('width', (val / lastMax * 100) + '%');
        if (attrs.progressBarTitle) {
          $(element).children('.progress-bar').tooltip({
            title: scope.$eval(attrs.progressBarTitle),
            animation: false
          });
        }
      }
    });

    scope.$watch(attrs.progressBarMax, function(val) {
      if (isNaN(val)) {} else {
        lastMax = val || 1; // don't allow val = 0;
        element.children('.progress-bar').css('width', (lastVal / lastMax * 100) + '%');
        if (attrs.progressBarTitle) {
          $(element).children('.progress-bar').tooltip({
            title: scope.$eval(attrs.progressBarTitle),
            animation: false
          });
        }
      }
    });

    if (attrs.progressBarClass) {
      scope.$watch(attrs.progressBarClass, function(val) {
        element.children('.progress-bar').removeClass(lastClass);
        lastClass = $filter('statusFilter')(val);
        element.children('.progress-bar').addClass(lastClass);
      });
    }
  }
}).directive('resultsBar', function() {
  return {
    scope: true,
    restrict: 'A',
    link: function(scope, element, attrs) {
      scope.results = [];
      scope.widthPerSlice = 100 + '%';

      scope.$parent.$watch(attrs.resultsBar, function(v) {
        if (!v) {
          return;
        }

        scope.results = v;
        scope.widthPerSlice = (100 / (v.length || 1)) + "%";

        setTimeout(function() {
          // Need to do setTimeout so template has a chance to catch up
          $(element).children('.result-slice').each(function(i, el) {
            if (i < scope.results.length) {
              if (scope.results[i].tooltip) {
                $(el).tooltip({
                  title: scope.results[i].tooltip,
                  animation: false
                });
              }

              if (scope.results[i].link) {
                $(el).attr('href', scope.results[i].link);
              }
            }
          });
        }, 0);
      });
    },
    template: '<a ng-repeat="result in results" ng-style="{ display : \'block\', width : widthPerSlice }" ' +
      ' ng-class="result.class" class="result-slice"></div>'
  };
})
 //buildGrid creates a group of task boxes in react. It serves as a shim between
//our angularjs and the React code used to create and manage the boxes.
  .directive('buildGrid', function gridDirective() {
    return ({
        link: link,
        scope: {
          collapsed: "=",
          onClose: "&",
          build: "="
        }
    });
   function link(scope, element, attributes) {
     //project out the tasks from the uiTask struct to what the react code expects.
     var tasks = _.map(scope.build.Tasks, function (task) {
        var t = task.Task;
        t.failed_test_names = task.failed_test_names;
        return t;
     });
     // bind the angular $destroy to a function that removes the react element
     // from the DOM.
     scope.$on("$destroy", unmountReactElement);

     // bind angular's changes in the 'collapsed' variable to re-render the React
     // element.
     scope.$watch("collapsed", renderReactElement);

     // renderReactElement packages the props and renders the React element into the DOM.
     function renderReactElement() {
     var props = {
       onClose: function() {
         scope.$apply(scope.onClose);
       },
       build: {
         id: scope.build.Build._id,
         taskStatusCount: scope.build.taskStatusCount,
         tasks: tasks
       },
       rolledUp: false,
       collapseInfo: {
         collapsed: scope.collapsed,
         activeTaskStatuses : ['failed','system-failed']
       }
     }
     ReactDOM.render(
       React.createElement(Build, props),
       element[0]
     );
   }
  function unmountReactElement() {
    React.unmountComponentAtNode(element[0]);
  }
   }
}).directive('ngBindHtmlUnsafe', function() {
  // Because ng-bind-html prevents you from entering text with left angle
  // bracket (<) we can't use it for logs
  return function(scope, element, attr) {
    scope.$watch(attr.ngBindHtmlUnsafe, function(v) {
      element.html(v);
    });
  };
});
