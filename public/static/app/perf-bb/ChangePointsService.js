mciModule.factory('ChangePointsService', function(
  $log, $mdDialog, PROCESSED_TYPE, Stitch, STITCH_CONFIG
) {
  var conn = Stitch.use(STITCH_CONFIG.PERF)

  function dbMarkPoints(points, mark) {
    // Mark change points as 'processed' by setting non-emty processed_type
    conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_CHANGE_POINTS)
        .updateMany({task_id: {$in: _.pluck(points, 'task_id')}}, {$set: {processed_type: mark}})
    })

    // Put items to prcoessed list
    conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS)
        // TODO Automate $$hashkey removal
        .insertMany(_.map(points, function(d) { return _.omit(d,'$$hashKey') }))
    })
    .catch(function(err) { // recover an duplication error
      $log.warn(err)
      return true
    })
    .finally(function() {
      dbChangeExistingMark(points, mark)
    })
  }

  function dbChangeExistingMark(points, mark) {
    conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS)
        .updateMany({task_id: {$in: _.pluck(points, 'task_id')}}, {$set: {processed_type: mark}})
    })
  }

  function dbUnmarkPoints(points) {
    // Mark change points as 'processed' by setting non-emty processed_type
    conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_CHANGE_POINTS)
        .updateMany({task_id: {$in: _.pluck(points, 'task_id')}}, {$set: {processed_type: undefined}})
    })

    // Put items to prcoessed list
    conn.query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS)
        .deleteMany({task_id: {$in: _.pluck(points, 'task_id')}})
    })
  }

  function dbDispatchMarkPoints(points, mark, mode) {
    if (!points || !points.length) return
    // Dispatching
    if (mark == undefined) {
      dbUnmarkPoints(points)
    } else if (_.contains(PROCESSED_TYPE.ALL, mark)) {
      if (mode == 'processed') {
        dbChangeExistingMark(points, mark)
      } else {
        dbMarkPoints(points, mark)
      }
    }
  }

  function confirmMarkAction(points) {
    return $mdDialog.show(
      $mdDialog.confirm()
        .ok('Ok')
        .cancel('Cancel')
        .title('Confirm')
        .textContent('Modify ' + points.length + ' item(s)?')
    )
  }

  // Sets processed_type for the change points
  // :param points: list of change point objects
  // :param mark: PROCESSED_TYPE
  // :param mode: processed|unprocessed
  function markPoints(points, mark, mode) {
    return confirmMarkAction(points).then(function(r) {
      // Apply change locally
      _.patch(points, {processed_type: mark})
      // Propagate changes to the db
      dbDispatchMarkPoints(points, mark, mode)
      return true
    }, _.noop)
  }

  return {
    markPoints: markPoints,
  }
})
