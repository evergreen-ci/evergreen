var directives = directives || {};
directives.drawer = angular.module('directives.drawer', []);
directives.drawer.directive('revisionBlurb', function($timeout) {
  return {
    restrict: 'E', 
    scope: {
      'linkType':'=linktype',
      'linkId':'=linkid',
      'revision':'=revision',
      'task':'=task',
      'hash':'&',
    },
    link: function(scope, element, attrs) {
      scope.getHref = function(){
        href = "/" + scope.linkType + "/" + scope.linkId;
        if(scope.hash){
          href = href + "#" + scope.hash();
        }
        return href;
      }
      scope.classFromStatus = function(status) {
        switch (status) {
          case 'failed':
          case 'timed_out':
          case 'heartbeat_timeout':
            return 'failed';
          case 'success':
            return 'success';
          case 'dispatched':
            return 'dispatched';
          case 'started':
            return 'started';
          case 'inactive':
            return 'inactive';
          default:
            return 'unstarted';
        }
      }

      if(scope.linkType=='task'){
        scope.showStatus = true
      }

      scope.revision.exec = { status: 'inactive' };
      if(scope.revision.task){
        scope.revision.exec = scope.revision.task;
      }
    },
    templateUrl: '/static/partials/revision_blurb.html'
  }

});

