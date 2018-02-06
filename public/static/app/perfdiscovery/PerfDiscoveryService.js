mciModule.factory('PerfDiscoveryService', function($q, ApiV1, ApiTaskdata) {
  var _respData = function(resp) {
    return resp.data
  }

  // TODO Should be part of `VersionResource`
  // :parm version: version object
  // :returns: Array of {taskId, taskName, buildName}
  function extractTasks(version) {
    return _.reduce(version.builds, function(items, build) {
      // Going over dict of tasks ({taskName: data})
      return items.concat(_.map(build.tasks, function(taskData, taskName) {
        return {
          taskId: taskData.task_id,
          taskName: taskName,
          buildName: build.name,
        }
      }))
    }, [])
  }

  // Makes series of HTTP calls and loads curent, baseline and history data
  // :rtype: $q.promise
  function queryData(tasks, version, baseline) {
    var current = $q.all(_.map(tasks, function(task) {
      return ApiTaskdata.getTaskById(task.taskId, 'perf')
        .then(_respData, function() { return null })
        .then(function(data) {
          if (data) {
            return $q.all({
              ctx: $q.resolve(task),
              current: data,
              history: ApiTaskdata.getTaskHistory(task.taskId, 'perf')
                .then(_respData, function(e) { return [] }),
              baseline: ApiTaskdata.getTaskCommit({
                projectId: data.project_id,
                revision: baseline.revision,
                variant: data.variant,
                taskName: task.taskName,
                name: 'perf',
              }).then(_respData, function() { null }),
            })
          } else {
            return null
          }
        })
    }))

    return $q.all({
      tasks: current,
    })
  }

  var seRe = new RegExp('[_ -](wt|wiredtiger|mmapv1|inmemory)$', 'i')

  // Takes build and task names
  // Extrcts storage engine from build/tasks name
  // :returns: dict of {storageEngine, task, build}
  // returned task and build names could be different
  function extractStorageEngine(build, task) {
    var storageEngine,
        attrs = {
          build: build,
          task: task,
        }

    if (build == undefined) return {}

    _.any(attrs, function(v, k) {
      var match = v.match(seRe)
      if (match) {
        storageEngine = match[1]
        attrs[k] = attrs[k].slice(0, match.start)
        return true
      }
    })

    // Set default
    if(!storageEngine) storageEngine = '(none)'

    return _.extend({storageEngine: storageEngine}, attrs)
  }

  function slug(data) {
    return [
      data.build,
      data.storageEngine,
      data.task, data.test,
      data.threads
    ].join(' - ') + 'T'
  }

  // :param item: test result data item
  // :param receiver: dict which will receive processed results
  // :param ctx: dict of {build, task, storageEngine} see `extractTasks`
  // returns: could be ignored
  function processItem(item, receiver, ctx) {
    var parts = item.name.split('-')
    // At some point we renamed the tests to remove -wiredTiger and -MMAPv1 suffixs.
    // Normalize old names to match the new ones
    if (parts.length == 2 && _.any('wWmM', _.bind(''.startsWith, parts[0]))) {
      item.name = parts[0]
    }

    return _.chain(item.results)
      .omit(_.isString)
      .each(function(speed, threads) {
        var extracted = extractStorageEngine(ctx.buildName, ctx.taskName)
        var data = {
          build: extracted.build,
          task: extracted.task,
          test: item.name,
          threads: +threads,
          speed: speed.ops_per_sec,
          storageEngine: extracted.storageEngine,
        }
        receiver[slug(data)] =  data
      })
  }

  // Processes `queryData` return value
  // Populates dicts of current (now, baseline) and history result items
  // returns dict of {now, baseline, history} test result items
  function processData(promise) {
    var now = {},
        baseline = {},
        history = {}

    return promise.then(function(data) {
      _.each(data.tasks, function(d) {
        _.each(d.current.data.results, function(result) {
          processItem(result, now, d.ctx)
        })
      })

      _.each(data.tasks, function(d) {
        _.each(d.baseline.data.results, function(result) {
          processItem(result, baseline, d.ctx)
        })
      })

      _.each(data.tasks, function(d) {
        _.each(d.history, function(histItems) {
          _.each(histItems.data.results, function(result) {
            processItem(result, history, d.ctx)
          })
        })
      })

      return {
        now: now,
        baseline: baseline,
        history: history,
      }
    })
  }

  // Calculates ratio for given test result `item` and `baseSpeed`
  function ratio(item, baseSpeed) {
    var ratio = item.speed / baseSpeed
    if (item.speed >= 0) {
      return ratio
    } else {
      return 1 / ratio
    }
  }

  function noBaselineFilter(results) {
    return function(item, id) {
      return results.baseline[id] == undefined
    }
  }

  // Returns data for the (ui) grid
  // :param promise: returned by `processData`
  function getRows(promise) {
    return promise.then(function(results) {
      return _.chain(results.now)
        .omit(noBaselineFilter(results))
        .map(function(item, id) {
          var baseline = results.baseline[id]
          var appendix = {
            ratio: ratio(item, baseline.speed),
            baseSpeed: baseline.speed,
            trendData: _.chain(results.history)
              .map(function(hist, id) {
                var base = results.baseline[id]
                if (base == undefined || !base.speed) {
                  return 1
                }
                return ratio(hist, baseline.speed)
              })
              .value(),
          }
          return _.extend({}, item, appendix)
        })
        .value()
    })
  }

  return {
    queryData: queryData,
    extractTasks: extractTasks,
    processData: processData,
    getRows: getRows,
  }
})
