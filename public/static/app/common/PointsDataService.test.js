describe('PointsDataServiceSpec', function () {
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
  const EMPTY_OUTLIERS = {rejects: [], outliers: {}};

  let service;
  let $db;

  /**
   *  SetUp the base environment / closure. You can call further beforeEach blocks,
   *  but the state will be 'set' on injection.
   */
  beforeEach(() => {
    stitch = {BSON: {BSONRegExp: BSONRegExp}};
    $window = {project: project};
    module($provide => {
      Stitch = {
        use: () => Stitch,
        query: () => Stitch,
        then: () => Stitch,
      };
      $provide.value('$window', $window);
      $provide.value('Stitch', Stitch);
    });
  });

  describe('getOutlierPointsQ', () => {
    describe('params', () => {
      beforeEach(function () {
        module($provide => {
          Stitch = {
            use: () => Stitch,
            query: () => Stitch,
            then: () => Stitch,
          };
          spyOn(stitch.BSON, 'BSONRegExp');
          $provide.value('Stitch', Stitch);
        });

        inject($injector => service = $injector.get('PointsDataService'));
      });
      describe('test param', () => {

        it('should default to canaries', function () {
          service.getOutlierPointsQ(project, variant, task);
          expect(stitch.BSON.BSONRegExp).toHaveBeenCalledWith('^(fio|canary|iperf|NetworkBandwidth)');
        });

        it('should pass real value', function () {
          service.getOutlierPointsQ(project, variant, task, test_name);
          expect(stitch.BSON.BSONRegExp.callCount).toBeUndefined();
        });

      });
    });

    describe('query', () => {
      beforeEach(() => {
        module($provide => {
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
          $provide.value('Stitch', Stitch);
        });

        inject($injector => service = $injector.get('PointsDataService'));
      });
      const getExpectedQuery = test => {
        if (_.isNull(test) || _.isUndefined(test)) {
          test = new stitch.BSON.BSONRegExp('^(fio|canary|iperf|NetworkBandwidth)');
        }
        return {
          project: project,
          variant: variant,
          task: task,
          test: test,
          $or: [{"outlier": true}, {"results.outlier": true}]
        }
      };
      describe('query handling', () => {

        it('should default to canaries', function () {
          service.getOutlierPointsQ(project, variant, task);
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_points);
          expect($db.find).toHaveBeenCalledWith(getExpectedQuery());
        });

        it('should pass on value', function () {
          service.getOutlierPointsQ(project, variant, task, test_name);
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_points);
          expect($db.find).toHaveBeenCalledWith(getExpectedQuery(test_name))
        });
      });
    });

    describe('points processing', () => {
      describe('success', () => {
        const setUpQueryResults = query_results => {
          module($provide => {
            Stitch = {
              use: () => Stitch,
              query: () => Stitch,
              then: (cb) => cb(query_results),
            };
            $provide.value('Stitch', Stitch);
          });

          inject($injector => service = $injector.get('PointsDataService'));
        };

        describe('no data', () => {
          it('should return an empty document', () => {
            setUpQueryResults([]);
            expect(service.getOutlierPointsQ(project, variant, task)).toEqual(EMPTY_OUTLIERS);
          });
        });

        describe('with data', () => {
          it('should return uniq task ids', () => {
            setUpQueryResults([
              // only rejected in base doc, ie. 'max' thread level
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_e35e8076dbddc863205cf24517e1b16dc9104d07_19_03_24_22_05_57',
                test:'canary_client-cpuloop-10x',
                outlier:true,
                rejected:true,
                results:[{thread_level:"1"}]
              },
              // duplicate rejected in results doc, ie. 'max' thread level
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_e35e8076dbddc863205cf24517e1b16dc9104d07_19_03_24_22_05_57',
                test:'fio_iops_test_read_iops',
                outlier:true,
                rejected:true,
                results:[{thread_level:"1"}]
              },
              // rejected not outlier, max level
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_10f196bb962c6d4f983b9d7b1209aff26f97573a_19_03_23_01_15_20',
                test:'canary_client-cpuloop-10x',
                rejected:true,
                results:[{outlier: true, thread_level:"1"}]
              },
              // rejected not outlier, results
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_10f196bb962c6d4f983b9d7b1209aff26f97573a_19_03_23_01_15_20',
                test:'fio_iops_test_read_iops',
                outlier: true,
                results:[{rejected:true,thread_level:"1"}]
              },
              // only rejected in results doc
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13',
                test:'canary_client-cpuloop-1x',
                results:[{rejected:true, outlier:true, thread_level:"1"}]
              },
              // outlier in results document
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13',
                test:test_name,
                outlier:true,
                results:[{rejected:true, outlier:true, thread_level:"1"}]
              },
              // outlier in base document, e.g. max level
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_58ff5248610305a35403020077a4dbdcd0691ff0_19_03_17_21_34_54',
                test:test_name,
                outlier:true,
                results:[{thread_level:"16"}]
              },
              // outlier in base document and result thread level
              {
                task_id: 'sys_perf_linux_standalone_bestbuy_agg_c13f0de07a8ad9523683db5569e7f8e6b60bd67d_19_03_14_13_50_51',
                test:test_name,
                outlier:true,
                results:[{outlier:true, thread_level:"1"}]
              }
            ]);
            const actual = service.getOutlierPointsQ(project, variant, task);
            expect(actual.rejects).toEqual([
              'sys_perf_linux_standalone_bestbuy_agg_e35e8076dbddc863205cf24517e1b16dc9104d07_19_03_24_22_05_57',
              'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13',
            ]);
            const outliers = actual.outliers;
            expect(_.keys(outliers)).toEqual([
              'sys_perf_linux_standalone_bestbuy_agg_e35e8076dbddc863205cf24517e1b16dc9104d07_19_03_24_22_05_57',
              'sys_perf_linux_standalone_bestbuy_agg_10f196bb962c6d4f983b9d7b1209aff26f97573a_19_03_23_01_15_20',
              'sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13',
              'sys_perf_linux_standalone_bestbuy_agg_58ff5248610305a35403020077a4dbdcd0691ff0_19_03_17_21_34_54',
              'sys_perf_linux_standalone_bestbuy_agg_c13f0de07a8ad9523683db5569e7f8e6b60bd67d_19_03_14_13_50_51',
            ]);

            expect(outliers['sys_perf_linux_standalone_bestbuy_agg_e35e8076dbddc863205cf24517e1b16dc9104d07_19_03_24_22_05_57']).toEqual([
              {test:'canary_client-cpuloop-10x',thread_level: 'max'},
              {test:'fio_iops_test_read_iops',thread_level: 'max'},
            ]);

            expect(outliers['sys_perf_linux_standalone_bestbuy_agg_10f196bb962c6d4f983b9d7b1209aff26f97573a_19_03_23_01_15_20']).toEqual([
              {test:'canary_client-cpuloop-10x',thread_level: '1'},
              {test:'fio_iops_test_read_iops',thread_level: 'max'},
            ]);

            expect(outliers['sys_perf_linux_standalone_bestbuy_agg_0a856820ba29e19dcba0979f71b11ae01f4be0f1_19_03_15_14_05_13']).toEqual([
              {test:'canary_client-cpuloop-1x',thread_level: '1'},
              {test:test_name,thread_level: '1'},
              {test:test_name,thread_level: 'max'},
            ]);

            expect(outliers['sys_perf_linux_standalone_bestbuy_agg_c13f0de07a8ad9523683db5569e7f8e6b60bd67d_19_03_14_13_50_51']).toEqual([
              {test:test_name,thread_level: '1'},
              {test:test_name,thread_level: 'max'},
            ]);
          });
        });
      });
      describe('failure', () => {
        const setUpErrorResults = () => {
          module($provide => {
            Stitch = {
              use: () => Stitch,
              query: () => Stitch,
              then: (cb, err) => err('something went wrong'),
            };
            $provide.value('Stitch', Stitch);
          });

          inject($injector => service = $injector.get('PointsDataService'));
        };

        it('should return an empty array', () => {
          setUpErrorResults();
          expect(service.getOutlierPointsQ(project, variant, task)).toEqual(EMPTY_OUTLIERS);
        });
      });
    });
  });
});
