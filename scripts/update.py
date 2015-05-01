'''
Deployment plan
- turn off taskrunner
- turn off scheduler
- wait for all dispatched tasks to start
- run update.js (this script)
- pull new code on motu
- rebuild agent
- restart api/ui server
- turn on taskrunner
- watch/monitor
- turn on scheduler
- watch/monitor
'''

import pymongo

# update host documents with new distros
db = pymongo.Connection()["mci"]
db.distro.update({}, {"$set":{"work_dir": "/data/mci"}}, False, True)
hosts = db.hosts.find({"status": {"$ne": "terminated"}}, {"_id: 1"})

for host in hosts:
	err = db.hosts.update({"_id": host["_id"]}, {"$set": {"distro.work_dir": "/data/mci"}})
	if err is not None:
		print "error updating host '%s': %s" %(host["_id"], err)
print "done!"