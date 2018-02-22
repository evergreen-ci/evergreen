describe('PerfDiscoveryServiceTest', function() {
  beforeEach(module('MCI'));

  var service, $httpBackend

  beforeEach(inject(function($injector) {
    service = $injector.get('PerfDiscoveryService')
    $httpBackend = $injector.get('$httpBackend')
  }))

  it('should extract tasks from version', function() {
    var version = {
      builds: {
        buildA: {
          name: 'buildAName',
          tasks: { taskA: {task_id: 'idA'}, taskB: {task_id: 'idB'}, } },
        buildB: {
          name: 'buildBName',
          tasks: { taskC: {task_id: 'idC'}, taskD: {task_id: 'idD'}, } } } }

    expect(
      service._extractTasks(version)
    ).toEqual([{
      taskId: 'idA',
      taskName: 'taskA',
      buildName: 'buildAName',
    }, {
      taskId: 'idB',
      taskName: 'taskB',
      buildName: 'buildAName',
    }, {
      taskId: 'idC',
      taskName: 'taskC',
      buildName: 'buildBName',
    }, {
      taskId: 'idD',
      taskName: 'taskD',
      buildName: 'buildBName',
    }])
  })

  it('extracts storageEngine from build name', function() {
    expect(
      service._extractStorageEngine('build-wt', 'task')
    ).toEqual({
      build: 'build',
      task: 'task',
      storageEngine: 'wt',
    })
  })

  it('extracts storageEngine from task name', function() {
    expect(
      service._extractStorageEngine('build', 'task-wiredtiger')
    ).toEqual({
      build: 'build',
      task: 'task',
      storageEngine: 'wiredtiger',
    })
  })

  it('handles no storageEngine case', function() {
    expect(
      service._extractStorageEngine('build', 'task')
    ).toEqual({
      build: 'build',
      task: 'task',
      storageEngine: '(none)',
    })
  })

  it('processes single data item', function() {
    var item = {
      name: 'name',
      results: {
        8: {ops_per_sec: 100},
        16: {ops_per_sec: 200},
      }
    }
    var ctx = { buildName: 'b-wt', taskName: 't' }
    var receiver = {}

    service._processItem(item, receiver, ctx)
    expect(
      receiver
    ).toEqual({
      'b-wt-t-name-8': {
        build: 'b',
        task: 't',
        storageEngine: 'wt',
        test: 'name',
        threads: 8,
        speed: 100,
      },
      'b-wt-t-name-16': {
        build: 'b',
        task: 't',
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
      trendData: [0.25, 2],
      avgVsSelf: [1.125, 0.5]
    }])

  })
})
