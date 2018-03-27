describe('PerfDiscoveryServiceTest', function() {
  beforeEach(module('MCI'))

  var deserialize, serealize

  beforeEach(inject(function($injector) {
    deserialize = $injector.get('httpParamDeserializer')
    serealize = $injector.get('$httpParamSerializer')
  }))

  it('deserializes one param', function() {
    var params = {a: '1'}
    expect(
      deserialize(serealize(params))
    ).toEqual(params)
  })

  it('deserializes two params', function() {
    var params = {a: '1', b: '2'}
    expect(
      deserialize(serealize(params))
    ).toEqual(params)
  })

  it('deserializes two params; one is an array', function() {
    var params = {a: ['1', '3'], b: '2'}
    expect(
      deserialize(serealize(params))
    ).toEqual(params)
  })

  it('deserializes two array params', function() {
    var params = {a: ['1', '3'], b: ['4', '2']}
    expect(
      deserialize(serealize(params))
    ).toEqual(params)
  })

  it('handles invalid params (no =)', function() {
    expect(deserialize('abc')).toEqual({})
  })

  it('handles invalid params (multiple =)', function() {
    expect(deserialize('a=b=c')).toEqual({})
  })

  it('handles empty params srting', function() {
    expect(deserialize('')).toEqual({})
  })
})
