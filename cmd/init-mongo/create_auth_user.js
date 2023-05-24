const timeout = 30 * 1000; // 30 seconds
const start = new Date();
let isPrimary = false;

while (true) {
        let lastError = null;
        try {
                isPrimary = db.hello()['isWritablePrimary']
        } catch (error) {
                lastError = error;
        }

        const diff = (new Date()).getTime() - start.getTime();

        if (isPrimary) {
                break;
        }

        if (diff > timeout) {
                console.error("create_auth_user.js: Could not check replica set status:")
                console.error(lastError);
                exit(1)
        }
        sleep(100);
}

db = db.getSiblingDB('admin');
db.createUser(
        {
                "user": "myUserAdmin",
                "pwd": "default",
                "roles": [
                        {
                                "role": "userAdminAnyDatabase",
                                "db": "admin"
                        },
                        {
                                "role": "dbAdminAnyDatabase",
                                "db": "admin"
                        },
                        {
                                "role": "dbAdmin",
                                "db": "admin"
                        },
                        {
                                "role": "readWriteAnyDatabase",
                                "db": "admin"
                        }
                ],
                "mechanisms": [
                        "SCRAM-SHA-1",
                        "SCRAM-SHA-256"
                ]
        }
);
