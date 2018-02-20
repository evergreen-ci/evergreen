mciModule.factory('ApiTaskdata', function($http, ApiUtil, API_TASKDATA) {
  var get = ApiUtil.httpGetter(API_TASKDATA.BASE)

  return {
    getTaskById: function(taskId, name) {
      return get(API_TASKDATA.TASK_BY_ID, {task_id: taskId, name: name})
    },

    getTaskHistory: function(taskId, name) {
      return get(API_TASKDATA.TASK_HISTORY, {task_id: taskId, name: name})
    },

    getTaskCommit: function(param) {
      // TODO Assert params
      return get(API_TASKDATA.TASK_COMMIT, {
        project_id: param.projectId,
        revision: param.revision,
        variant: param.variant,
        task_name: param.taskName,
        name: param.name,
      })
    },

    getProjectTags: function() {
      return get(API_TASKDATA.PROJECT_TAGS)
    },
  }
})
