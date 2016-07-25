var directives = directives || {};

directives.patch = angular.module('directives.patch', ['filters.common', 'directives.github']);

directives.patch.directive('patchCommitPanel', function() {
  return {
    scope : true,
    restrict : 'E',
    templateUrl : '/static/partials/patch_commit_panel.html',
    link : function(scope, element, attrs) {
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
        // get diff totals
        totalAdd = 0;
        totalDel = 0;
        _.each(scope.patchinfo.Patch.Patches, function(patch) {
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

      // lookup table with constants for sorting / displaying diff results.
      // there is redundancy here between "success/pass" and "failed/fail"
      // to allow this to work generically with both test and task statuses
      scope.diffTypes = {
        successfailed:  {icon:"fa-bug", type: 0},
        passfail:       {icon:"fa-bug", type: 0},
        failedfailed:   {icon:"fa-question", type: 1},
        failfail:       {icon:"fa-question", type: 1},
        failed:         {icon:"fa-bug", type: 2},
        fail:           {icon:"fa-bug", type: 2},
        failedsuccess:  {icon:"fa-star", type: 3},
        failpass:       {icon:"fa-star", type: 3},
        success:        {icon:"fa-star", type: 4},
        pass:           {icon:"fa-star", type: 4},
        successsuccess: {icon:"", type: 5},
        passpass:       {icon:"", type: 5},
      };

      // helper for ranking status combinations
      scope.getDisplayInfo = function(diff) {
        // concat results for key lookup
        if (diff.original.status) {
          // task diffs have an extra layer we need to extract
          key = diff.original.status + diff.patch.status;
        } else {
          // test diffs are simpler
          key = diff.original + diff.patch;
        }
        if (key in scope.diffTypes) {
            return scope.diffTypes[key];
        }
        // else return a default
        return {icon: "", type:1000};
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
