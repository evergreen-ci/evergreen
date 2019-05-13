mciModule.factory('MuteDataService', function(
  $log, Stitch, STITCH_CONFIG
) {
  // Get the mutes as a promise
  // :param query: The query to evaluate.
  // :param projection: Specify the fields in the returned documents.
  const queryQ = (query, projection) => {
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_MUTE_OUTLIERS)
        .find(query, projection)
        .execute();
    }).then((docs) => {
        return docs
      },
      err => {
        // Try to gracefully handle an error.
        $log.error('Cannot load outliers!', err);
        return [];
      });
  };

  const addMute = (mute) => {
    const mute_identifier = _.pick(mute, 'revision', 'project', 'variant' ,'task' , 'test', 'thread_level');
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_MUTE_OUTLIERS)
        .updateOne(mute_identifier,
                  {
                    $set: { enabled: true },
                    $setOnInsert: mute
                  },
                  { upsert: true }
                );
    }).then(result => {
        const { matchedCount, modifiedCount } = result;
        if(result.upsertedId) {
          console.log('Created new mute.');
        } else {
          if(matchedCount) {
            if (modifiedCount) {
              console.log('Updated a existing mute.');
            } else {
              console.log('Successfully added a new mute.');
            }
          }
        }
        return mute_identifier;
      });
  };
  const unMute = (mute) => {
    const mute_identifier = _.pick(mute, 'revision', 'project', 'variant' ,'task' , 'test', 'thread_level');
    return Stitch.use(STITCH_CONFIG.PERF).query(function (db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_MUTE_OUTLIERS)
        .updateOne(mute_identifier, { $set: { enabled: false } });
    }).then(result => {
      const { matchedCount, modifiedCount } = result;
      if(matchedCount) {
        if (modifiedCount) {
          console.log('Updated a existing mute.');
        } else {
          console.log('Successfully added a new mute.');
        }
      }
      return mute_identifier;
    });
  };

  return {
    queryQ: queryQ,
    addMute: addMute,
    unMute : unMute ,
  };
});
