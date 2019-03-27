describe('PointsDataServiceSpec', function() {
  beforeEach(module('MCI'));
  function BSONRegExp(test) {
    return test;
  }

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';
  const test_name = ' distinct_types_no_predicate-useAgg';
  const db_perf = 'perf';
  const coll_points = 'points';
  const STITCH_CONFIG = {
    PERF:{
      DB_PERF: db_perf,
      COLL_POINTS: coll_points
    }
  };

  let service;
  let $db;

  describe('getRejectedPointsQ', () => {
    beforeEach(function() {
      module(function($provide) {
        $window = {project: project};
        db = {
          db: () => db,
          collection: () => db,
          find: () => db,
          execute: () => db,
        };
        Stitch = {
          use: () => Stitch,
          query: () => Stitch,
          then: () => Stitch,
        };

        stitch = {BSON: {BSONRegExp: BSONRegExp}};
        spyOn(stitch.BSON, 'BSONRegExp');

        $provide.value('$window', $window);
        $provide.value('Stitch', Stitch);
        $provide.value('stitch', stitch);
      });

      inject(function($injector) {
        service = $injector.get('PointsDataService');
      })
    });
    describe('test param', () => {

      it('should default to canaries', function() {
        service.getRejectedPointsQ(project, variant, task);
        expect(stitch.BSON.BSONRegExp).toHaveBeenCalledWith('^(fio|canary|iperf|NetworkBandwidth)');
      });

      it('should pass real value', function() {
        service.getRejectedPointsQ(project, variant, task, test_name);
        expect(stitch.BSON.BSONRegExp.callCount).toBe(undefined);
      });

    });
  });

  describe('query', () => {
    beforeEach(function() {
      module(function($provide) {
        $window = {project: project};
        $db = {
          db: () => $db,
          collection: () => $db,
          find: () => $db,
          execute: () => $db,
          then: () => $db,
        };
        spyOn($db, 'db').and.callThrough();
        spyOn($db, 'collection').and.callThrough();
        spyOn($db, 'find').and.callThrough();
        spyOn($db, 'execute').and.callThrough();

        Stitch = {
          use: () => Stitch,
          query: (cb) => cb($db),
          then: () => Stitch,
        };
        $provide.value('$window', $window);
        $provide.value('Stitch', Stitch);
        $provide.value('STITCH_CONFIG', STITCH_CONFIG);
      });

      inject(function($injector) {
        service = $injector.get('PointsDataService');
      })
    });

    describe('query handling', () => {

      it('should default to canaries', function() {
        service.getRejectedPointsQ(project, variant, task);
        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_points);
        expect($db.find).toHaveBeenCalledWith({
          project: project,
          variant: variant,
          task: task,
          test: new stitch.BSON.BSONRegExp('^(fio|canary|iperf|NetworkBandwidth)'),
          $or : [
            {'$and':[{ "rejected" : true }, { "outlier" : true }]},
            {'$and':[{ "results.rejected" : true }, { "results.outlier" : true }]}]
        });
      });

      it('should pass on value', function() {
        service.getRejectedPointsQ(project, variant, task, test_name);
        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_points);
        expect($db.find).toHaveBeenCalledWith({
          project: project,
          variant: variant,
          task: task,
          test: test_name,
          $or : [
            {'$and':[{ "rejected" : true }, { "outlier" : true }]},
            {'$and':[{ "results.rejected" : true }, { "results.outlier" : true }]}]
        });
      });

    });
  });

  describe('points processing', () => {
    describe('success', () => {
      describe('no data', () => {
        beforeEach(function() {
          module(function($provide) {
            $window = {project: project};

            Stitch = {
              use: () => Stitch,
              query: () => Stitch,
              then: (cb) => cb([]),
            };
            $provide.value('$window', $window);
            $provide.value('Stitch', Stitch);
          });

          inject(function($injector) {
            service = $injector.get('PointsDataService');
          })
        });

        it('should return an empty array', function() {
          expect(service.getRejectedPointsQ(project, variant, task)).toEqual([])
        });
      });
      describe('with data', () => {
        beforeEach(function() {
          module(function($provide) {
            $window = {project: project};

            Stitch = {
              use: () => Stitch,
              query: () => Stitch,
              then: (cb) => cb([
                {task_id:'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13'},
                {task_id:'sys_perf_linux_standalone_bestbuy_agg_58ff5248610305a35403020077a4dbdcd0691ff0_19_03_17_21_34_54'},
                {task_id:'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13'}
                ]),
            };
            $provide.value('$window', $window);
            $provide.value('Stitch', Stitch);
          });

          inject(function($injector) {
            service = $injector.get('PointsDataService');
          })
        });

        it('should return uniq task ids', function() {
          expect(service.getRejectedPointsQ(project, variant, task)).toEqual([
            'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13',
            'sys_perf_linux_standalone_bestbuy_agg_58ff5248610305a35403020077a4dbdcd0691ff0_19_03_17_21_34_54'])
        });
      });
    });
    describe('failure', () => {
      beforeEach(function() {
        module(function($provide) {
          $window = {project: project};

          Stitch = {
            use: () => Stitch,
            query: () => Stitch,
            then: (cb, err) => err('something went wrong!'),
          };
          $provide.value('$window', $window);
          $provide.value('Stitch', Stitch);
        });

        inject(function($injector) {
          service = $injector.get('PointsDataService');
        })
      });

      it('should return an empty array', function() {
        expect(service.getRejectedPointsQ(project, variant, task)).toEqual([]);
      });
    });
  });
});
