mciModule.factory('$timeline', function($http) {
  var data = {
    Versions: []
  };

  var endpoint = '/json/timeline/' + encodeURIComponent(project);

  data.loadMore = function(project) {
    var params = {
      skip: data.Versions.length
    };

    $http.
      get(endpoint, { params: params }).
      success(function(result) {
        data.TotalVersions = result.TotalVersions;
        data.Versions =
          data.Versions.concat(result.Versions);
      }).
      error(function(error) {
        data.error = error.message;
      });
  };

  data.loadPage = function(project, limit, skip) {
    var params = {
      limit: limit,
      skip: skip
    };

    data.Versions = [];
    $http.
      get(endpoint, { params: params }).
      success(function(result) {
        data.TotalVersions = result.TotalVersions;
        data.Versions = result.Versions;

        _.each(data.Versions, function(version) {
          _.each(version.Builds, function(build) {
            build.taskResults = [];
            _.each(build.Tasks, function(task) {
              build.taskResults.push({
                link: '/task/' + task.Task.id,
                tooltip: task.Task.display_name,
                'class': task.Task.status
              });
            });
          });
        });
      }).
      error(function(error) {
        data.error = error.message;
      });
  };

  return data;
});