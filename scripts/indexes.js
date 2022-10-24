//======alertrecord======//
db.alertrecord.ensureIndex({
    "subscription_id": 1,
    "host_id": 1
})
db.alertrecord.ensureIndex({
    "subscription_id": 1,
    "version_id": 1,
    "type": 1
})
db.alertrecord.ensureIndex({
    "subscription_id": 1,
    "type": 1,
    "version_id": 1
})
db.alertrecord.ensureIndex({
    "subscription_id": 1,
    "type": 1,
    "project_id": 1,
    "variant": 1,
    "task_name": 1,
    "test_name": 1,
    "order": -1
})
db.alertrecord.ensureIndex({
    "subscription_id": 1,
    "project_id": 1,
    "task_name": 1,
    "type": 1,
    "variant": 1,
    "alert_time": -1,
    "order": -1
})

//======artifact_files======//
db.artifact_files.ensureIndex({
    "task": 1
})
db.artifact_files.ensureIndex({
    "build": 1
})

//======builds======//
db.builds.ensureIndex({
    "build_variant": 1,
    "status": 1,
    "order": 1
})
db.builds.ensureIndex({
    "start_time": 1
})
db.builds.ensureIndex({
    "finished": 1
})
db.builds.ensureIndex({
    "gitspec": 1
})
db.builds.ensureIndex({
    "version": 1
})
db.builds.ensureIndex({
    "branch": 1,
    "r": 1,
    "order": 1
})

//======patch files====//
db.patchfiles.files.ensureIndex({
    "filename": 1
})

//======events======//
db.events.ensureIndex({
    "ts": 1
})
db.events.ensureIndex({
    "r_id": 1,
    "r_type": 1,
    "ts": 1
})
db.events.ensureIndex({
    "processed_at": 1,
    "ts": 1
})

//======hosts======//
db.hosts.ensureIndex({
    "started_by": 1,
    "status": 1
})
db.hosts.ensureIndex({
    "running_task": 1,
    "status": 1
})
db.hosts.ensureIndex({
    "host_type": 1,
    "_id": 1
})
db.hosts.ensureIndex({
    "distro._id": 1,
    "status": 1
})
db.hosts.ensureIndex({
    "running_task": 1
}, {
    sparse: true,
    unique: true
})
db.hosts.createIndex({
    "last_bv": 1,
    "last_group": 1,
    "last_project": 1,
    "last_version": 1,
    "status": 1,
    "last_task": 1
}, {
    background: true
})
db.hosts.createIndex({
    "running_task_bv": 1,
    "running_task_group": 1,
    "running_task_project": 1,
    "running_task_version": 1,
    "status": 1,
    "running_task": 1
}, {
    background: true
})
db.hosts.createIndex({
    "parent_id": 1
})
db.hosts.createIndex({
    "container_pool_settings.id": 1
})
db.hosts.createIndex({
    "status": 1,
    "spawn_options.spawned_by_task": 1
})

//======pushes======//
db.pushes.ensureIndex({
    "status": 1,
    "location": 1,
    "order": 1
})
db.pushes.ensureIndex({
    "task_id": 1
})

//======patches======//
db.patches.ensureIndex({
    "branch": 1,
    "create_time": 1
})
db.patches.ensureIndex({
    "version": 1
})
db.patches.ensureIndex({
    "author": 1,
    "create_time": 1
})
db.patches.ensureIndex({
    "github_patch_data.pr_number": 1,
    "github_patch_data.base_repo": 1,
    "github_patch_data.base_owner": 1,
    "create_time": 1
}, {
    "sparse": true
})

//======project_ref======//
db.project_ref.ensureIndex({
    "identifier": 1
})
db.project_ref.ensureIndex({
    "owner": 1,
    "repo": 1
})
db.project_ref.ensureIndex({
    "triggers.project": 1
})

//======task_bk======//
db.task_bk.ensureIndex({
    "branch": 1,
    "build_variant": 1,
    "name": 1
})

//======tasks======//
db.tasks.ensureIndex({
    "build_variant": 1,
    "display_name": 1,
    "order": 1
})
db.tasks.ensureIndex({
    "gitspec": 1,
    "build_variant": 1,
    "display_name": 1
})
db.tasks.ensureIndex({
    "status": 1,
    "build_variant": 1,
    "order": 1
})
db.tasks.ensureIndex({
    "build_variant": 1,
    "display_name": 1,
    "status": 1,
    "order": 1
})
db.tasks.ensureIndex({
    "branch": 1,
    "build_variant": 1,
    "display_name": 1,
    "status": 1,
    "r": 1,
    "activated": 1,
    "order": 1
})
db.tasks.ensureIndex({
    "activated": 1,
    "status": 1
})
db.tasks.ensureIndex({
    "branch": 1,
    "build_variant": 1,
    "status": 1,
    "finish_time": 1
})
db.tasks.ensureIndex({
    "build_id": 1
})
db.tasks.ensureIndex({
    "status": 1,
    "finish_time": 1
})
db.tasks.ensureIndex({
    "version": 1,
    "display_name": 1
})
db.tasks.ensureIndex({
    "order": 1,
    "display_name": 1
})
db.tasks.ensureIndex({
    "status": 1,
    "start_time": 1,
    "finish_time": 1
})
db.tasks.ensureIndex({
    "branch": 1,
    "status": 1,
    "test_results.test_file": 1,
    "test_results.status": 1
}, {
    partialFilterExpression: {
        "branch": "mongodb-mongo-master"
    }
})
db.tasks.ensureIndex({
    "branch": 1,
    "r": 1,
    "display_name": 1,
    "create_time": 1
})
db.tasks.ensureIndex({
    "branch": 1,
    "r": 1,
    "status": 1
})
db.tasks.ensureIndex({
    "branch": 1,
    "r": 1,
    "build_variant": 1
})
db.tasks.ensureIndex({
    "finish_time": 1,
    "_id": 1
})
db.tasks.ensureIndex({
    "build_variant": 1,
    "branch": 1,
    "order": 1
})
db.tasks.ensureIndex({
    "execution_tasks": 1
})
db.tasks.createIndex({
    "distro": 1,
    "status": 1,
    "activated": 1,
    "priority": 1
}, {
    background: true
})
db.tasks.createIndex({
    "distro_aliases": 1,
    "status": 1,
    "activated": 1,
    "priority": 1
}, {
    background: true
})
db.tasks.createIndex({
    "branch": 1,
    "finish_time": 1
})

