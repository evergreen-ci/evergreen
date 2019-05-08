/*
 * This service contains functions which can access the points collection.
 */
mciModule.factory('OutliersDataService', function($log, Stitch, STITCH_CONFIG) {
  // Get a list of rejected points.
  // Destructuring http://exploringjs.com/es6/ch_destructuring.html
  const getOutliersQ = (project, {variant, task, test, revision, type = 'detected'}) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      // Remove Undefined values, null and false are acceptable.
      const query  = _.omit({
        revision:revision,
        project: project,
        variant: variant,
        task: task,
        test: test,
        type : type
      }, _.isUndefined);

      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_OUTLIERS)
        .find(query)
        .execute();
    }).then((docs) => docs,
      err => {
        // Try to gracefully handle an error.
        $log.error('Cannot load outliers!', err);
        return [];
      });
  };

  const getMarkedOutliersQ = (project, {variant, task, test, revision}) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      // Remove Undefined values, null and false are acceptable.
      const query  = _.omit({
        revision:revision,
        project: project,
        variant: variant,
        task: task,
        test: test,
      }, _.isUndefined);

      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_MARKED_OUTLIERS)
        .find(query)
        .execute();
    }).then((docs) => docs,
      err => {
        // Try to gracefully handle an error.
        $log.error('Cannot load marked outliers!', err);
        return [];
      });
  };

  const aggregateQ = (pipeline) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_OUTLIERS)
        .aggregate(pipeline);
    });
  };

  return {
    getOutliersQ: getOutliersQ,
    getMarkedOutliersQ: getMarkedOutliersQ,
    aggregateQ: aggregateQ,
  }
});
