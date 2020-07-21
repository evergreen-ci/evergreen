import json
import argparse
import hashlib

# this script takes a json file as input and destructively hashes all string values in it
def main():
    args = argparse.ArgumentParser()
    args.add_argument("file")
    flags = args.parse_args()

    with open(flags.file, "r") as f:
        data = json.load(f)
        f.close()

    traverse(data)
    
    with open(flags.file, "w") as f:
        json.dump(data, f)
        f.close()

def traverse(obj):
    if isinstance(obj, dict):
        for key, value in obj.items():
            if isinstance(value, dict):
                traverse(value)
            elif isinstance(value, list) or isinstance(value, tuple):
                for i,v in enumerate(value):
                    value[i] = traverse(v)
                obj[key] = value
            else:
                if (isinstance(value, str) or isinstance(value, unicode)):
                    obj[key] = hashlib.sha256(value).hexdigest()
    else:
        if (isinstance(obj, str) or isinstance(obj, unicode)):
            return hashlib.sha256(obj).hexdigest()


main()
