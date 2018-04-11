describe('PerfDiscoveryDataServiceTest', function() {
  beforeEach(module('MCI'));

  var githash = '1234567890123456789012345678901234567890'
  var project = 'proj'

  var service, $httpBackend, $window, PD

  beforeEach(function() {
    module(function($provide) {
      $window = {project: project}
      $provide.value('$window', $window)
    })

    inject(function($injector) {
      service = $injector.get('PerfDiscoveryDataService')
      $httpBackend = $injector.get('$httpBackend')
      PD = $injector.get('PERF_DISCOVERY')
    })
  })

  it('should extract tasks from version', function() {
    var version = {
      builds: {
        buildA: {
          id: 'baid',
          name: 'buildAName',
          tasks: { taskA: {task_id: 'idA'}, taskB: {task_id: 'idB'}, } },
        buildB: {
          id: 'bbid',
          name: 'buildBName',
          tasks: { taskC: {task_id: 'idC'}, taskD: {task_id: 'idD'}, } } } }

    expect(
      service._extractTasks(version)
    ).toEqual([{
      buildId: 'baid',
      taskId: 'idA',
      taskName: 'taskA',
      buildName: 'buildAName',
    }, {
      buildId: 'baid',
      taskId: 'idB',
      taskName: 'taskB',
      buildName: 'buildAName',
    }, {
      buildId: 'bbid',
      taskId: 'idC',
      taskName: 'taskC',
      buildName: 'buildBName',
    }, {
      buildId: 'bbid',
      taskId: 'idD',
      taskName: 'taskD',
      buildName: 'buildBName',
    }])
})

  it('processes single data item', function() {
    var item = {
      name: 'name',
      results: {
        8: {ops_per_sec: 100},
        16: {ops_per_sec: 200},
      }
    }
    var ctx = {
      buildName: 'b-wt', taskName: 't', taskId: 'tid', buildId: 'bid'
    }
    var receiver = {}

    service._processItem(item, receiver, ctx, 'wt')
    expect(
      receiver
    ).toEqual({
      'b-wt-wt-t-name-8': {
        build: 'b-wt',
        task: 't',
        buildURL: '/build/bid',
        taskURL: '/task/tid',
        storageEngine: 'wt',
        test: 'name',
        threads: 8,
        speed: 100,
      },
      'b-wt-wt-t-name-16': {
        build: 'b-wt',
        task: 't',
        buildURL: '/build/bid',
        taskURL: '/task/tid',
        storageEngine: 'wt',
        test: 'name',
        threads: 16,
        speed: 200,
      },
    })
  })

  it('processes the data', function() {
    var data = [{
      current: {
        data: {
          results: [{
            name: 'test',
            results: {
              8: {ops_per_sec: 100}}}
          ]}},
      baseline: {
        data: {
          results: [{
            name: 'test',
            results: {
              8: {ops_per_sec: 100}}}
          ]}},
      history: [{
        order: 1,
        data: {
          results: [{
            name: 'test',
            results: {8: {ops_per_sec: 100}}}
          ]}}
      ],
      ctx: {buildName: 'b', taskName: 't'}
    }]

    var processed = service._onProcessData(data)

    expect(
      _.keys(processed.now).length
    ).toBe(1)

    expect(
      _.keys(processed.baseline).length
    ).toBe(1)

    expect(
      processed.history[0]
    ).toBeDefined()

    expect(
      _.keys(processed.history[0]).length
    ).toBe(1)
  })

  it('prcocesses the empty data', function() {
    var data = [{
      current: null,
      baseline: null,
      history: [],
      ctx: {buildName: 'b', taskName: 't'}
    }]

    expect(
      service._onProcessData(data)
    ).toEqual({
      now: {},
      baseline: {},
      history: [],
    })
  })

  it('processes null items', function() {
    var data = [null]

    expect(
      service._onProcessData(data)
    ).toEqual({
      now: {},
      baseline: {},
      history: [],
    })
  })

  it('convert test data to row items', function() {
    var results = {
      now: {
        'b-wt-t-name-8': {
          build: 'b',
          task: 't',
          storageEngine: 'wt',
          test: 'name',
          threads: 8,
          speed: 100 } },
      baseline: {
        'b-wt-t-name-8': {
          build: 'b',
          task: 't',
          storageEngine: 'wt',
          test: 'name',
          threads: 8,
          speed: 200 } },
      history: [{
          'b-wt-t-name-8': {
            build: 'b',
            task: 't',
            storageEngine: 'wt',
            test: 'name',
            threads: 8,
            speed: 400 }
        }, {
          'b-wt-t-name-8': {
            build: 'b',
            task: 't',
            storageEngine: 'wt',
            test: 'name',
            threads: 8,
            speed: 50 } },
      ]
    }

    expect(
      service._onGetRows(results)
    ).toEqual([{
      build: 'b',
      task: 't',
      storageEngine: 'wt',
      test: 'name',
      threads: 8,
      speed: 100,
      baseSpeed: 200,
      ratio: 0.5,
      trendData: [0.25, 2, 0.5],
      avgVsSelf: [1.125, 0.5],
      avgRatio: 1.125,
    }])

  })

  it('Extracts versions from the response', function() {
    var resp = {
      data: {
        versions: [{
          rolled_up: true,
        }, {
          rolled_up: false,
          versions: [{
            version_id: 'v_id',
            revision: 'v_rev',
          }]
        }]
      }
    }

    expect(
      service._versionSelectAdaptor(resp)
    ).toEqual([{
      kind: PD.KIND_VERSION,
      id: 'v_id',
      name: 'v_rev',
    }])
  })

  it('Extracts versions from the response', function() {
    var resp = {
      data: [{
        name: 't_name',
        obj: { version_id: 'v_id' },
      }]
    }

    expect(
      service._tagSelectAdaptor(resp)
    ).toEqual([{
      kind: PD.KIND_TAG,
      id: 'v_id',
      name: 't_name',
    }])
  })

  it('Finds tag/version in items', function() {
    var item1 = {id: 'id1', name: 'name1'}
    var item2 = {id: 'id2', name: 'name2'}
    var item3 = {id: 'sys-perf_githash', name: 'name2'}
    var items = [item1, item2, item3]

    expect(
      service.findVersionItem(items, 'id1')
    ).toBe(item1)

    expect(
      service.findVersionItem(items, 'id2')
    ).toBe(item2)

    expect(
      service.findVersionItem(items, 'id3')
    ).toBeUndefined()

    expect(
      service.findVersionItem(items, 'name2')
    ).toBe(item2)

    expect(
      service.findVersionItem(items, 'githash')
    ).toBe(item3)
  })

  it('Adds query based item to comp items', function() {
    var LEN24_A = '1234567890123456789012_A'
    var LEN24_B = '1234567890123456789012_B'
    var item1 = {id: LEN24_A, name: 'name1'}
    var item2 = {id: 'id2', name: 'name2'}
    var items = [item1, item2]

    expect(
      service.getVersionOptions(items, 'noitem')
    ).toBe(items)

    expect(
      service.getVersionOptions(items, 'id1')
    ).toBe(items)

    expect(
      service.getVersionOptions(items, LEN24_A)
    ).toBe(items)

    expect(
      service.getVersionOptions(items, LEN24_B)
    ).toEqual(items.concat({
      kind: PD.KIND_VERSION,
      id: LEN24_B,
      name: LEN24_B,
    }))

    expect(
      service.getVersionOptions(items, githash)
    ).toEqual(items.concat({
      kind: PD.KIND_VERSION,
      id: project + '_' + githash,
      name: githash,
    }))
  })

  it('attempts to build a comp item from query string', function() {
    var LEN24 = '123456789012345678901234'
    var versionIdLong = project + '_' + githash

    expect(
      service.getQueryBasedItem('tag name')
    ).toBeUndefined()

    expect(
      service.getQueryBasedItem(LEN24)
    ).toEqual({kind: PD.KIND_VERSION, id: LEN24, name: LEN24})

    expect(
      service.getQueryBasedItem(versionIdLong)
    ).toEqual({
      kind: PD.KIND_VERSION, id: versionIdLong, name: versionIdLong
    })

    expect(
      service.getQueryBasedItem(githash)
    ).toEqual({
      kind: PD.KIND_VERSION, id: versionIdLong, name: githash
    })
  })
})
