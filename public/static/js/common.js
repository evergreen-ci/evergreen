var common = (function() {
    return {

        getBuildResultString: function(status) {
            switch (status) {
            case 'success':
                return 'Build successful';
            case 'failed':
                return 'Build failed';
            case 'cancelled':
                return 'Build cancelled';
            case 'started':
                return 'Build in progress';
            case 'unstarted': 
                return 'Build not started';
            }

            return 'Build status unknown';
        },

        getTaskResultString: function(status) {
            switch (status) {
            case 'success':
                return 'Task completed successfully';
            case 'failed':
                return 'Task failed';
            case 'cancelled':
                return 'Task cancelled';
            case 'started':
                return 'Task in progress';
            case 'dispatched':
                return 'Task in progress';
            case 'undispatched': 
                return 'Task not yet started';
            }

            return 'Task status unknown';
        },

        isStarted: function(status) {
            return status !== 'unstarted' && status !== 'undispatched';  
        },

        isFinished: function(status) {
            return status === 'success' || status === 'failed';
        },

        makeDateReadable: function(date) {
            var YMD = date.substring(0, date.indexOf('T'));
            var year = YMD.substring(0, YMD.indexOf('-'));
            var month = this.getMonth(YMD);
            var day = this.getDay(YMD);
            var start = date.indexOf('T')+1
            var time = date.substring(start, start+8);
            return month + ' ' + day + ', ' + year + ' ' + time ;   
        },

        getMonth: function(YMD) {
            var monthNum = YMD.substring(YMD.indexOf('-') + 1, YMD.lastIndexOf('-'));
            switch (monthNum) {
            case '01':
                return 'January';
            case '02':
                return 'February';
            case '03':
                return 'March';
            case '04':
                return 'April';
            case '05':
                return 'May';
            case '06':
                return 'June';
            case '07':
                return 'July';
            case '08':
                return 'August';
            case '09':
                return 'September';
            case '10':
                return 'October';
            case '11':
                return 'November';
            case '12':
                return 'December';
            }    
        },

        getDay: function(YMD) {
            return YMD.substring(YMD.lastIndexOf('-') + 1, YMD.length);
        },

        urlParam: function(name) {
            var results = new RegExp('[\\?&amp;]' + name + '=([^&amp;#]*)').exec(window.location.href);
            if (!results) {
                return 0
            }
            return results[1];   
        },

        toRef: function(ref) {
            window.location.href = ref;
        }
    }
})();