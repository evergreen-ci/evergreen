describe('OutliersDataService', function () {
  beforeEach(module('MCI'));

  function BSONRegExp(test) {
    return test;
  }

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';
  const test_name = ' distinct_types_no_predicate-useAgg';
  const db_perf = 'perf';
  const coll_outliers = 'outliers';

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

        it('should default type to detected', function () {
          service.getOutliersQ(project, {});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_outliers);
          expect($db.find).toHaveBeenCalledWith({project: project, type: 'detected'});
        });

        it('should allow other types', function () {
          service.getOutliersQ(project, {type: 'suspicious'});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_outliers);
          expect($db.find).toHaveBeenCalledWith({project: project, type: 'suspicious'});
        });

        it('should allow variants', function () {
          service.getOutliersQ(project, {variant: variant});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_outliers);
          expect($db.find).toHaveBeenCalledWith({project: project, variant: variant, type: 'detected'});
        });

        it('should allow variants', function () {
          service.getOutliersQ(project, {test: test_name});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_outliers);
          expect($db.find).toHaveBeenCalledWith({project: project, test: test_name, type: 'detected'});
        });

        it('should allow tasks', function () {
          service.getOutliersQ(project, {task: task});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_outliers);
          expect($db.find).toHaveBeenCalledWith({project: project, task: task, type: 'detected'});
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
});
