import os
import subprocess
import time

import argh
import requests
import yaml

endpoints = {
    "ui": 9090,
    "api": 8080,
}
default_config_file = "smoke_config.yml"
default_binary_name = "evergreen"
default_test_file = "smoke_test.yml"


def load_yaml(fn):
    """Load test definitions."""
    with open(fn) as f:
        return yaml.load(f)


def test_endpoints(tests_file=None, conf=None, binary=None):
    """Run tests against UI and API endpoints."""
    # load tests
    wd = os.getcwd()
    if not tests_file:
        tests_file = os.path.join(wd, "scripts", default_test_file)
    tests = load_yaml(tests_file)

    # wait for web service to start
    _start_evergreen(conf, binary)
    attempts = 10
    for i in range(attempts):
        try:
            r = requests.get("http://localhost:{0}".format(endpoints["ui"]))
        except requests.ConnectionError:
            print("attempt {0} of {1} failed to connect")
            if i == attempts-1:
                print("failed to connect after {0} attempts, exiting".format(attempts))
                exit(1)
            time.sleep(1)

    # check endpoints
    errors = []
    for endpoint_type, port in endpoints.iteritems():
        if endpoint_type in tests:
            for endpoint, expected in tests[endpoint_type].iteritems():
                print("Getting endpoint \"{0}\"".format(endpoint))
                r = requests.get("http://localhost:{0}{1}".format(port, endpoint))
                for text in expected:
                    if text not in r.text:
                        errors.append([endpoint_type, endpoint, text])
    if len(errors) > 0:
        print("Encountered errors contacting endpoints:")
        for error in errors:
            print("{0} endpoint \"{1}\" did not contain \"{2}\"".format(
                error[0], error[1], error[2]))
        exit(1)
    else:
        print("All endpoints checked successfully")


def _start_evergreen(conf=None, binary=None):
    wd = os.getcwd()
    if not conf:
        conf = os.path.join(wd, "scripts", default_config_file)
    if not binary:
        binary = os.path.join(wd, "scripts", default_binary_name)

    # start web service
    os.environ["EVGHOME"] = wd
    print("conf is {0}".format(conf))
    subprocess.Popen([os.path.join(wd, "scripts", "evergreen"),
                      "service",
                      "web",
                      "--conf",
                      conf])

    # start runner
    subprocess.Popen([os.path.join(wd, "scripts", "evergreen"),
                      "service",
                      "runner",
                      "--conf",
                      conf])


def start_evergreen(conf=None, binary=None):
    """Start Evergreen web service and runner."""
    _start_evergreen(conf, binary)
    while True:
        time.sleep(1)


if __name__ == "__main__":
    parser = argh.ArghParser()
    parser.add_commands([start_evergreen, test_endpoints])
    parser.dispatch()
