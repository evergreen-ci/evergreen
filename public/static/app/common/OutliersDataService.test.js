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
          service.getMarkedOutliersQ({project: project});
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_marked_outliers);
        });

        it('should find query', function () {
          service.getMarkedOutliersQ({project: project, variant: variant, task: task, test: test_name, revision: revision});
          expect($db.find).toHaveBeenCalledWith({project: project, variant: variant, task: task, test: test_name, revision: revision});
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
            expect(service.getMarkedOutliersQ({project: project, variant: variant, task: task})).toEqual([]);
          });
        });

        describe('with data', () => {
          it('should return uniq task ids', () => {
            setUpQueryResults([1, 2, 3]);
            expect(service.getMarkedOutliersQ({project: project, variant: variant, task: task})).toEqual([1, 2, 3]);
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
            expect(service.getMarkedOutliersQ({project: project, variant: variant, task: task})).toEqual([]);
          });
        });
      });
    });
  });

  describe('addMark', () => {
    let mark;
    let mark_identifier;
    describe('query', () => {
      beforeEach(() => {
        module($provide => {
          $db = {
            db: () => $db,
            collection: () => $db,
            updateOne: () => $db,
            execute: () => $db,
            then: () => $db,
          };
          spyOn($db, 'db').and.callThrough();
          spyOn($db, 'collection').and.callThrough();
          spyOn($db, 'updateOne').and.callThrough();
          spyOn($db, 'execute').and.callThrough();

          Stitch = {
            use: () => Stitch,
            query: (cb) => cb($db),
            then: () => Stitch,
          };
          $provide.value('Stitch', Stitch);
          mark = {
            revision: 'f871dc86098cde8ab69286c650491cbdd637d724',
            project: 'sys-perf',
            variant: 'linux-standalone' ,
            task: 'big_update',
            test: 'BigUpdate.Loader.IndividualBulkInsert',
            thread_level: '10',
            _id: 'ObjectId'
          };
          mark_identifier = _.pick(mark, 'revision', 'project', 'variant' , 'task', 'test', 'thread_level');
        });

        inject($injector => service = $injector.get('OutliersDataService'));
      });

      describe('query handling', () => {

        it('should use correct database / collection', function () {
          service.addMark(mark_identifier, mark);
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_marked_outliers);
        });

        it('should updateOne', function () {
          service.addMark(mark_identifier, mark);
          expect($db.updateOne).toHaveBeenCalledWith(mark_identifier, {$setOnInsert: mark}, { upsert: true });
        });

      });
    });

    describe('results processing', () => {
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

        describe('result processing', () => {
          it('should return identifier on upsert', () => {
            setUpQueryResults({matchedCount: 0,  modifiedCount: 0, upsertedId: 'upsertedId'});
            expect(service.addMark(mark_identifier, mark)).toEqual(mark_identifier);
          });

          it('should return identifier on match but not modified', () => {
            setUpQueryResults({matchedCount: 1,  modifiedCount: 0, upsertedId: null});
            expect(service.addMark(mark_identifier, mark)).toEqual(mark_identifier);
          });

          it('should return identifier on modified', () => {
            setUpQueryResults({matchedCount: 1,  modifiedCount: 1, upsertedId: null});
            expect(service.addMark(mark_identifier, mark)).toEqual(mark_identifier);
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

          it('should return undefined', () => {
            setUpErrorResults();
            expect(service.addMark(mark_identifier, mark)).toBe(undefined);
          });

        });
      });
    });
  });

  describe('removeMark', () => {
    let mark_identifier;
    describe('query', () => {
      beforeEach(() => {
        module($provide => {
          $db = {
            db: () => $db,
            collection: () => $db,
            deleteOne: () => $db,
            then: () => $db,
          };
          spyOn($db, 'db').and.callThrough();
          spyOn($db, 'collection').and.callThrough();
          spyOn($db, 'deleteOne').and.callThrough();

          Stitch = {
            use: () => Stitch,
            query: (cb) => cb($db),
            then: () => Stitch,
          };
          $provide.value('Stitch', Stitch);
          mark_identifier = {
            revision: 'f871dc86098cde8ab69286c650491cbdd637d724',
            project: 'sys-perf',
            variant: 'linux-standalone' ,
            task: 'big_update',
            test: 'BigUpdate.Loader.IndividualBulkInsert',
            thread_level: '10',
          };
        });

        inject($injector => service = $injector.get('OutliersDataService'));
      });

      describe('query handling', () => {

        it('should use correct database / collection', function () {
          service.removeMark(mark_identifier);
          expect($db.db).toHaveBeenCalledWith(db_perf);
          expect($db.collection).toHaveBeenCalledWith(coll_marked_outliers);
        });

        it('should updateOne', function () {
          service.removeMark(mark_identifier);
          expect($db.deleteOne).toHaveBeenCalledWith(mark_identifier);
        });

      });
    });

    describe('results processing', () => {
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

        describe('result processing', () => {
          it('should return mark_identifier on no delete', () => {
            setUpQueryResults({deletecCount: 0});
            expect(service.removeMark(mark_identifier)).toBe(mark_identifier);
          });

          it('should return identifier on delete', () => {
            setUpQueryResults({deletecCount: 1});
            expect(service.removeMark(mark_identifier)).toEqual(mark_identifier);
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

          it('should return undefined', () => {
            setUpErrorResults();
            expect(service.removeMark(mark_identifier)).toBe(undefined);
          });
        });
      });
    });
  });

});
