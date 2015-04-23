module.exports = function(grunt) {
    grunt.initConfig({
        pkg: grunt.file.readJSON('package.json'),

        watch: {
            files: ['static/less/**'],
            tasks: ['css']
        },

        less: {
            main: {
                options: {
                    paths: ['static/less'],
                    sourceMap: true,
                    sourceMapFilename: 'static/dist/less.map',
                    sourceMapURL: '/static/dist/less.map',
                    sourceMapRootpath: '../../'
                },
                files: {
                    'static/dist/css/styles.css': 'static/less/main.less'
                }
            }
        },

        cssmin: {
            combine: {
                files: {
                    'static/dist/css/styles.min.css': 'static/dist/css/styles.css'
                }
            }
        }
    });

    grunt.loadNpmTasks('grunt-contrib-less');
    grunt.loadNpmTasks('grunt-contrib-cssmin');
    grunt.loadNpmTasks('grunt-contrib-watch');

    grunt.registerTask('css', ['less', 'cssmin']);

    grunt.registerTask('default', ['css']);
};
