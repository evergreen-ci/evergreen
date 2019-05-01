mciModule.factory('WhitelistService', function(
  $log, $mdDialog, PROCESSED_TYPE, Stitch, STITCH_CONFIG, $q
) {
  const conn = Stitch.use(STITCH_CONFIG.PERF);

  // Sets processed_type for the change points
  // :param points: list of change point objects
  // :param mark: PROCESSED_TYPE
  // :param mode: processed|unprocessed
  function addWhitelist(task_revisions) {
    return conn.query(function(db) {
      const outliers_collection = db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_WHITELISTED_OUTLIERS);

      const promises = task_revisions.map(function(task_revision){
        const document = _.pick(task_revision, 'revision', 'project', 'variant', 'task');
        return outliers_collection.updateOne({_id:document}, document, {upsert:true});
      });
      return $q.all(promises);
    })

  }

  function removeWhitelist(task_revisions) {
    const query = {_id: {$in: task_revisions.map((task_revision) => _.pick(task_revision, 'revision', 'project', 'variant', 'task'))}};
    return conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_WHITELISTED_OUTLIERS)
        .deleteMany(query);
    })
  }

  function confirmMarkAction(task_revisions, action) {
    return $mdDialog.show(
      $mdDialog.confirm()
        .ok('Ok')
        .cancel('Cancel')
        .title('Confirm')
        .textContent(action + ' ' + task_revisions.length + ' item(s)?')
    );
  }

  const getWhitelistQ = (project, {variant, task, test, revision}) => {
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
        .collection(STITCH_CONFIG.PERF.COLL_WHITELISTED_OUTLIERS)
        .find(query)
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
    addWhitelist: _.wrap(addWhitelist, function(func, task_revisions) {
      const promise =  confirmMarkAction(task_revisions, 'Add whitelisting for').
                            then(() => func(task_revisions));
      // return promise.then(() => true);
      return promise.then(function(results){
        $log.debug('addWhitelist', results);
        return true;
      });
    }),
    removeWhitelist: _.wrap(removeWhitelist, function(func, task_revisions) {
      const promise =  confirmMarkAction(task_revisions, 'Remove whitelisting for').
                          then(() => func(task_revisions));
      return promise.then(function(results){
        $log.debug('removeWhitelist', results);
        return true;
      });
    }),
    getWhitelistQ:getWhitelistQ,
  }
});
