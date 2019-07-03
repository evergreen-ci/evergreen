// constants
finishedStatuses = ["success","failed"]

function taskIsBlocked(task) {
    for(dep of task.depends_on) {
        if (dep.unattainable) {
            return true
        }
    }

    return false
}

function unattainableResolved(task) {
    for (dep of task.depends_on) {
        if (dep.unattainable == null) {
            return false
        }
    }
    return true
}

function getDepTaskMap(task) {
    var depIDs = []
    for (dep of task.depends_on) {
        depIDs.push(dep._id)
    }
    depTasks = db.tasks.find({"_id": {"$in": depIDs}}, {"status": 1, "depends_on": 1});

    var depTaskMap = {}
    for (task of depTasks) {
        depTaskMap[task._id] = task
    }

    return depTaskMap
}

//
// example invocation:
//
// startingDate = ISODate("2013-01-01T00:00:00Z")
// millisecondsPerDay = 86400000
// delta = millisecondsPerDay * 1
// sleepMilliseconds = 100
// migrate(startingDate, delta, sleepMilliseconds)

function migrate(startingDate, delta, sleepMilliseconds) {
    while(startingDate < new Date()) {
        loops = 0
        tasksSize = 0
        while (true) {
            tasks = db.tasks.find({"create_time": {"$gte": startingDate, "$lte": new Date(startingDate.getTime() + delta)}, "status":"undispatched", "depends_on.status": {"$in": ["success", "failed", "", "*"]}, "depends_on": {"$elemMatch":{"unattainable": {"$exists": false}}}}, {"depends_on":1}).toArray()
            printjson(loops++)
            if (tasks.length == 0 || tasks.length == tasksSize) {
                break
            }
            tasksSize = tasks.length

            for (i=0; i < tasks.length; i++) {
                taskUpdated = false
                dependsOn = tasks[i].depends_on
                for (j=0; j < dependsOn.length; j++) {
                    if(dependsOn[j].unattainable != null) {
                        continue
                    }
                    
                    depTasksMap = getDepTaskMap(task)
                    if (!(dependsOn[j]._id in depTasksMap)) {
                        break
                    }
                    depTask = depTasksMap[dependsOn[j]._id]
                    if (finishedStatuses.includes(depTask.status)) {
                        taskUpdated = true
                        // 1st degree blocked
                        if(dependsOn[j].status != "*" && depTask.status != dependsOn[j].status) {
                            dependsOn[j].unattainable = true
                        } else {
                            dependsOn[j].unattainable = false
                        }
                    } else if (dependsOn[j].status != "*" && taskIsBlocked(depTask._id)) {
                        taskUpdated = true
                        dependsOn[j].unattainable = true
                    } else if (unattainableResolved(depTask._id)) {
                        taskUpdated = true
                        dependsOn[j].unattainable = false
                    }
                }
                if(taskUpdated) {
                    db.tasks.updateOne({"_id": tasks[i]._id}, {"$set": {"depends_on": dependsOn}})
                }
            }

        }
        startingDate = new Date(startingDate.getTime() + delta)
        sleep(sleepMilliseconds)
    }
}