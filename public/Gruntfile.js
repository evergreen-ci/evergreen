module.exports = function(grunt) {

  require('load-grunt-tasks')(grunt);

    grunt.initConfig({
        pkg: grunt.file.readJSON('package.json'),

        babel: {
          options: {
            sourceMap: true,
            plugins: ['transform-react-jsx'],
            presets: ['es2015', 'react']
          }
        },

        react: {
          dynamic_mappings: {
            files: [{
              expand: true,
              cwd: 'static/js/jsx',
              src: ['**/*.jsx'],
              dest: 'static/js',
              ext: '.js'
            }]
          }
        },

        watch: {
          css: {
            files: ['static/less/**'],
            tasks: ['css']
          },
          react: {
            files: ['static/js/jsx/**'],
            tasks: ['react']
          }
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
    grunt.loadNpmTasks('grunt-react');

    grunt.registerTask('css', ['less', 'cssmin']);
    grunt.registerTask('default', ['css']);
};
