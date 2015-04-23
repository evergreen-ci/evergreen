#import boto
#s3 = bot
#ec2 = boto.ec2.connect_to_region('us-east-1')


import itertools
import boto
import pymongo

db = pymongo.Connection()['mci']
ec2 = boto.connect_ec2()
chain = itertools.chain.from_iterable
all_instances = list(chain([res.instances for res in ec2.get_all_instances()]))
all_instance_ids = set(instance.id for instance in all_instances)
#print existing_instances #gives you a list of all instances

motuhosts = list(db.hosts.find())
motuhostsmap = {}
for h in motuhosts:
    motuhostsmap[h['_id']] = h['status']
    


for instance in all_instances:
    if instance.id not in motuhostsmap:
        print "instance %s is NOT in motu DB (%s)" % (instance.id, len(instance.tags))
    if instance.id in motuhostsmap:
        print "instance %s is in motu DB with status: %s" % (instance.id, motuhostsmap[instance.id])




