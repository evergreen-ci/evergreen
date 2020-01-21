var directives = directives || {};

directives.patch = angular.module('directives.patch', ['filters.common', 'directives.github']);

directives.patch.directive('patchCommitPanel', function() {
  return {
    scope : true,
    restrict : 'E',
    templateUrl : '/static/partials/patch_commit_panel.html',
    link : function(scope, element, attrs) {
      scope.formatByCommit = function(summaries) {
          var byCommits = {};
          _.each(summaries, function(summary) {
              if (byCommits[summary.Description] !== undefined) {
                  byCommits[summary.Description].push(summary);
              } else {
                  byCommits[summary.Description] = [summary];
              }
          });
          return byCommits
      };
      scope.patchinfo = {};
      scope.basecommit = {};
      if(attrs.hideDescription){
        scope.hideDescription = true
      }
      scope.$parent.$watch(attrs.basecommit, function(v) {
        scope.basecommit = v;
      });
      scope.$parent.$watch(attrs.patchinfo, function(v) {
        scope.patchinfo = v;
        scope.patchinfo.shorthash = scope.patchinfo.Patch.Githash.substr(0, 10)
        // get diff totals
        totalAdd = 0;
        totalDel = 0;
        _.each(scope.patchinfo.Patch.Patches, function(patch) {
          patch.formatted = scope.formatByCommit(patch.PatchSet.Summary);

          _.each(patch.PatchSet.Summary, function(diff) {
            totalAdd += diff.Additions;
            totalDel += diff.Deletions;
          })
        });
        scope.totals = {additions: totalAdd, deletions: totalDel};
      });
      scope.timezone = attrs.timezone;
      scope.base = attrs.base;
      scope.baselink = attrs.baselink;
    }
  };
});

directives.patch.directive('patchDiffPanel', function() {
  return {
    scope : true,
    restrict : 'E',
    templateUrl : '/static/partials/patch_diff.html',
    link : function(scope, element, attrs) {

      scope.baselink = attrs.baselink;
      scope.type = attrs.type;

      const failedStrings = ["fail", "failed", "timed-out", "test-timed-out", "unresponsive"];
      const successStrings = ["success", "pass"];
      const failCategory = "fail";
      const successCategory = "success";
      const unknownCategory = "unknown";
      
      scope.category = function(status) {
        if (failedStrings.includes(status)) {
          return failCategory;
        }
        if (successStrings.includes(status)) {
          return successCategory;
        }
        return unknownCategory;
      };

      // helper for ranking status combinations
      scope.getDisplayInfo = function(diff) {
        var beforeCategory, afterCategory;
        if (typeof(diff.original) == "object") {
          // task diffs have an extra layer we need to extract
          beforeCategory = scope.category(diff.original.status);
          afterCategory = scope.category(diff.patch.status);
        } else {
          beforeCategory = scope.category(diff.original);
          afterCategory = scope.category(diff.patch);
        }
        
        if (beforeCategory === successCategory && afterCategory === failCategory) {
          return {icon:"fa-bug", type: 0};
        }
        if (beforeCategory === failCategory && afterCategory === failCategory) {
          return {icon:"fa-question", type: 1};
        }
        if (beforeCategory === unknownCategory && afterCategory === failCategory) {
          return {icon:"", type: 2};
        }
        if (beforeCategory === failCategory && afterCategory === successCategory) {
          return {icon:"fa-star", type: 3};
        }
        if (beforeCategory === unknownCategory && afterCategory === successCategory) {
          return {icon:"", type: 4};
        }
        return {icon:"", type: 5};
      }

      scope.diffs = [];
      scope.$parent.$watch(attrs.diffs, function(d) {
        // only iterate if valid diffs are given
        if (!d || !d.length) {
          return
        }
        scope.diffs = d;
        _.each(scope.diffs, function(diff) {
          diff.originalLink = scope.baselink + diff.original;
          diff.patchLink = scope.baselink + diff.patch;
          diff.displayInfo = scope.getDisplayInfo(diff.diff);
        });
      });

    }
  };
});
