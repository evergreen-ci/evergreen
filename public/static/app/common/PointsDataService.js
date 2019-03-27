/*
 * This service contains functions which can access the points collection.
 */
mciModule.factory('PointsDataService', function($log, Stitch, STITCH_CONFIG) {
  // Get a list of rejected points.
  const getRejectedPointsQ = (project, variant, task, test) => {
    test = test ? test : new stitch.BSON.BSONRegExp('^(fio|canary|iperf|NetworkBandwidth)');
    return Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
      const query = {
        project: project,
        variant: variant,
        task: task,
        test: test,
        $or : [
          {'$and':[{ "rejected" : true }, { "outlier" : true }]},
          {'$and':[{ "results.rejected" : true }, { "results.outlier" : true }]}]
      };
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_POINTS)
        .find(query)
        .execute();
    }).then(
        function(docs) {
          return _.chain(docs).pluck('task_id').uniq().value();
        }, function(err) {
          // Try to gracefully handle an error.
          $log.error('Cannot load rejected points!', err);
          return [];
        })
  } ;

  return {
    getRejectedPointsQ: getRejectedPointsQ,
  }
});
