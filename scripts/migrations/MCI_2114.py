from __future__ import print_function
import traceback
import pymongo

from datetime import datetime, timedelta

db = pymongo.MongoClient()['mci']

i = 0

# keys into version doc
IDS = "builds"
VARIANTS= "build_variants"
BATCH_TIME = "batch_time"
ACTIVATED = "activated"

VARIANTS_STATUS = "build_variants_status"

def build_variants_status_from_patch_version(doc):
    bvs = []
    for (build_id, build_variant) in zip(version[IDS], version[VARIANTS]):
        activated = version[ACTIVATED]
        build = db.builds.find_one({"_id": build_id})
        if build != None:
            if "activated" in build:
                activated = build["activated"]
            
            # due to some other bug, we might as well canonicalize the build id (if we can)
            # since it could be the display name
            if "build_variant" in build:
                build_variant = build["build_variant"]

        bvs.append({
            "build_variant": build_variant,
            "activated": activated,
            "build_id": build_id,
        })
    return bvs

def build_variants_status_from_gitter_version(doc):
    bvs = []
    for (build_id) in version[IDS]:
        # previously 
        activated = version[ACTIVATED]
        build = db.builds.find_one({"_id": build_id})

        assert (build != None) # protect against inconsistent data

        if "activated" in build:
            activated = build["activated"]
        # due to a bug in mci, the build array in versions created by the repotracker
        # previously cointained the display names of the variants
        # rather than their ids
        assert "build_variant" in build
        build_variant = build["build_variant"]

        bv = {
            "build_variant": build_variant,
            "activated": activated,
            "build_id": build_id
        }

        if BATCH_TIME in version:
            bv["activate_at"] = datetime.utcnow() + timedelta(minutes=version[BATCH_TIME])

        bvs.append(bv)

    return bvs

failures = 0
successes = 0

for version in db.versions.find():
    hasVariants = VARIANTS in version
    hasBuildIds = IDS in version
    hasBatchTime = BATCH_TIME in version
    hasActivated = ACTIVATED in version

    bvs = {}
    try:
        if version["r"] == "gitter_request":
            bvs = build_variants_status_from_gitter_version(version)
        elif version["r"] == "patch_request":
            bvs = build_variants_status_from_patch_version(version)
        else:
            assert False
        db.versions.update({"_id": version["_id"]}, {"$set": {VARIANTS_STATUS: bvs}})
    except:
        print(traceback.format_exc())
        failures += 1
        print("Failed to migrate version! Printing id ...")
        print("=====================================")
        print(version["_id"])
        print("======================================")
        continue 
    successes += 1

print("Migrated {} docs with {} failures".format(successes, failures))
