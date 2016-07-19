print("do not run this file");
(function(){

// spawnhosts per month
db.hosts.aggregate([{$match:{started_by:{"$ne": "mci"}}}, 
                    {$group:{_id: {month: { $month: "$creation_time" },  year: { $year: "$creation_time" }}, total:{ $sum: 1 }}},
                    {$sort: {"_id.year":1, "_id.month":1}}]) 

// spawnhosts per month as CSV
db.hosts.aggregate([{$match:{started_by:{"$ne": "mci"}}}, 
                    {$group:{_id: {month: { $month: "$creation_time" },  year: { $year: "$creation_time" }}, total:{ $sum: 1 }}},
                    {$sort: {"_id.year":1, "_id.month":1}}]).result.forEach(
                      function(result){print(result._id.month+"/1/"+result._id.year+", "+result.total);})


// hosts per month as CSV
db.hosts.aggregate([{$match:{started_by: "mci", creation_time:{$gt: new Date(Date() - 1000*60*60*600)}}}, 
                    {$group:{_id: {month: { $month: "$creation_time" },  year: { $year: "$creation_time" }}, total:{ $sum: 1 }}},
                    {$sort: {"_id.year":1, "_id.month":1}}]).result.forEach(
                      function(result){print(result._id.month+"/1/"+result._id.year+", "+result.total);})


// patches per month, csv
db.versions.aggregate([{$match:{create_time:{$exists:true}, r:"patch_request"}},
                       {$group:{_id: {month: { $month: "$create_time" },  year: { $year: "$create_time" }}, total:{ $sum: 1 }}},
                       {$sort: {"_id.year":1, "_id.month":1}}]).result.forEach(
                           function(result){print(result._id.month+"/1/"+result._id.year+", "+result.total);})


// git versions per month, csv
db.versions.aggregate([{$match:{create_time:{$exists:true}, r:"gitter_request"}},
                       {$group:{_id: {month: { $month: "$create_time" },  year: { $year: "$create_time" }}, total:{ $sum: 1 }}},
                       {$sort: {"_id.year":1, "_id.month":1}}]).result.forEach(
                           function(result){print(result._id.month+"/1/"+result._id.year+", "+result.total);})

// patches per month per distro, csv
db.versions.aggregate([{$match:{create_time:{$exists:true}, r:"patch_request", identifier:{$exists:true}}},
                       {$group:{_id: {project:"$identifier", month: { $month: "$create_time" },  year: { $year: "$create_time" }}, total:{ $sum: 1 }}},
                       {$sort: {"_id.project":1, "_id.year":1, "_id.month":1}}]).result.forEach(
                           function(result){print(result._id.project+", "+result._id.month+"/1/"+result._id.year+", "+result.total);})

// git version per month per distro, csv
db.versions.aggregate([{$match:{create_time:{$exists:true}, r:"gitter_request", identifier:{$exists:true}}},
                       {$group:{_id: {project:"$identifier", month: { $month: "$create_time" },  year: { $year: "$create_time" }}, total:{ $sum: 1 }}},
                       {$sort: {"_id.project":1, "_id.year":1, "_id.month":1}}]).result.forEach(
                           function(result){print(result._id.project+", "+result._id.month+"/1/"+result._id.year+", "+result.total);})





})
