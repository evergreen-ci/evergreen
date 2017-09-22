mciModule.factory('$timeline', function($http, $filter) {
  var data = {
    Versions: []
  };

  var endpoint = '/json/timeline/' + encodeURIComponent(project);

  data.loadMore = function(project) {
    var params = {
      skip: data.Versions.length
    };

    $http.
    get(endpoint, {
      params: params
    }).then(
    function(resp) {
      var result = resp.data;
      data.TotalVersions = result.TotalVersions;
      data.Versions =
        data.Versions.concat(result.Versions);
    },
    function(resp) {
      data.error = resp.data.message;
    });
  };

  data.loadPage = function(project, limit, skip) {
    var params = {
      limit: limit,
      skip: skip
    };

    data.Versions = [];
    $http.
    get(endpoint, {
      params: params
    }).then(
    function(resp) {
      var result = resp.data;
      data.TotalVersions = result.TotalVersions;
      data.Versions = result.Versions;

      _.each(data.Versions, function(version) {
        _.each(version.Builds, function(build) {
          build.taskResults = [];
          _.each(build.Build.tasks, function(task) {
            build.taskResults.push({
              link: '/task/' + task.id,
              tooltip: task.display_name,
              'class': $filter('statusFilter')(task),
            });
          });
        });
      });
    },
    function(resp) {
      data.error = resp.data.message;
    });
  };

  return data;
});
