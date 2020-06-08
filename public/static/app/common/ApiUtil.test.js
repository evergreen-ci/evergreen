describe('ApiUtilTest', function() {
  beforeEach(module('MCI'));

  var apiUtil, $httpBackend;

  beforeEach(inject(function($injector) {
    apiUtil = $injector.get('ApiUtil');
    $httpBackend = $injector.get('$httpBackend')
  }));

  it('should append URLs to BASE', function() {
    var BASE_API = 'base/api';
    var get = apiUtil.httpGetter(BASE_API);
    $httpBackend.expectGET(BASE_API + '/test').respond(200);
    get('test');
    $httpBackend.flush()
  });

  it('allows no base URL', function() {
    var get = apiUtil.httpGetter();
    $httpBackend.expectGET('/test').respond(200);
    get('test');
    $httpBackend.flush()
  });

  it('allows / base URL', function() {
    var get = apiUtil.httpGetter('/');
    $httpBackend.expectGET('/test').respond(200);
    get('test');
    $httpBackend.flush()
  });

  it('allows no base URL and allows do / reqs', function() {
    var get = apiUtil.httpGetter();
    $httpBackend.expectGET('/test').respond(200);
    get('/test');
    $httpBackend.flush()
  });

  it('allows {templating}', function() {
    var get = apiUtil.httpGetter();
    var URL = '{a}/{b}/{c}';
    $httpBackend.expectGET('/1/2/3').respond(200);
    get(URL, {a: 1, b: 2, c: 3});
    $httpBackend.flush()
  });

  it('passes HTTP parameters', function() {
    var get = apiUtil.httpGetter();
    $httpBackend.expectGET('/test?p=1').respond(200);
    get('test', {}, {p: 1});
    $httpBackend.flush()
  })

  it('should append URLs to BASE POST', function() {
    var BASE_API = 'base/api';
    var post = apiUtil.httpPoster(BASE_API);
    $httpBackend.expectPOST(BASE_API + '/test').respond(200);
    post('test');
    $httpBackend.flush()
  });

  it('allows no base URL POST', function() {
    var post = apiUtil.httpPoster();
    $httpBackend.expectPOST('/test').respond(200);
    post('test');
    $httpBackend.flush()
  });

  it('allows / base URL POST', function() {
    var post = apiUtil.httpPoster('/');
    $httpBackend.expectPOST('/test').respond(200);
    post('test');
    $httpBackend.flush()
  });

  it('allows no base URL and allows do / reqs POST', function() {
    var post = apiUtil.httpPoster();
    $httpBackend.expectPOST('/test').respond(200);
    post('/test');
    $httpBackend.flush()
  });

  it('allows {templating} POST', function() {
    var post = apiUtil.httpPoster();
    var URL = '{a}/{b}/{c}';
    $httpBackend.expectPOST('/1/2/3').respond(200);
    post(URL, {a: 1, b: 2, c: 3});
    $httpBackend.flush()
  });
});
