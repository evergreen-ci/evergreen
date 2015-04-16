'''
Migration plan
- turn off taskrunner
- turn off scheduler
- turn off hostinit
- turn off monitor
- wait for all dispatched tasks to fetch distro
- wait for all hostinit jobs to complete
- run migrateHosts.js (this script)
- pull new code on motu
- rebuild agent
- rebuild/restart api/ui server
- watch/monitor api/ui server
- turn on taskrunner
- watch/monitor - ensure all tasks are starting properly
- turn on scheduler
- watch/monitor - ensure there're no snafus
- turn on hostinit
- watch/monitor - ensure hosts are moving from starting - provisioned - running
- turn on monitor
- watch/monitor - ensure it isn't bugging out
'''

import pymongo
import subprocess

# import new distros
p = subprocess.Popen(["mongoimport", "-d", "mci", "-c", "distro", "scripts/distros.json"], stdout=subprocess.PIPE,stderr=subprocess.PIPE)

p.wait()
print p.stdout.read()
print p.stderr.read()

# update host documents with new distros
db = pymongo.Connection()["mci"]
distro_ids = db.hosts.distinct("distro_id")
for _id in distro_ids:
	distro = db.distro.find_one({"_id": _id})
	if distro is None:
		print "no distro found for %s..." %_id
	else:
		print "updating hosts for %s distro.." %_id
		err = db.hosts.update({"distro_id": _id}, {"$set": {"distro": distro}}, upsert=True, multi=True)
		if err is not None:
			print "error updating hosts for distro '%s': %s" %(_id, err)
print "creating index on 'distros._id' in hosts collection"
db.hosts.create_index("distro._id")
