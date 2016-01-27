//======artifact_files======//
db.artifact_files.ensureIndex({ "task" : 1 })
db.artifact_files.ensureIndex({ "build" : 1 })

//======builds======//
db.builds.ensureIndex({ "build_variant" : 1, "status" : 1, "order" : 1 })
db.builds.ensureIndex({ "start_time" : 1 })
db.builds.ensureIndex({ "finished" : 1 })
db.builds.ensureIndex({ "gitspec" : 1 })
db.builds.ensureIndex({ "version" : 1 })
db.builds.ensureIndex({ "branch" : 1, "r" : 1, "order" : 1 })

//======patch files====//
db.patchfiles.files.createIndex({"filename":1})

//======event_log======//
db.event_log.ensureIndex({ "r_id" : 1, "data.r_type" : 1, "ts" : 1 })

//======hosts======//
db.hosts.ensureIndex({ "status": 1 })
db.hosts.ensureIndex({ "started_by" : 1, "status" : 1 })
db.hosts.ensureIndex({ "running_task" : 1, "status" : 1 })
db.hosts.ensureIndex({ "host_type" : 1, "_id" : 1 })
db.hosts.ensureIndex({ "branch" : 1, "create_time" : 1 })
db.hosts.ensureIndex({ "version" : 1 })
db.hosts.ensureIndex({ "author" : 1 })
db.hosts.ensureIndex({ "distro._id" : 1, "status" : 1 })

//======pushes======//
db.pushes.ensureIndex({ "status" : 1, "location" : 1, "order" : 1 })

//======spawn_requests======//
db.spawn_requests.ensureIndex({ "host" : 1 })
db.spawn_requests.ensureIndex({ "user" : 1, "status" : 1 })

//======task_bk======//
db.task_bk.ensureIndex({ "branch" : 1, "build_variant" : 1, "name" : 1 })

//======tasks======//
db.tasks.ensureIndex({ "build_variant" : 1, "display_name" : 1, "order" : 1 })
db.tasks.ensureIndex({ "gitspec" : 1, "build_variant" : 1, "display_name" : 1 })
db.tasks.ensureIndex({ "status" : 1, "build_variant" : 1, "order" : 1 })
db.tasks.ensureIndex({ "build_variant" : 1, "display_name" : 1, "status" : 1, "order" : 1 })
db.tasks.ensureIndex({ "branch":1, "build_variant":1, "display_name":1, "status":1, "r":1, "activated":1, "order":1})
db.tasks.ensureIndex({ "activated" : 1, "status" : 1 })
db.tasks.ensureIndex({ "branch" : 1, "build_variant" : 1, "status" : 1, "finish_time" : 1 })
db.tasks.ensureIndex({ "build_id" : 1 })
db.tasks.ensureIndex({ "status" : 1, "finish_time" : 1 })
db.tasks.ensureIndex({ "version" : 1, "display_name" : 1 })
db.tasks.ensureIndex({ "order" : 1, "display_name" : 1 })

//======versions======//
db.versions.ensureIndex({ "order" : 1 })
db.versions.ensureIndex({ "builds" : 1 })
db.versions.ensureIndex({ "identifier" : 1, "r" : 1, "order" : 1 })
db.versions.ensureIndex({ "branch" : 1, "gitspec" : 1 })
db.versions.ensureIndex({ "versions.build_variant_status.build_variant" : 1, "versions.build_variant_status.activated" : 1, "r": 1 })

//======alerts=======//
db.alerts.createIndex({queue_status:1})
