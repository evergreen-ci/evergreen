/*
 * This service contains functions which can access the points collection.
 */
mciModule.factory('OutliersDataService', function($log, Stitch, STITCH_CONFIG) {
  // Get a list of rejected points.
  // Destructuring http://exploringjs.com/es6/ch_destructuring.html
  const getOutliersQ = (project, {variant, task, test, revision, type = 'detected'}) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      // The following omit call will remove keys with Undefined values.
      // Keys with null and false are acceptable.
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

  // Get a list of marked outliers.
  // :param query: The query to evaluate.
  // :return: A list of matching documents. On error an empty list is returned.
  const getMarkedOutliersQ = (query) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
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

  // Add a mark for a single outlier matches the mark identifier.
  // A mark identifier is revision, project, variant ,task , test, thread_level.
  // :param query: The query to evaluate.
  // :return: A list of matching documents. On error an empty list is returned.
  const addMark = (mark_identifier, mark) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_MARKED_OUTLIERS)
        .updateOne(mark_identifier,
          {
            $setOnInsert: mark
          },
          { upsert: true }
        );
    }).then(result => {
      const { matchedCount, modifiedCount, upsertedId } = result;
      if(!upsertedId && !modifiedCount) {
        if(matchedCount) {
          $log.warn('Outlier was already marked.');
        } else {
          $log.warn('No outlier was matched.');
        }
      }
      return mark_identifier;
    }, err => {
      $log.error(err);
      return undefined;
    });
  };

  // Remove a mark for a single outlier matches the mark identifier.
  // A mark identifier is revision, project, variant ,task ,test, thread_level.
  // :param mark_identifier: The mark_identifier to evaluate.
  // :return: The mark_identifier for the document matched, null is returned on no match. On error undefined is returned.
  const removeMark = (mark_identifier) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_MARKED_OUTLIERS)
        .deleteOne(mark_identifier);
    }).then(result => {
      const { deletedCount } = result;
      if(!deletedCount) {
        $log.warn('Successfully remove marks.');
      }
      return mark_identifier;
    }, err => {
      $log.error(err);
      return undefined;
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
    addMark: addMark,
    removeMark: removeMark,
    aggregateQ: aggregateQ,
  }
});
