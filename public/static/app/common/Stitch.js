mciModule.factory('Stitch', function(
  $q, STITCH_CONFIG
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
