import pymongo
# TODO have this file take the contents of the .yml file and put it into the project_ref file. 
db = pymongo.Connection()["mci_dev"]
collection = db["project_ref"]

local_config_file = open("config_dev/project/sample.yml")
local_config = "\n".join(local_config_file.readlines())

db.project_ref.update({"identifier" : "sample"}, {"identifier" : "sample", "owner_name": "mpobrien", "branch_name": "master",
                       "repokind": "github", "enabled": True, "repo_name": "sample",
                        "tracked": True, "batch_time": 120, "local_config" : local_config, "remote": False},upsert=True)
