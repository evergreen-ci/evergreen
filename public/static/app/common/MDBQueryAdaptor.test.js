describe('MDBQueryAdaptorSpec', function() {
  beforeEach(module('MCI'));

  var svc

  beforeEach(inject(function($injector) {
    svc = $injector.get('MDBQueryAdaptor')
  }))

  it('condenses query string', function() {
    expect(
      svc._condense('  a   \tb c ')
    ).toBe('abc')
  })

  it('tokenizes num field filtering query', function() {
    var cases = [
      // Input / Output
      ['qry', ['qry']],
      ['>qry', ['>', 'qry']],
      ['<qry', ['<', 'qry']],
      ['>=qry', ['>', '=', 'qry']],
      ['<=qry', ['<', '=', 'qry']],
      // Behavior of this is undefined - should not fail
      ['<=<=', ['<', '=', '<=']],
      ['<<<<', ['<', '<<<']],
    ]

    _.each(cases, function(c) {
      expect(svc._numTypeTokenizer(c[0])).toEqual(c[1])
    })
  })

  it('tokenizes str field filtering query', function() {
    expect(svc._strTypeTokenizer('query')).toEqual(['query'])
  })

  it('parses num field filtering tokens', function() {
    var cases = [
      // Input / Output
      [['1'], {op: '==', term: 1}],
      [['>', '1'], {op: '>', term: 1}],
      [['>', '=', '1'], {op: '>=', term: 1}],
      // Invalid input
      [['>'], undefined],
      [['qry'], undefined],
      [['>', '>'], undefined],
      // Not realistic scenario (undefined behavior)
      [['qry', '>'], undefined],
    ]

    _.each(cases, function(c) {
      expect(svc._numTypeParser(c[0])).toEqual(c[1])
    })
  })

  it('parses str field filtering tokens (icontains)', function() {
    expect(
      svc._strTypeParser(['query'])
    ).toEqual({
      op: 'icontains',
      term: 'query',
    })
  })

  it('parses str field filtering tokens (exact)', function() {
    expect(
      svc._strTypeParser(['=', 'query'])
    ).toEqual({
      op: '==',
      term: 'query',
    })
  })

  it('compiles single filtering query', function() {
    expect(
      svc._predicateCompiler('fld')({op: '==', term: 10})
    ).toEqual({fld: {$eq: 10}})
  })

  it('compiles restricted query language to mdb query', function() {
    expect(
      svc.compileFiltering([
        {type: 'number', field: 'a', term: '10'},
        {type: 'number', field: 'b', term: '>5'},
        {type: 'string', field: 'c', term: 'term'},
      ])
    ).toEqual({$match: {
      a: {$eq: 10},
      b: {$gt: 5},
      c: {$regex: 'term', $options: 'i'},
    }})
  })

  it('compiles sorting', function() {
    expect(
      svc.compileSorting([{
        field: 'fld',
        direction: 'asc'
      }])
    ).toEqual({$sort: {fld: 1}})
  })
})
