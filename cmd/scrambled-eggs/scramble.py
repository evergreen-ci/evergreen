import json
import argparse
import hashlib

def main():
    '''takes a json file as input and destructively hashes all string values in it'''
    args = argparse.ArgumentParser()
    args.add_argument("file")
    flags = args.parse_args()

    with open(flags.file, "r") as f:
        data = json.load(f)

    traverse(data)
    
    with open(flags.file, "w") as f:
        json.dump(data, f)

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


if __name__ == "__main__":
    main()
