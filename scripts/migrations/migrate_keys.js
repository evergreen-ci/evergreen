var keys = [{local: 'vars.aws_key', remote: 'project_aws_key'},
        {local: 'vars.aws_secret', remote: 'project_aws_secret'}]

var projects =  ['mci', 'mongo-cap','mongodb-mongo-master-coverage',
        'mongodb-mongo-master', 'mongodb-mongo-v2.6' , 'node-mongodb-native-3.0',
        'rocksdb', 'mongo-tools', 'mongo-c-driver', 'mongo-cpp-driver', 'mongoscope', 'mongodb-mongo-v3.0',
        'toolchain-builder', 'mongodb-mongo-master-sanitize', 'mongoimport', 'mongoexport', 'mongodump', 'mongotop', 'mongodb-mongo-v2.6.3']

for (x = 0; x < projects.length; x++){
  var project_name = projects[x]
  for (var i = 0; i < keys.length; i++) {
    key = keys[i]
    var project = db.project_vars.findOne({'_id' : project_name})
    if (project){
      if (project.vars[key.remote]) {
        var remote_key = project.vars[key.remote]
        var pairing = {}
        pairing[key.local] = remote_key
        db.project_vars.update({'_id' : project_name}, {'$set' : pairing})
      }
  }
  }
}

// set the signing auth 2.6 key
var project_name = 'mongodb-mongo-v2.6'
var project = db.project_vars.findOne({'_id' : project_name})
if (project){
  db.project_vars.update({'_id' : project_name}, {'$set' : {'vars.signing_auth_token' : project.vars['signing-auth-token-2-6']}})
}

// set the signing auth 2.6.3 key
var project_name = 'mongodb-mongo-v2.6.3'
var project = db.project_vars.findOne({'_id' : project_name})
if (project){
  db.project_vars.update({'_id' : project_name}, {'$set' : {'vars.signing_auth_token' : project.vars['signing-auth-token-2-6']}})
}

// set the signing auth 3.0 key with 'signing-auth-2-8'
var project_name = 'mongodb-mongo-master'
var project = db.project_vars.findOne({'_id' : project_name})
if (project) {
  db.project_vars.update({'_id' : project_name}, {'$set' : {'vars.signing_auth_token' : project.vars['signing-auth-token-2-8']}})
}

// set the signing auth 3.0 key with 'signing-auth-2-8'
var project_name = 'mongodb-mongo-v3.0'
var project = db.project_vars.findOne({'_id' : project_name})
if (project) {
  db.project_vars.update({'_id' : project_name}, {'$set' : {'vars.signing_auth_token' : project.vars['signing-auth-token-2-8']}})
}


var project_name = 'rocksdb'
var project = db.project_vars.findOne({'_id' : project_name})
if (project) {
  db.project_vars.update({'_id' : project_name}, {'$set' : {'vars.signing_auth_token' : project.vars['signing-auth-token-2-8']}})
}
