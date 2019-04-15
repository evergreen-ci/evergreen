// Karma configuration
// Generated on Tue Sep 10 2013 18:10:11 GMT-0400 (EDT)

// Install karma via npm install karma -g to run

module.exports = function(config) {
  config.set({

    // base path, that will be used to resolve files and exclude
    basePath: '../../../',


    // frameworks to use
    frameworks: ['jasmine'],


    // list of files / patterns to load in the browser
    files: [
      'thirdparty/angular.min.js',
      'thirdparty/angular-*.js',
      'thirdparty/md-*.js',
      'thirdparty/underscore-min.js',
      'thirdparty/d3.min.js',
      'thirdparty/jquery.js',
      'thirdparty/ui-grid.min.js',
      'thirdparty/select.min.js',
      'thirdparty/moment.min.js',
      'js/mci_module.js',
      'js/**/*.js',
      'plugins/*/js/*.js',
      'app/!(waterfall)/*.js',
    ],


    // list of files to exclude
    exclude: [
    ],

    // test results reporter to use
    // possible values: 'dots', 'progress', 'junit', 'growl', 'coverage'
    reporters: ['progress'],


    // web server port
    port: 9876,


    // enable / disable colors in the output (reporters and logs)
    colors: true,


    // level of logging
    // possible values: config.LOG_DISABLE || config.LOG_ERROR || config.LOG_WARN || config.LOG_INFO || config.LOG_DEBUG
    logLevel: config.LOG_INFO,


    // enable / disable watching file and executing tests whenever any file changes
    autoWatch: true,

    junitReporter: {
      outputDir: '../../bin/jstests', // results will be saved as $outputDir/$browserName.xml
      outputFile: undefined, // if included, results will be saved as $outputDir/$browserName/$outputFile
      suite: '', // suite will become the package name attribute in xml testsuite element
      useBrowserName: false, // add browser name to report and classes names
      nameFormatter: undefined, // function (browser, result) to customize the name attribute in xml testcase element
      classNameFormatter: undefined, // function (browser, result) to customize the classname attribute in xml testcase element
      properties: {}, // key value pair of properties to add to the <properties> section of the report
      xmlVersion: null // use '1' if reporting to be per SonarQube 6.2 XML format
    },

    plugins: [
      require('karma-junit-reporter'),
      require('jasmine'),
      require('karma-jasmine'),
      require('karma-jsdom-launcher')
    ],

    // Start these browsers
    browsers: ['jsdom'],


    // If browser does not capture in given timeout [ms], kill it
    captureTimeout: 60000,


    // Continuous Integration mode
    // if true, it capture browsers, run tests and exit
    singleRun: true
  });
};
