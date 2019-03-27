/*
 * This service contains functions which can access the points collection.
 */
mciModule.factory('OutliersDataService', function($log, Stitch, STITCH_CONFIG) {
  // Get a list of rejected points.
  // Destructuring http://exploringjs.com/es6/ch_destructuring.html
  const getOutliersQ = (project, {variant, task, test, type = 'detected'}) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      // Remove Undefined values, null and false are acceptable.
      const query  = _.omit({
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

  return {
    getOutliersQ: getOutliersQ,
  }
});