//======old_tasks======//
db.old_tasks.ensureIndex({
    "branch": 1,
    "r": 1,
    "display_name": 1,
    "create_time": 1
})
db.old_tasks.ensureIndex({
    "branch": 1,
    "r": 1,
    "status": 1
})
db.old_tasks.ensureIndex({
    "branch": 1,
    "r": 1,
    "build_variant": 1
})
db.old_tasks.ensureIndex({
    "old_task_id": 1
})
db.old_tasks.ensureIndex({
    "branch": 1,
    "finish_time": 1
})
db.old_tasks.createIndex({
    "execution_tasks": 1
})

//======versions======//
db.versions.ensureIndex({
    "order": 1
})
db.versions.ensureIndex({
    "identifier": 1,
    "r": 1,
    "order": 1
})
db.versions.ensureIndex({
    "branch": 1,
    "gitspec": 1
})
db.versions.ensureIndex({
    "identifier": 1,
    "r": 1,
    "create_time": 1
})
db.versions.createIndex({
    "project": 1,
    "periodic_build_id": 1,
    "create_time": -1
}, {
    "partialFilterExpression": {
        "periodic_build_id": {
            "$exists": true
        }
    }
})
db.versions.createIndex({
    "identifier": 1,
    "order": 1
})
db.versions.createIndex({
    "identifier": "1",
    "r": "1",
    "build_variants_status.build_variant": "1",
    "build_variants_status.activated": "1"
})

//======test_logs=====//
db.test_logs.ensureIndex({
    "execution": 1,
    "name": 1,
    "task": 1
})

//======json======//
db.json.ensureIndex({
    "task_id": 1
})
db.json.ensureIndex({
    "project_id": 1,
    "tag": 1
})
db.json.ensureIndex({
    "name": 1,
    "task_id": 1
})
db.json.ensureIndex({
    "version_id": 1
})

//======testresults======//
db.testresults.ensureIndex({
    "task_id": 1,
    "task_execution": 1
})

//======project_aliases======//
db.project_aliases.ensureIndex({
    "project_id": 1,
    "alias": 1
})

//======patch_intents======//
db.patch_intents.ensureIndex({
    "processed": 1,
    "intent_type": 1
})
db.patch_intents.ensureIndex({
    "msg_id": 1
}, {
    "unique": true,
    "sparse": true
})

//======github_hooks======//
db.github_hooks.ensureIndex({
    "owner": 1,
    "repo": 1
}, {
    "unique": true
})

//======subscriptions======//
db.subscriptions.ensureIndex({
    "type": 1,
    "trigger": 1,
    "selectors": 1
})
db.subscriptions.ensureIndex({
    "owner_type": 1,
    "owner": 1
})
db.subscriptions.createIndex({
    "last_updated": 1,
}, {
    sparse: true,
    expireAfterSeconds: 1209600 // 2 weeks
})

//======users======//
db.users.ensureIndex({
    "settings.github_user.uid": 1
}, {
    unique: true
})
db.users.ensureIndex({
    "login_cache.token": 1
}, {
    unique: true,
    sparse: true
})
db.users.createIndex({
    "only_api": 1
}, {
    sparse: true
})

//======notifications======//
db.notifications.ensureIndex({
    "sent_at": 1
})

//======hourly_test_stats======//
db.hourly_test_stats.createIndex({
    "_id.date": 1
}, {
    expireAfterSeconds: 26 * 7 * 24 * 3600
}) // 26 weeks TTL
db.hourly_test_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.task_name": 1,
    "_id.date": 1
})

//======daily_test_stats======//
db.daily_test_stats.createIndex({
    "_id.date": 1
}, {
    expireAfterSeconds: 26 * 7 * 24 * 3600
}) // 26 weeks TTL
db.daily_test_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.test_file": 1,
    "_id.date": 1
})
db.daily_test_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.task_name": 1,
    "_id.date": 1
})
db.daily_test_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.task_name": 1,
    "_id.variant": 1,
    "_id.date": 1
})
db.daily_test_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.test_file": 1,
    "_id.task_name": 1,
    "_id.date": 1
})
db.daily_test_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.test_file": 1,
    "_id.task_name": 1,
    "_id.variant": 1,
    "_id.date": 1
})

//======daily_task_stats======//
db.daily_task_stats.createIndex({
    "_id.date": 1
}, {
    expireAfterSeconds: 26 * 7 * 24 * 3600
}) // 26 weeks TTL
db.daily_task_stats.createIndex({
    "_id.project": 1,
    "_id.requester": 1,
    "_id.task_name": 1,
    "_id.date": 1
})

//======manifest======//
db.manifest.createIndex({
    "project": 1,
    "revision": 1
})