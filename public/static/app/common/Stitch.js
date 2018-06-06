mciModule.factory('Stitch', function(
  $q
) {
  var db // cached, initialized on demand

  // Returns stitch `db` object
  // Once initialized, cached value will be returned
  // Most probably, you shouldn't use this fn (see `query` fn)
  function getDB() {
    // return cached db value
    if (db) { return $q.resolve(db).promise }

    var client = stitch.StitchClientFactory.create('evergreen_perf_plugin-wwdoa')

    return $q.when(
      client.then(function(client) {
        db = client.service('mongodb', 'mongodb-atlas').db('perf')
        return client.login()
      })
    ).then(function() { return db })
  }
  
  // Executes stitch query
  // :param dbQuery: (db) -> db.operations(...)
  function query(dbQuery) {
    return $q(function(resolve, reject) {
      getDB().then(function(db) {
        dbQuery(db).then(
          function(ret) { resolve(ret) },
          function(err) { reject(err) }
        )
      })
    })
  }

  return {
    query: query,
    getDB: getDB,
  }
})
