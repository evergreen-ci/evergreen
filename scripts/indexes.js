//======alertrecord======//
db.alertrecord.ensureIndex({ "host_id" : 1 })
db.alertrecord.ensureIndex({ "version_id" : 1, "type" : 1 })
db.alertrecord.ensureIndex({ "type" : 1, "version_id" : 1 })

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
db.patchfiles.files.ensureIndex({"filename":1})

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
db.hosts.ensureIndex({ "status" : 1, "create_time": 1, "termination_time" : 1, "provider": 1 })
db.hosts.ensureIndex({ "running_task": 1}, {sparse: true, unique: true})

//======pushes======//
db.pushes.ensureIndex({ "status" : 1, "location" : 1, "order" : 1 })

//======patches======//
db.patches.ensureIndex({ "branch" : 1, "create_time" : 1 })
db.patches.ensureIndex({ "version" : 1 })
db.patches.ensureIndex({ "author" : 1, "create_time" : 1 })
db.patches.ensureIndex({ "github_patch_data.pr_number" : 1, "github_patch_data.base_repo" : 1,"github_patch_data.base_owner" : 1, "create_time" : 1 }, { "sparse": true })

//======project_ref======//
db.project_ref.ensureIndex({ "identifier" : 1 })
db.project_ref.ensureIndex({ "owner" : 1, "repo" : 1 })

//======spawn_requests======//
db.spawn_requests.ensureIndex({ "host" : 1 })
db.spawn_requests.ensureIndex({ "user" : 1, "status" : 1 })

//======task_bk======//
db.task_bk.ensureIndex({ "branch" : 1, "build_variant" : 1, "name" : 1 })

//======task_event_log======//
db.task_event_log.ensureIndex({ "r_id" : 1, "data.r_type" : 1, "ts" : 1 })

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
db.tasks.ensureIndex({ "status": 1, "start_time" : 1, "finish_time" : 1})
db.tasks.ensureIndex({ "branch": 1, "status": 1, "test_results.test_file" : 1, "test_results.status": 1}, {partialFilterExpression: {"branch": "mongodb-mongo-master"}})
db.tasks.ensureIndex({ "branch": 1, "r" : 1, "display_name" : 1})
db.tasks.ensureIndex({ "branch": 1, "r" : 1, "status" : 1})
db.tasks.ensureIndex({ "branch": 1, "r" : 1, "build_variant" : 1})
db.tasks.ensureIndex({ "finish_time": 1, "_id": 1})
db.tasks.ensureIndex({ "build_variant": 1, "branch" : 1, "order" : 1})
db.tasks.ensureIndex({ "execution_tasks": 1})

//======old_tasks======//
db.old_tasks.ensureIndex({ "branch": 1, "r" : 1, "display_name" : 1})
db.old_tasks.ensureIndex({ "branch": 1, "r" : 1, "status" : 1})
db.old_tasks.ensureIndex({ "branch": 1, "r" : 1, "build_variant" : 1})

//======versions======//
db.versions.ensureIndex({ "order" : 1 })
db.versions.ensureIndex({ "builds" : 1 })
db.versions.ensureIndex({ "identifier" : 1, "r" : 1, "order" : 1 })
db.versions.ensureIndex({ "branch" : 1, "gitspec" : 1 })
db.versions.ensureIndex({ "versions.build_variant_status.build_variant" : 1, "versions.build_variant_status.activated" : 1, "r": 1 })
db.versions.ensureIndex({ "create_time": 1, "r": 1  })

//======alerts=======//
db.alerts.ensureIndex({ "queue_status" : 1 })

//======test_logs=====//
db.test_logs.ensureIndex({ "execution" : 1, "name" : 1, "task" : 1 })

//======json======//
db.json.ensureIndex({ "task_id" : 1 })
db.json.ensureIndex({ "project_id" : 1, "tag" : 1 })
db.json.ensureIndex({ "name" : 1, "task_id" : 1 })
db.json.ensureIndex({ "version_id" : 1 })

//======testresults======//
db.testresults.ensureIndex({ "task_id" : 1, "task_execution" : 1 })

//======project_aliases======//
db.project_aliases.ensureIndex({ "project_id" : 1, "alias" : 1 })

//======patch_intents======//
db.patch_intents.ensureIndex({ "processed" : 1, "intent_type" : 1 })
db.patch_intents.ensureIndex({ "msg_id" : 1 }, { "unique" : true, "sparse": true })

//======github_hooks======//
db.github_hooks.ensureIndex({ "owner" : 1, "repo" : 1 }, { "unique": true })
