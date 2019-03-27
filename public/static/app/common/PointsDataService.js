/*
 * This service contains functions which can access the points collection.
 */
if (_.isUndefined(_.isSet)) {
  _.mixin({
    isSet: function (obj) {
      return toString.call(obj) === '[object Set]';
    }
  });
}

mciModule.factory('PointsDataService', function ($log, Stitch, STITCH_CONFIG) {
  // Get a list of rejected points.
  const rejectedMatcher = _.matcher({outlier: true, rejected: true});

  const getOutlierPointsQ = (project, variant, task, test) => {
    test = test || new stitch.BSON.BSONRegExp('^(fio|canary|iperf|NetworkBandwidth)');
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      const query = {
        project: project,
        variant: variant,
        task: task,
        test: test,
        $or: [{"outlier": true}, {"results.outlier": true}]
      };
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_POINTS)
        .find(query)
        .execute();
    }).then(
      docs => _.chain(docs)
        .reduce((memo, point) => {
          const task_id = point.task_id;
          const test = point.test;
          const rejected = _.chain(point.results).map(rejectedMatcher).push(rejectedMatcher(point)).some().value();

          if (rejected) {
            memo.rejects.add(task_id);
          }

          let outlierThreadLevels = _.chain(point.results)
            .map()
            .push({outlier: point.outlier, thread_level: 'max'})
            .where({outlier: true})
            .pluck('thread_level')
            .map(thread_level => ({test: test, thread_level: thread_level}))
            .value();

          if (_.has(memo.outliers, task_id)) {
            memo.outliers[task_id] = memo.outliers[task_id].concat(outlierThreadLevels);
          } else {
            memo.outliers[task_id] = outlierThreadLevels;
          }
          return memo
        }, {rejects: new Set(), outliers: {}})
        .map((value, key) => [key, _.isSet(value) ? Array.from(value) : value])
        .object()
        .value(),
      //.mapObject((value) => Array.from(value)),
      err => {
        // Try to gracefully handle an error.
        $log.error('Cannot load outliers and rejected points!', err);
        return {rejects: [], outliers: {}};
      });
  };

  return {
    getOutlierPointsQ: getOutlierPointsQ,
  }
});
