var directives = directives || {};

directives.github = angular.module('directives.github', ['filters.common', 'directives.visualization']);

directives.github.directive('gravatar', function(md5) {
  return {
    restrict : 'A',
    link: function(scope, element, attrs) {
      // use protocol relative url to avoid mixed-content warning
      var gravatarUrl = "//www.gravatar.com/avatar/";
      var gravatarQuery = "?s=50&d=https%3A%2F%2Fi2.wp.com%2Fa248.e.akamai.net%2Fassets.github.com%2Fimages%2Fgravatars%2Fgravatar-user-420.png%3Fssl%3D1";

      scope.$watch(attrs.gravatar, function(v) {
        element.attr('src', gravatarUrl + md5.createHash(v.toLowerCase()) + gravatarQuery);
      });
    }
  };
});

directives.github.directive('githubCommitPanel', function(EvgUtil) {
  return {
    scope : true,
    restrict : 'E',
    templateUrl : '/static/partials/github_commit_panel.html',
    link : function(scope, element, attrs) {
      scope.commit = {};
      scope.$parent.$watch(attrs.commit, function(v) {
        scope.commit = v;

        // TODO Probably, instead of this hack, version/task models
        //      should have consistent time (EVG-5475)
        // Tries use create_time, if not available tries to parse time from id,
        // at the last, tries to use ingest_time
        scope.authoredAt = _.cascade(
          v.create_time,
          v.build_id && EvgUtil.parseId(v.build_id).createTime.zone(attrs.timezone).format(),
          v.ingest_time
        )
      });
      scope.timezone = attrs.timezone;
    }
  };
});
