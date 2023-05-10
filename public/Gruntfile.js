module.exports = function (grunt) {
  require("load-grunt-tasks")(grunt);
  grunt.initConfig({
    pkg: grunt.file.readJSON("package.json"),

    babel: {
      options: {
        sourceMap: true,
        plugins: ["transform-react-jsx"],
        presets: ["env"],
      },
      dist: {
        files: [
          {
            expand: true,
            cwd: "static/app",
            src: ["**/*.jsx"],
            dest: "static/dist/js",
            ext: ".js",
          },
        ],
      },
    },

    watch: {
      css: {
        files: ["static/less/**"],
        tasks: ["css"],
      },
      react: {
        files: ["static/app/**/*.jsx"],
        tasks: ["babel"],
      },
    },

    less: {
      main: {
        options: {
          paths: ["static/less"],
          sourceMap: true,
          sourceMapFilename: "static/dist/less.map",
          sourceMapURL: "/static/dist/less.map",
          sourceMapRootpath: "../../",
        },
        files: {
          "static/dist/css/styles.css": "static/less/main.less",
        },
      },
    },

    cssmin: {
      combine: {
        files: {
          "static/dist/css/styles.min.css": "static/dist/css/styles.css",
        },
      },
    },
  });

  grunt.loadNpmTasks("grunt-babel");
  grunt.loadNpmTasks("grunt-contrib-less");
  grunt.loadNpmTasks("grunt-contrib-cssmin");
  grunt.loadNpmTasks("grunt-contrib-watch");

  grunt.registerTask("css", ["less", "cssmin"]);
  grunt.registerTask("default", ["css"]);
};
