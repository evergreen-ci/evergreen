describe('ApiUtilTest', function() {
  beforeEach(module('MCI'));

  var apiUtil, $httpBackend

  beforeEach(inject(function($injector) {
    apiUtil = $injector.get('ApiUtil')
    $httpBackend = $injector.get('$httpBackend')
  }))

  it('should append URLs to BASE', function() {
    var BASE_API = 'base/api'
    var get = apiUtil.httpGetter(BASE_API)
    $httpBackend.expectGET(BASE_API + '/test').respond(200)
    get(_.template('{base}/test'))
    $httpBackend.flush()
  })

  it('allows no base URL', function() {
    var get = apiUtil.httpGetter()
    $httpBackend.expectGET('test').respond(200)
    get(_.template('test'))
    $httpBackend.flush()
  })

  it('allows {templating}', function() {
    var get = apiUtil.httpGetter()
    var URL = _.template('{a}/{b}/{c}')
    $httpBackend.expectGET('1/2/3').respond(200)
    get(URL, {a: 1, b: 2, c: 3})
    $httpBackend.flush()
  })

  it('passes HTTP parameters', function() {
    var get = apiUtil.httpGetter()
    $httpBackend.expectGET('test?p=1').respond(200)
    get(_.template('test'), {}, {p: 1})
    $httpBackend.flush()
  })
})
