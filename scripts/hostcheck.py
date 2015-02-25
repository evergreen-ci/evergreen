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

whitelist = []

motuhosts = list(db.hosts.find())
motuhostsmap = {}
for h in motuhosts:
    motuhostsmap[h['_id']] = h['status']

spots = list(ec2.get_all_spot_instance_requests())
i_to_sir = {}
for spot in spots:
    if spot.instance_id:
        i_to_sir[spot.instance_id] = spot.id

for instance in all_instances:
    if instance.id in i_to_sir:
        instance.id = i_to_sir[instance.id]
    if instance.id in whitelist:
        print "instance %s is in whitelist" % (instance.id))
    if instance.id not in motuhostsmap:
        print "instance %s is in AWS with status 'AWS:%s' NOT in motu DB, tags are %s" % (instance.id, instance.state, repr(instance.tags))
    if instance.id in motuhostsmap:
        print "instance %s is in motu DB with status: %s" % (instance.id, motuhostsmap[instance.id])
