import pymongo

db = pymongo.Connection()['mci']

db.hosts.update({"started_by": "mci"}, {"$set": {"user_host": True}})
db.hosts.update({"started_by": {"$ne": "mci"}}, {"$set": {"user_host": False}})

