// 
// Command to be run by mongosh to ensure that a local MongoDB instance is up
//

const timeout = 30 * 1000; // 30 seconds
const start = new Date();
let connection = null;

while (true) {
	let lastError = null;
	try {
		connection = new Mongo("localhost:27017");
	} catch(error) {
		lastError = error;
	}

	const diff = (new Date()).getTime() - start.getTime();

	if (connection) {
		console.log("waitForMongo.js: Successfully connected to local MongoDB instance:")
		break;
	}

	if (diff > timeout) {
		console.error("waitForMongo.js: Could not connect to local MongoDB instance:")
		console.error(lastError);
		exit(1)
	}
	sleep(100);
}
