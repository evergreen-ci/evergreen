mciModule.factory('PerfDiscoveryDataService', function (
  $q, $window, ApiV1, ApiV2, ApiTaskdata, BF, EVG, MPA_UI,
  PERF_DISCOVERY, Stitch, STITCH_CONFIG
) {
  var PD = PERF_DISCOVERY

  /*********************
   * UTILITY FUNCTIONS *
   *********************/

  var respData = function (resp) {
    return resp.data
  }

  function slug(data) {
    return [
      data.build,
      data.storageEngine,
      data.task, data.test,
      data.threads
    ].join('-')
  }

  // Calculates ratio for given test result `speed` and `baseSpeed` reference
  function ratio(speed, baseSpeed) {
    var ratio = speed / baseSpeed
    if (speed >= 0) {
      return ratio
    } else {
      // Negatives mean latency in ms. Invert the ratio so higher is better.
      return 1 / ratio
    }
  }

  function noBaselineFilter(results) {
    return function (item, id) {
      return results.baseline[id] == undefined
    }
  }

  function isShortItemId(id) {
    return id.length == EVG.PATCH_ID_LEN
  }

  function isLongVersionId(id) {
    return id.indexOf('_') > 0 && id.length > EVG.PATCH_ID_LEN
  }

  function isGitHash(id) {
    return id.length == EVG.GIT_HASH_LEN
  }

  function githashToVersionId(githash) {
    return $window.project.replace('sys-perf', 'sys_perf') + '_' + githash
  }

  // For given `query` attempts to construct valid selctable
  // comparison item. It could be version short/long id, githash
  // Should not be used for tags (all tags are available on ui)
  function getQueryBasedItem(query) {
    var GITHASH = 'h'
    var ID_SHORT = 's'
    var ID_LONG = 'l'

    if (query == undefined) return

    var idKind = (
      isGitHash(query) ? GITHASH :
      isShortItemId(query) ? ID_SHORT :
      isLongVersionId(query) ? ID_LONG : undefined
    )

    var uniformId = (
      idKind == GITHASH ? githashToVersionId(query) :
      _.contains([ID_SHORT, ID_LONG], idKind) ? query : undefined
    )

    // There are not enough information to differentiate
    // version id from patch id
    var kind = PD.KIND_VERSION

    if (uniformId) {
      return {
        id: uniformId,
        name: query,
        kind: kind,
      }
    } else {
      return undefined
    }
  }

  /******************
   * DATA PROCESING *
   ******************/

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
      .each(function (speed, threads) {
        var data = {
          build: ctx.buildName,
          buildId: ctx.buildId,
          buildVariant: ctx.buildVariant,
          task: ctx.taskName,
          taskURL: MPA_UI.TASK_BY_ID({
            task_id: ctx.taskId
          }),
          buildURL: MPA_UI.BUILD_BY_ID({
            build_id: ctx.buildId
          }),
          taskId: ctx.taskId,
          test: item.name,
          threads: +threads,
          speed: speed.ops_per_sec,
          storageEngine: ctx.storageEngine,
        }
        receiver[slug(data)] = data
      })
  }

  // data is [
  //   {
  //     current: <item>,
  //     baseline: <item>,
  //     history: [<item>, ...],
  //     ctx: <context obj>
  //   },
  //   {...}
  // ]
  function onProcessData(data) {
    var now = {},
      baseline = {},
      history = {}

    _.each(data, function (d) {
      // Skip empty items
      if (d == null) return

      // TODO add group validation instead of individual
      // Process current (revision) data
      if (d.current) {
        // Copy storageEngine to the context
        d.ctx.storageEngine = d.current.data.storageEngine || '(none)'

        _.each(d.current.data.results, function (result) {
          processItem(result, now, d.ctx)
        })
      }

      // Process baseline data
      if (d.baseline) {
        _.each(d.baseline.data.results, function (result) {
          processItem(result, baseline, d.ctx)
        })
      }

      // Process history data
      _.each(d.history, function (histItems) {
        // Create an empty 'bucket' for history items if not exists
        var order = histItems.order
        if (history[order] == undefined) {
          history[order] = {}
        }
        _.each(histItems.data.results, function (result) {
          processItem(result, history[order], d.ctx)
        })
      })
    })

    return {
      now: now,
      baseline: baseline,
      // Sorts history items by `order` and returns the list
      history: _.map(
        _.sortBy(_.keys(history)),
        function (key) {
          return history[key]
        }
      ),
    }
  }

  function onGetRows(results) {
    return _.chain(results.now)
      .omit(noBaselineFilter(results))
      .map(function (item, id) {
        var baseline = results.baseline[id]
        var appendix = {
          ratio: ratio(item.speed, baseline.speed),
          baseSpeed: baseline.speed,
        }

        var trendData = _.chain(results.history)
          .take(100)
          .map(function (run) {
            var base = run[id]
            if (base == undefined || !base.speed) {
              return 1
            }
            return ratio(item.speed, base.speed)
          })
          .value()

        // Trend chart data
        appendix.trendData = trendData.concat(appendix.ratio)

        var avgRatio = d3.mean(trendData)
        appendix.avgRatio = avgRatio
        appendix.avgVsSelf = [avgRatio, appendix.ratio]
        return _.extend({}, item, appendix)
      })
      .value()
  }

  // TODO Should be part of `VersionResource`
  // :parm version: version object
  // :returns: Array of {taskId, taskName, buildName}
  function extractTasks(version) {
    return _.reduce(version.builds, function (items, build) {
      // Going over dict of tasks ({taskName: data})
      return items.concat(_.map(build.tasks, function (taskData, taskName) {
        return {
          taskId: taskData.task_id,
          buildId: build.id,
          taskName: taskName,
          buildName: build.name,
          buildVariant: build.variant,
        }
      }))
    }, [])
  }

  function queryBuildData(version) {
    return $q.all(
      _.map(version.build_variants_status, function (d) {
        return ApiV1.getBuildDetail(d.build_id).then(
          respData,
          function (e) {
            return {}
          }
        )
      })
    )
  }

  function tasksOfBuilds(promise) {
    return promise.then(function (builds) {
      return extractTasks({
        builds: builds
      })
    })
  }

  function versionSelectAdaptor(versionsRes) {
    return _.chain(versionsRes.data.versions)
      .where({
        rolled_up: false
      })
      .map(function (d) {
        return {
          kind: PD.KIND_VERSION,
          id: d.versions[0].version_id,
          name: d.versions[0].revision,
        }
      })
      .value()
  }

  function tagSelectAdaptor(tagsRes) {
    return _.map(tagsRes.data, function (d) {
      return {
        kind: PD.KIND_TAG,
        id: d.obj.version_id,
        name: d.name
      }
    })
  }

  // * Group BFs by task, bv and test
  // * Sort BFs in each group by status/date
  // * Mark open BFs by setting `_isOpen` to true -
  //   required for the next step
  //            / GRP 1 \  / GRP 2 \ / GRP N \
  // :returns: [[BF, ...], [BF, ...],  ...   ]
  function postprocessBFs(bfs) {
    return _.chain(bfs)
      // Group all BFs by task, bv and test
      .groupBy(function (bf) {
        return [bf.tasks, bf.buildvariants, bf.tests && ''].join('|')
      })
      // In each group, sort by status/date and mark 'open' items
      .map(function (bfsGroup) {
        return _.chain(bfsGroup)
          // Split Bfs into two category 'Open' and 'not open'
          .partition(function (bf) {
            return _.contains(BF.OPEN_STATUSES, bf.status)
          })
          // Sort BFs in each split by date created
          .map(function (bfs) {
            return _.sortBy(bfs, '-created')
          })
          // Kind of minor optimisation - sets _isOpen to true for
          // each BF which was considered to be open on the first stage
          .each(function (bfs, idx) {
            idx == 0 && _.each(bfs, function (bf) {
              bf._isOpen = true
            })
          })
          // Merging two splits into single array of BFs
          .flatten(true)
          .value()
      })
      .value()
  }

  /******************
   * PROMISE CHAINS *
   ******************/

  // :param rows: list of grid rows
  // :rtype: $q.promise({taskId: {key, link}, ...})
  function getBFTicketsForRows(rows) {
    var uniqueTasks = _.chain(rows)
      .uniq(false, function (d) {
        return d.task
      })
      .pluck('task')
      .value()

    return Stitch.use(STITCH_CONFIG.PERF).query(function (client) {
      return client
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_BUILD_FAILURES)
        .aggregate([{
            $match: {
              tasks: {
                $in: uniqueTasks
              }
            }
          },
          // Denormalization
          {
            $unwind: {
              path: '$tasks',
              preserveNullAndEmptyArrays: true
            }
          },
          {
            $unwind: {
              path: '$buildvariants',
              preserveNullAndEmptyArrays: true
            }
          },
          {
            $unwind: {
              path: '$project',
              preserveNullAndEmptyArrays: true
            }
          },
          {
            $unwind: {
              path: '$tests',
              preserveNullAndEmptyArrays: true
            }
          },
          {
            $unwind: {
              path: '$first_failing_revision',
              preserveNullAndEmptyArrays: true
            }
          },
        ])
    }).then(postprocessBFs)
  }

  // Makes series of HTTP calls and loads curent, baseline and history data
  // :rtype: $q.promise
  function queryData(tasksPromise, baselineTasks) {
    return $q.all({
      tasks: tasksPromise,
      baselineTasks: baselineTasks
    }).then(function (promise) {
      return $q.all(_.map(promise.tasks, function (task) {
        return ApiTaskdata.getTaskById(task.taskId, 'perf')
          .then(respData, function () {
            return null
          })
          .then(function (data) {
            if (data) {
              var baseline = _.findWhere(promise.baselineTasks, {
                buildName: task.buildName,
                taskName: task.taskName,
              })
              // Skip items with no baseline results
              if (!baseline) return null

              return $q.all({
                ctx: $q.resolve(task),
                current: data,
                history: ApiTaskdata.getTaskHistory(task.taskId, 'perf')
                  .then(respData, function (e) {
                    return []
                  }),
                baseline: ApiTaskdata.getTaskById(baseline.taskId, 'perf')
                  .then(respData, function (e) {
                    return null
                  }),
              })
            } else {
              return null
            }
          })
      }))
    })
  }

  // Processes `queryData` return value
  // Populates dicts of current (now, baseline) and history result items
  // returns dict of {now, baseline, history} test result items
  function processData(promise) {
    return promise.then(onProcessData)
  }

  // Returns data for the (ui) grid
  // :param promise: returned by `processData`
  function getRows(promise) {
    return promise.then(onGetRows)
  }

  /**************
   * PUBLIC API *
   **************/

  // Queries list of available options for compare from/to dropdowns
  // :returns: (Promise of) list of versions/tags
  // :rtype: Promise([{
  //    kind: 't(ag)|v(ersion)',
  //    id: 'version_id',
  //    name: 'display name'
  // }, ...])
  function getComparisionOptions(projectId) {
    return $q.all([
      ApiV2.getRecentVersions(projectId).then(versionSelectAdaptor),
      ApiTaskdata.getProjectTags(projectId).then(tagSelectAdaptor)
    ]).then(function (data) {
      return Array.prototype.concat.apply([], data)
    })
  }

  function findVersionItem(items, query) {
    if (query == undefined) return

    return _.find(items, function (d) {
      // Checks if query is inside or equal an id
      // Useful for long verion ids and revisions
      // e.g. $GITHASH will match sys-perf_$GITHASH
      return d.id.indexOf(query) > -1 || query == d.name
    })
  }

  // This function is used to make possible to type arbitrary
  // version revision into version drop down
  function getVersionOptions(items, query) {
    var found = findVersionItem(items, query)
    if (!found) {
      var queryBased = getQueryBasedItem(query)
      if (queryBased) {
        return items.concat(queryBased)
      }
    }
    return items
  }

  function getCompItemVersion(compItem) {
    return ApiV2.getVersionById(compItem.id)
      // Extract version object
      .then(function (res) {
        return res.data
      })
  }

  function getData(version, baselineTag) {
    return getRows(
      processData(
        queryData(
          tasksOfBuilds(queryBuildData(version)),
          tasksOfBuilds(queryBuildData(baselineTag))
        )
      )
    )
  }

  return {
    // public api
    getData: getData,
    getComparisionOptions: getComparisionOptions,
    findVersionItem: findVersionItem,
    getVersionOptions: getVersionOptions,
    getCompItemVersion: getCompItemVersion,
    getQueryBasedItem: getQueryBasedItem,
    getBFTicketsForRows: getBFTicketsForRows,

    // For teting
    _extractTasks: extractTasks,
    _processItem: processItem,
    _onProcessData: onProcessData,
    _onGetRows: onGetRows,
    _versionSelectAdaptor: versionSelectAdaptor,
    _tagSelectAdaptor: tagSelectAdaptor,
  }
})