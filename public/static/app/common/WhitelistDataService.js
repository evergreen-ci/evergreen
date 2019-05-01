mciModule.factory('WhitelistDataService', function(
  $log, $mdDialog, PROCESSED_TYPE, Stitch, STITCH_CONFIG, $q
) {
  const conn = Stitch.use(STITCH_CONFIG.PERF);

  // Adds a whitelist entry for task
  // :param task_revisions: list of task_revisions, containing 'revision', 'project', 'variant', 'task' values.
  function addWhitelist(task_revisions) {
    return conn.query(function(db) {
      const outliers_collection = db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_WHITELISTED_OUTLIERS);

      const promises = task_revisions.map(function(task_revision){
        const query = _.pick(task_revision, 'revision', 'project', 'variant', 'task');
        return outliers_collection.updateOne(query, task_revision, {upsert: true});
      });
      return $q.all(promises);
    });
  }

  // Removes whitelist entries for tasks
  // :param task_revisions: list of task_revisions, containing 'revision', 'project', 'variant', 'task' values.
  function removeWhitelist(task_revisions) {
    const query = {$or: task_revisions.map((task_revision) => _.pick(task_revision, 'revision', 'project', 'variant', 'task'))};
    return conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_WHITELISTED_OUTLIERS)
        .deleteMany(query);
    });
  }

  // Get the whitelist as a promise
  // :param query: The query to evaluate.
  // :param projection: Specify the fields in the returned documents.
  const getWhitelistQ = (query, projection) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      // Remove Undefined values, null and false are acceptable.
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_WHITELISTED_OUTLIERS)
        .find(query, projection)
        .execute();
    }).then((docs) => {
        return docs
      },
      err => {
        // Try to gracefully handle an error.
        $log.error('Cannot load outliers!', err);
        return [];
      });
  };

  return {
    addWhitelist: addWhitelist,
    removeWhitelist: removeWhitelist,
    getWhitelistQ:getWhitelistQ,
  };
});
