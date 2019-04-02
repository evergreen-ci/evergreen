describe('OutliersDataService', function () {
  beforeEach(module('MCI'));

  function BSONRegExp(test) {
    return test;
  }

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';
  const revision = 'f871dc86098cde8ab69286c650491cbdd637d724';
  const test_name = ' distinct_types_no_predicate-useAgg';
  const db_perf = 'perf';
  const coll_outliers = 'outliers';
  const coll_marked_outliers = 'marked_outliers';

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

        inject($injector => service = $injector.get('OutliersDataService'));
      });
      describe('query handling', () => {

        it('should use correct database and collection', function () {
          service.getOutliersQ(project, {});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_outliers);
        });

        it('should default type to detected', function () {
          service.getOutliersQ(project, {});
          expect($db.find).toHaveBeenCalledWith({project: project, type: 'detected'});
        });

        it('should allow other types', function () {
          service.getOutliersQ(project, {type: 'suspicious'});
          expect($db.find).toHaveBeenCalledWith({project: project, type: 'suspicious'});
        });

        it('should allow variants', function () {
          service.getOutliersQ(project, {variant: variant});
          expect($db.find).toHaveBeenCalledWith({project: project, variant: variant, type: 'detected'});
        });

        it('should allow variants', function () {
          service.getOutliersQ(project, {test: test_name});
          expect($db.find).toHaveBeenCalledWith({project: project, test: test_name, type: 'detected'});
        });

        it('should allow tasks', function () {
          service.getOutliersQ(project, {task: task});
          expect($db.find).toHaveBeenCalledWith({project: project, task: task, type: 'detected'});
        });

        it('should allow revision', function () {
          service.getOutliersQ(project, {revision: revision});
          expect($db.find).toHaveBeenCalledWith({project: project, revision: revision, type: 'detected'});
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

          inject($injector => service = $injector.get('OutliersDataService'));
        };

        describe('no data', () => {
          it('should return an empty document', () => {
            setUpQueryResults([]);
            expect(service.getOutliersQ(project, variant, task)).toEqual([]);
          });
        });

        describe('with data', () => {
          it('should return uniq task ids', () => {
            setUpQueryResults([1, 2, 3]);
            expect(service.getOutliersQ(project, variant, task)).toEqual([1, 2, 3]);
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

            inject($injector => service = $injector.get('OutliersDataService'));
          };

          it('should return an empty array', () => {
            setUpErrorResults();
            expect(service.getOutliersQ(project, variant, task)).toEqual([]);
          });
        });
      });
    });
  });

  describe('getMarkedOutliersQ', () => {

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

        inject($injector => service = $injector.get('OutliersDataService'));
      });
      describe('query handling', () => {

        it('should use correct database / collection', function () {
          service.getMarkedOutliersQ(project, {});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_marked_outliers);
        });

        it('should allow variants', function () {
          service.getMarkedOutliersQ(project, {variant: variant});
          expect($db.find).toHaveBeenCalledWith({project: project, variant: variant});
        });

        it('should allow variants', function () {
          service.getMarkedOutliersQ(project, {test: test_name});
          expect($db.find).toHaveBeenCalledWith({project: project, test: test_name});
        });

        it('should allow tasks', function () {
          service.getMarkedOutliersQ(project, {task: task});
          expect($db.find).toHaveBeenCalledWith({project: project, task: task});
        });

        it('should allow revision', function () {
          service.getMarkedOutliersQ(project, {revision: revision});
          expect($db.find).toHaveBeenCalledWith({project: project, revision: revision});
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

          inject($injector => service = $injector.get('OutliersDataService'));
        };

        describe('no data', () => {
          it('should return an empty document', () => {
            setUpQueryResults([]);
            expect(service.getMarkedOutliersQ(project, variant, task)).toEqual([]);
          });
        });

        describe('with data', () => {
          it('should return uniq task ids', () => {
            setUpQueryResults([1, 2, 3]);
            expect(service.getMarkedOutliersQ(project, variant, task)).toEqual([1, 2, 3]);
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

            inject($injector => service = $injector.get('OutliersDataService'));
          };

          it('should return an empty array', () => {
            setUpErrorResults();
            expect(service.getMarkedOutliersQ(project, variant, task)).toEqual([]);
          });
        });
      });
    });
  });
});
