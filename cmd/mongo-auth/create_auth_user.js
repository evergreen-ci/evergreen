db = db.getSiblingDB('admin');
function createAuth(){
	db.createUser(
		{
                "user" : "myUserAdmin",
		"pwd": "default",
                "roles" : [
                        {
                                "role" : "userAdminAnyDatabase",
                                "db" : "admin"
                        },
                        {
                                "role" : "dbAdminAnyDatabase",
                                "db" : "admin"
                        },
                        {
                                "role" : "dbAdmin",
                                "db" : "admin"
                        },
                        {
                                "role" : "readWriteAnyDatabase",
                                "db" : "admin"
                        }
                ],
                "mechanisms" : [
                        "SCRAM-SHA-1",
                        "SCRAM-SHA-256"
                ]
        }
	)
};
createAuth();
