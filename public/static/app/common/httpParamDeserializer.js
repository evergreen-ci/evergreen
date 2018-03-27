// Deserializes http params (?a=1&b=2) into JS object ({a: 1, b: 2})
// Compatible with Anguarjs $httpParamSerializer
mciModule.factory('httpParamDeserializer', function($log) {
  // Parses single http param string (e.g. 'a=1')
  // Returns array [key, value]
  function paramExtractor(paramBit) {
    var values = paramBit.split('=')
    if (values.length != 2) {
      $log.warn('Invalid query param: ' + paramBit)
      return undefined
    }
    return values
  }

  // Reduces list of params [[key, value], ...]
  // into obj {key: [value, ...], ...}
  // 'params' expected to be Object
  function paramReducer(params, d) {
    var key = d[0]
    var val = d[1]
    var existing = params[key]

    if (existing !== undefined) {
      params[key].push(val)
    } else {
      params[key] = [val]
    }

    return params
  }

  // The final step, which makes this deserializer compatible
  // with AngularJS $httpParamSerealizer
  // gets rid of an array wrapper for params with single value
  // e.g. {a: ['v']} to {a: 'v'}
  // Do nothing to multivalue params like {a: ['v1', 'v2']}
  function denormalize(val, key) {
    return val.length == 1
      ? val[0]
      : val
  }

  return function(paramString) {
    var bits = decodeURIComponent(paramString).split('&')
    return _.chain(bits)
      .map(paramExtractor) // Convert a=1 to ['a', '1']
      .filter() // Filter out invalid (blank) params
      .reduce(paramReducer, {}) // Convert param list to obj
      .mapObject(denormalize) // Use value as is, when it is not an array
      .value()
  }
})
