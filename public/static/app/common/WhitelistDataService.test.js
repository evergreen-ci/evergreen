describe('WhitelistDataServiceSpec', function () {
  beforeEach(module('MCI'));

  const project = 'sys-perf';
  const variant = 'linux-standalone';
  const task = 'bestbuy_agg';
  const test_name = ' distinct_types_no_predicate-useAgg';
  const db_perf = 'perf';
  const coll_whitelist = 'whitelisted_outlier_tasks';
  const query = {'project': project, 'variant': variant, 'task': task, 'test': test_name};
  const mongo_filter = {'_id': 1};
  let service;
  let $db;
  const createTaskRevision = (i) => {
    i = i || 1;
    return {'revision': 'revision ' + i, 'project': 'project ' + 1, 'variant': 'variant ' + i, 'task': 'task ' + i, 'random': i}
  };
  const createTaskRevisionQuery = (i) => {
    i = i || 1;
    return {'revision': 'revision ' + i, 'project': 'project ' + 1, 'variant': 'variant ' + i, 'task': 'task ' + i}
  };
  const setUpTaskRevisions = (count) => _.range( count || 1).map(createTaskRevision);
  const setUpTaskRevisionQueries = (count) => _.range( count || 1).map(createTaskRevisionQuery);

  /**
   *  SetUp the base environment / closure. You can call further beforeEach blocks,
   *  but the state will be 'set' on injection.
   */
  beforeEach(() => {
    stitch = {};
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

  describe('addWhitelist', () => {
    describe('query handling', () => {
      let $q;
      let deferred;
      beforeEach(() => {
        module($provide => {
          $db = {
            db: () => $db,
            collection: () => $db,
            execute: () => $db,
            then: () => $db,
            updateOne: () => $db,
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
        });

        inject($injector => {
          $q = $injector.get('$q');
          deferred = $q.defer();
          spyOn($q, 'all').and.returnValue(deferred.promise);

          service = $injector.get('WhitelistDataService')
        });


      });
      it('should process a single task_revisions', function () {
        const task_revisions = setUpTaskRevisions();
        const query  = createTaskRevisionQuery();

        service.addWhitelist(task_revisions);
        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_whitelist);
        expect($db.updateOne).toHaveBeenCalledWith(query, task_revisions[0], {upsert: true});
      });
      it('should process multiple task_revisions', function () {
        const count = 2;
        const task_revisions = setUpTaskRevisions(count);
        const queries  = setUpTaskRevisionQueries(count);

        service.addWhitelist(task_revisions);
        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_whitelist);
        expect($db.updateOne).toHaveBeenCalledTimes(count);

        const calls = _.zip(queries, task_revisions, _.range( count).map(() => ({upsert: true})));
        expect(calls).toEqual($db.updateOne.calls.all().map((call) => call.args));
      });
    });
  });

  describe('removeWhitelist', () => {
    describe('query handling', () => {
      beforeEach(() => {
        module($provide => {
          $db = {
            db: () => $db,
            collection: () => $db,
            execute: () => $db,
            then: () => $db,
            deleteMany: () => $db,
          };
          spyOn($db, 'db').and.callThrough();
          spyOn($db, 'collection').and.callThrough();
          spyOn($db, 'deleteMany').and.callThrough();
          spyOn($db, 'execute').and.callThrough();

          Stitch = {
            use: () => Stitch,
            query: (cb) => cb($db),
            then: () => Stitch,
          };
          $provide.value('Stitch', Stitch);
        });

        inject($injector => service = $injector.get('WhitelistDataService'));
      });

      it('should process a single task_revisions', function () {
        const task_revisions = setUpTaskRevisions();
        const query  =  {$or: setUpTaskRevisionQueries()};

        service.removeWhitelist(task_revisions);

        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_whitelist);
        expect($db.deleteMany).toHaveBeenCalledWith(query);
      });

      it('should process multiple task_revisions', function () {
        const count = 2;
        const task_revisions = setUpTaskRevisions(count);
        const query  =  {$or: setUpTaskRevisionQueries(count)};

        service.removeWhitelist(task_revisions);

        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_whitelist);
        expect($db.deleteMany).toHaveBeenCalledWith(query);
      });
    });
  });

  describe('getWhitelistQ', () => {
    describe('query handling', () => {
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

        inject($injector => service = $injector.get('WhitelistDataService'));
      });
      it('should pass on query', function () {
        service.getWhitelistQ(query);
        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_whitelist);
        expect($db.find).toHaveBeenCalledWith(query, undefined);
      });
      it('should pass on projection', function () {
        service.getWhitelistQ(query, mongo_filter);
        expect($db.db).toHaveBeenCalledWith(db_perf);
        expect($db.collection).toHaveBeenCalledWith(coll_whitelist);
        expect($db.find).toHaveBeenCalledWith(query, mongo_filter);
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

          inject($injector => service = $injector.get('WhitelistDataService'));
        };

        describe('no data', () => {
          it('should return an empty document', () => {
            setUpQueryResults([]);
            expect(service.getWhitelistQ()).toEqual([]);
          });
        });

        describe('with data', () => {
          it('should return uniq task ids', () => {
            const query_results = ['results'];
            setUpQueryResults(query_results);
            const actual = service.getWhitelistQ();
            expect(actual).toEqual(query_results);
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

          inject($injector => service = $injector.get('WhitelistDataService'));
        };

        it('should return an empty array', () => {
          setUpErrorResults();
          expect(service.getWhitelistQ(project)).toEqual([]);
        });
      });
    });
  });
});
