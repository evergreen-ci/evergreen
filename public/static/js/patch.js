var mci = (function() {

    var patchInfo = {};
    var DEFAULT_LIMIT = 10;

    function sortBuildsWithinVersions() {
        $('.patch-builds-list').each(function(idx, buildsList) {
            var list = $(this);
            var sortedChildren = list.children('.patch-build:not(.row-header)').sort(function(a, b) {
                var variantA = $(a).children('.build-link').text().trim();
                var variantB = $(b).children('.build-link').text().trim();
                return (variantA < variantB) ? -1 : (variantA > variantB ? 1 : 0);
            });
            list.children('.patch-build:not(.row-header)').remove();
            list.append(sortedChildren);
        });
    }

    function addLinksAndStyles() {

        function toBuildPage(buildId) {
            common.toRef(mci.urlPrefix + 'build/' + buildId);
        }

        function toTaskPage(taskId) {
            common.toRef(mci.urlPrefix + 'task/' + taskId);
        }

        var versions = patchInfo.Versions;
        for (var v_i = 0, v_l = versions.length; v_i < v_l; v_i++) {
            var version = versions[v_i];
            if (!version.Version.activated) {
                continue;
            }
            var builds = version.Builds;
            for (var b_i = 0, b_l = builds.length; b_i < b_l; b_i++) {
                var build = builds[b_i];
                var buildDomElement = $('#'+build.Build.Id);
                (function() {
                    var buildId = build.Build.Id;
                    buildDomElement.click(function() {
                        toBuildPage(buildId);
                    });
                })();
                var tasks = build.Tasks;
                for (var t_i = 0, t_l = tasks.length; t_i < t_l; t_i++) {
                    var task = tasks[t_i];
                    var taskDomElement = $('#'+task.Task.Id);
                    taskDomElement.tooltip({
                        'placement': 'top',
                        'title': task.Task.DisplayName,
                        'animation':false,
                    });
                    (function() {
                        var taskId = task.Task.Id;
                        taskDomElement.click(function() {
                            toTaskPage(taskId);
                        });
                    })();
                }
            }
        }

        // make all gitspec links point to the right location
        $(".git-link").each(function(index, element) {
            var elements = $(element).attr('id').split('-');
            $(element).attr('href', 'https://github.com/' + elements[1] + '/' + elements[2] + 
                (elements[3] == 'master' ? '' : '/tree/' + elements[3]) + 
                '/commit/' + elements[4]);
        });
    }

    function activatePaginationButtons() {

        function skipValFromUrl() {
            return common.urlParam('skip') || '0';
        }

        function limitFromUrl() {
            return common.urlParam('limit') || '10';
        }

        $('#most-recent').click(function() {
            common.toRef(mci.urlPrefix + '?skip=0');
        });

        if (parseInt(skipValFromUrl(), 10) == 0) {
            $('#previous').addClass('disabled');
        } else {
            $('#previous').click(function() {
                var skip = parseInt(skipValFromUrl(), 10) - DEFAULT_LIMIT;
                common.toRef(mci.urlPrefix + '?skip=' + (skip < 0 ? 0 : skip));
            });
        }

        if (parseInt(skipValFromUrl(), 10) + DEFAULT_LIMIT >= patchInfo.TotalVersions) {
            $('#next').addClass('disabled');
        } else {
            $('#next').click(function() {
                var skip = parseInt(skipValFromUrl(), 10) + DEFAULT_LIMIT;
                common.toRef(mci.urlPrefix + '?skip=' + (skip < 0 ? 0 : skip));
            });
        }

        $('#least-recent').click(function() {
            var totalVersions = patchInfo.TotalVersions;
            var lastPageBegin = totalVersions - (totalVersions%DEFAULT_LIMIT);
            common.toRef(mci.urlPrefix + '?skip=' + lastPageBegin);
        });
    }

    function pollForpatch() {
        var skipParam = common.urlParam('skip') || 0;
        var limitParam = common.urlParam('limit') || 10;
        $.ajax({
            'url': '/patch_content?skip=' + skipParam + '&limit' + limitParam,
            'success': function(data, err) {
                $('#content').html(data);
            },
            'error': function(jqXHR, status, errorThrown) {
                console.log('Error polling: ' + errorThrown);
                console.log('Status: ' + status);
            } 
        });
    }

    var mci = {
        urlPrefix: '/',

        loadpatchInfo: function(info) {
            patchInfo = info;
        },

        activatepatch: function() {
            sortBuildsWithinVersions();
            addLinksAndStyles();
            activatePaginationButtons();

            // for reloading the patch view (prevents refreshing)
            //setInterval(pollForpatch, 2000);
        }
    };

    return mci;
})();
