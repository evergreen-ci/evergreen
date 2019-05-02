mciModule.factory('ApiTaskdata', function($http, $filter, ApiUtil, API_TASKDATA) {
  var get = ApiUtil.httpGetter(API_TASKDATA.BASE);
  const cedarApp = "https://cedar.mongodb.com"; //TODO make a configuration option
  var cedarAPI = ApiUtil.httpGetter(cedarApp);

  return {
    getTaskById: function(taskId, name) {
      return get(API_TASKDATA.TASK_BY_ID, {task_id: taskId, name: name})
    },

    getTaskHistory: function(taskId, name) {
      return get(API_TASKDATA.TASK_HISTORY, {task_id: taskId, name: name})
    },

    getTaskByCommit: function(param) {
      // Optional TODO Assert params
      return get(API_TASKDATA.TASK_BY_COMMIT, {
        project_id: param.projectId,
        revision: param.revision,
        variant: param.variant,
        task_name: param.taskName,
        name: param.name,
      })
    },

    getTaskByTag: function(param) {
      // Optional TODO Assert params
      return get(API_TASKDATA.TASK_BY_TAG, {
        project_id: param.projectId,
        tag: param.tag,
        variant: param.variant,
        task_name: param.taskName,
        name: param.name,
      })
    },

    getProjectTags: function(projectId) {
      return get(API_TASKDATA.PROJECT_TAGS, {}, {project_id: projectId})
    },

    getExpandedTaskById: function(taskId) {
      return cedarAPI("rest/v1/perf/task_id/" + taskId);
    },
  }
})
