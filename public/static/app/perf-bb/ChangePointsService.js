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
      conn.query(function(db) {
        return db
          .db(STITCH_CONFIG.PERF.DB_PERF)
          .collection(STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS)
          .updateMany({task_id: {$in: _.pluck(points, 'task_id')}}, {$set: {processed_type: mark}})
      })
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
        .updateMany({task_id: {$in: _.pluck(points, 'task_id')}})
    })
  }

  function dbDispatchMarkPoints(points, mark) {
    if (!points || !points.length) return
    // Dispatching
    if (mark == undefined) {
      dbUnmarkPoints(points)
    } else if (_.contains(PROCESSED_TYPE.ALL, mark)) {
      dbMarkPoints(points, mark)
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

  function markPoints(points, mark) {
    return confirmMarkAction(points).then(function(r) {
      // Apply change locally
      _.patch(points, {processed_type: mark})
      // Propagate changes to the db
      dbDispatchMarkPoints(points, mark)
      return true
    }, _.noop)
  }

  return {
    markPoints: markPoints,
  }
})
