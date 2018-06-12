/* 
 * Wraps stitch.js with more convenient interface
 * Usage:
 * 
 * Stitch.use(config).query(
 *   (db) -> db./your query/.execute()
 * ).then((docs) -> ...)
 *
 * API:
 * * use(config) - sets config for stitch client.
 *     returns self (this service)
 *     `config` is object (constant) with following required fields:
 *       - appId (e.g. 'evergreen_perf_plugin-wwdoa')
 *       - serviceType (Looks like it always should be 'mongodb')
 *       - serviceName (e.g. 'mongodb-atlas')
 * * query((db) ->) - executes stitch query (db) -> { }
 *     returns promise with list of docs
 * * getDB() - returns stitch db object for given contextual `config`
 *     Kinda internal function and shouldb't be used normally
 *     Caches db objects internally.
 *     TODO might require to have some TTL
 *
 * NOTES:
 * * .use(config) could be called just once - the most recent config
 *   will be stored in the service context
*/
mciModule.factory('Stitch', function(
  $q
) {
  var dbCache = {}

  function dbSlug(config) {
    return config.appId + config.serviceType + config.serviceName
  }

  // Returns stitch `db` object
  // Once initialized, cached value will be returned
  // Most probably, you shouldn't use this fn (see `query` fn)
  function getDB() {
    var self = this

    // Check if config is set
    if (self.config === undefined) {
      return $q.reject(
        new Error(
          'No config provided! Usage: Stitch.use(config).query((db) -> {}).then(...)'
        )
      )
    }

    // Check if all required values exists in the config
    if (_.any(['appId', 'serviceType', 'serviceName'], function(d) {
        return self.config[d] === undefined
    })) {
      return $q.reject(
        new Error(
          'Config is invalid! Should have appId, serviceType and serviceName params'
        )
      )
    }

    var slug = dbSlug(self.config)

    // return cached db value
    if (dbCache[slug]) { return $q.resolve(dbCache[slug]).promise }

    var client = stitch.StitchClientFactory.create(self.config.appId)

    return $q.when(
      client.then(function(client) {
        dbCache[slug] = client.service(self.config.serviceType, self.config.serviceName)
        return client.login()
      })
    ).then(function() { return dbCache[slug] })
  }
  
  // Executes stitch query
  // :param dbQuery: (db) -> db.operations(...)
  function query(dbQuery) {
    var self = this
    return $q(function(resolve, reject) {
      self.getDB().then(function(db) {
        dbQuery(db).then(
          function(ret) { resolve(ret) },
          function(err) { reject(err) }
        )
      }, function(err) { reject(err) })
    })
  }

  function use(config) {
    this.config = config
    return this
  }

  return {
    use  : use,
    query: query,
    getDB: getDB,
  }
})
