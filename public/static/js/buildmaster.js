mciModule.controller('VersionMatrixController', function($scope, $window, $location){
	//data populated on page load
	var builds = $window.builds
	$scope.version = $window.version
	$scope.versionsByGitspec = $window.gitspecMap
	$scope.userTz = $window.userTz

	if ($scope.version) {
		$scope.commit = {
			message : $scope.version.message,
			author : $scope.version.author,
			author_email : $scope.version.author_email,
			push_time : $scope.version.create_time,
			gitspec : $scope.version.revision,
			repo_owner : $window.repoOwner,
			repo_name : $window.repo
		};
	} else {
		$scope.commit = null
	}

	//take the builds and matrixify them
	var taskOccurrenceCounts = {}
	var buildNumCounts = {}
	tableout = {}
	for(var i=0;i<builds.length;i++){
		var bv = builds[i].build_variant
		tableout[bv] = {}
		for(var j=0;j<builds[i].tasks.length;j++){
			var task = builds[i].tasks[j]
			tableout[bv][task.display_name] = {"current":task}
			buildNumCounts[bv] = (buildNumCounts[bv] || 0) + 1
			taskOccurrenceCounts[task.display_name] = (taskOccurrenceCounts[task.display_name] || 0) + 1
		}
	}

	var newlyFixed = []
	var newlyBroken = []

	var testdisplaynames = {}
	for(var i=0; i<version_hist.length;i++){
		var id = version_hist[i]._id
		var task_d = id.d
		testdisplaynames[task_d]=1
		var bv_n = id.v

		if(tableout[bv_n]){
			if(!tableout[bv_n][task_d]){
				tableout[bv_n][task_d] = {}
			}
			tableout[bv_n][task_d].prevTasks = version_hist[i].tests
			tableout[bv_n][task_d].prevStatus = latestStatus(version_hist[i].tests)
		}else{
			tableout[bv_n] = {}
		}
	}

	$scope.currentTask = null;
	$scope.currentCell = "asfa";
	$scope.currentBV = "";
	$scope.currentTest = "";
	$scope.variants = Object.keys(tableout).sort()

	$scope.testnames = Object.keys(testdisplaynames).sort(function(a,b){
		//Always put compile first, push last.
		if(a=='compile' || b=='push') return -1;
		if(a=='push' || b =='compile') return 1
		//Otherwise, sort by whichever task is most common
		return taskOccurrenceCounts[b] - taskOccurrenceCounts[a]
	})

	$scope.tableData = tableout

	$scope.getTooltipClass = function(testStatus){
		if(testStatus == "undispatched"){
			return "undispatched"
		}else if(testStatus == "success"){
			return "success"
		}else if(testStatus == "failed"){
			return "failed"
		}else if(testStatus == "started" || testStatus == "dispatched"){
			return "started"
		}
	}


	$scope.guessVersion = function(task_id){
		gitspec = $scope.guessGitspec(task_id)
		return $scope.versionsByGitspec[gitspec]
	}

	//This is hacky. It tries to figure out the gitspec based on a task's ID.
	//Since the build's task cache doesn't actually contain the gitspec field, this will
	//have to do for now.
	$scope.guessGitspec = function(task_id){
		if(task_id){
			parts = task_id.split('_')
			for(var i=0;i<parts.length;i++){
				if(parts[i].length == 40){
					//this was probably the gitspec
					return parts[i]
				}
			}
			return '?'
		}
		return '?'
	}

	$scope.showTaskPopover = function(bv, test, target){
		$scope.currentTask = target
		if($scope.tableData[bv] && $scope.tableData[bv][test]){
			$scope.currentCell = $scope.tableData[bv][test]
			$scope.currentBV = bv
			$scope.currentTest = test
		}else{
			$scope.currentCell = null
		}
	}

	function latestStatus(tests){
		for(var i=0;i<tests.length;i++){
			if(tests[i].s == "success"){
				return tests[i].s
			}else if(tests[i].s == "failed"){
				return "failure"
			}
		}
		return "undispatched"
	}

	$scope.highlightHeader = function(row, col){
		$('.header-cell.highlighted').removeClass('highlighted')
		$($('.header-cell').get(col)).addClass('highlighted')
		$('.tablerow .header').removeClass('highlighted')
		$($('.tablerow .header').get(row)).addClass('highlighted')
	}

	$scope.getGridClass = function(bv, test){
		var returnval = ""
		var bvRow = $scope.tableData[bv]
		if (!bvRow) return "skipped"
		var cell = bvRow[test]
		if(!cell) return "skipped"
		if(cell.current){
			if(cell.current.status == "undispatched"){ 
				returnval = "was-" + cell.prevStatus
			}else if(cell.current.status == "failed"){
				returnval = "failure"
			}else if(cell.current.status == "success"){
				returnval = "success"
			}else if(cell.current.status == "started" || cell.current.status == "dispatched"){
				returnval = "was-" + cell.prevStatus + " started"
			}
			return returnval
		}else{
			return "was-" + cell.prevStatus
		}
	}
})
