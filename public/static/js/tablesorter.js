/*
 *
 * TableSorter 2.0 - Client-side table sorting with ease!
 * Version 2.0.5d (update)
 * @requires jQuery v1.2.3
 *
 * Copyright (c) 2007 Christian Bach
 * Examples and docs at: http://tablesorter.com
 * Dual licensed under the MIT and GPL licenses:
 * http://www.opensource.org/licenses/mit-license.php
 * http://www.gnu.org/licenses/gpl.html
 *
 */
/**
 *
 * @description Create a sortable table with multi-column sorting capabilitys
 *
 * @example $('table').tablesorter();
 * @desc Create a simple tablesorter interface.
 *
 * @example $('table').tablesorter({ sortList:[[0,0],[1,0]] });
 * @desc Create a tablesorter interface and sort on the first and secound column in ascending order.
 *
 * @example $('table').tablesorter({ headers: { 0: { sorter: false}, 1: {sorter: false} } } });
 * @desc Create a tablesorter interface and disableing the first and secound column headers.
 *
 * @example $('table').tablesorter({ headers: { 0: {sorter: "digit"}, 1: {sorter: "currency"} } });
 * @desc Create a tablesorter interface and set a column parser for the first and secound column.
 *
 *
 * @param Object settings An object literal containing key/value pairs to provide optional settings.
 *
 * @option String cssHeader (optional)          A string of the class name to be appended to sortable tr elements in the thead of the table. 
 *                                              Default value: "header"
 *
 * @option String cssAsc (optional)             A string of the class name to be appended to sortable tr elements in the thead on a ascending sort. 
 *                                              Default value: "headerSortUp"
 *
 * @option String cssDesc (optional)            A string of the class name to be appended to sortable tr elements in the thead on a descending sort. 
 *                                              Default value: "headerSortDown"
 *
 * @option String sortInitialOrder (optional)   A string of the inital sorting order can be asc or desc. 
 *                                              Default value: "asc"
 *
 * @option String sortMultisortKey (optional)   A string of the multi-column sort key. 
 *                                              Default value: "shiftKey"
 *
 * @option String textExtraction (optional)     A string of the text-extraction method to use. 
 *                                              For complex html structures inside td cell set this option to "complex", 
 *                                              on large tables the complex option can be slow. 
 *                                              Default value: "simple"
 *
 * @option Object headers (optional)            An array containing the forces sorting rules. 
 *                                              This option let's you specify a default sorting rule. 
 *                                              Default value: null
 *
 * @option Array sortList (optional)            An array containing the forces sorting rules. 
 *                                              This option let's you specify a default sorting rule. 
 *                                              Default value: null
 *
 * @option Array sortForce (optional)           An array containing forced sorting rules. 
 *                                              This option let's you specify a default sorting rule, which is prepended to user-selected rules.
 *                                              Default value: null
 *
 * @option Array sortAppend (optional)          An array containing forced sorting rules. 
 *                                              This option let's you specify a default sorting rule, which is appended to user-selected rules.
 *                                              Default value: null
 *
 * @option Boolean widthFixed (optional)        Boolean flag indicating if tablesorter should apply fixed widths to the table columns.
 *                                              This is usefull when using the pager companion plugin.
 *                                              This options requires the dimension jquery plugin.
 *                                              Default value: false
 *
 * @option Boolean cancelSelection (optional)   Boolean flag indicating if tablesorter should cancel selection of the table headers text.
 *                                              Default value: true
 *
 * @option Boolean locale (optional)            A locale String indicating which date format and decimal point to use.
 *                                              Default value: us
 *
 * @option Boolean debug (optional)             Boolean flag indicating if tablesorter should display debuging information usefull for development.
 *
 * @option Boolean useUI (optional)             Boolean flag indicating if tablesorter use the ui theme classes.
 *                                              Default value: false
 * @type jQuery
 *
 * @name tablesorter
 *
 * @cat Plugins/Tablesorter
 *
 * @author Christian Bach/christian.bach@polyester.se
 */

(function($) {
  $.extend({
    tablesorter: new function() {

      var parsers = [],
        widgets = [];

      this.defaults = {
        cssHeader: "header",
        cssAsc: "headerSortUp",
        cssDesc: "headerSortDown",
        cssChildRow: "expand-child",
        cssUI: {
          widget: "ui-widget ui-widget-content ui-corner-all",
          header: "ui-widget-header ui-corner-all",
          hover: "ui-state-hover",
          icon: "ui-icon",
          iconBoth: "ui-icon-arrowthick-2-n-s",
          iconDesc: "ui-icon-arrowthick-1-n",
          iconAsc: "ui-icon-arrowthick-1-s"
        },
        sortInitialOrder: "asc",
        sortMultiSortKey: "shiftKey",
        sortForce: null,
        sortAppend: null,
        textExtraction: "simple",
        parsers: {},
        widgets: [],
        widgetZebra: {
          css: ["even", "odd"]
        },
        headers: {},
        widthFixed: false,
        cancelSelection: true,
        sortList: [],
        headerList: [],
        locale: "us",
        format: {
          us: {
            decimal: '.',
            date: '/'
          },
          en: {
            decimal: '.',
            date: '/'
          },
          eu: {
            decimal: ',',
            date: '.'
          },
          de: {
            decimal: ',',
            date: '.'
          }
        },
        onRenderHeader: null,
        selectorHeaders: 'thead th',
        useUI: false,
        debug: false
      };

      /* debuging utils */
      function benchmark(s, d) {
        log(s + "," + (new Date().getTime() - d.getTime()) + "ms");
      }

      this.benchmark = benchmark;

      function log(s) {
        if (typeof console != "undefined" && typeof console.debug != "undefined") {
          console.log(s);
        } else {
          alert(s);
        }
      }

      /* parsers utils */
      function buildParserCache(table, $headers) {

        if (table.config.debug) {
          var parsersDebug = "";
        }

        var rows = table.tBodies[0].rows;

        if (table.tBodies[0].rows[0]) {

          var list = [],
            cells = rows[0].cells,
            l = cells.length;

          for (var i = 0; i < l; i++) {
            var p = false;

            if ($.metadata && ($($headers[i]).metadata() && $($headers[i]).metadata().sorter)) {

              p = getParserById($($headers[i]).metadata().sorter);

            } else if ((table.config.headers[i] && table.config.headers[i].sorter)) {

              p = getParserById(table.config.headers[i].sorter);
            }
            if (!p) {
              p = detectParserForColumn(table, cells[i]);
            }

            if (table.config.debug) {
              parsersDebug += "column:" + i + " parser:" + p.id + "\n";
            }

            list.push(p);
          }
        }

        if (table.config.debug) {
          log(parsersDebug);
        }

        return list;
      };

      function detectParserForColumn(table, node) {
        var l = parsers.length;
        for (var i = 1; i < l; i++) {
          if (parsers[i].is($.trim(getElementText(table.config, node)), table, node)) {
            return parsers[i];
          }
        }
        // 0 is always the generic parser (text)
        return parsers[0];
      }

      function getParserById(name) {
        var l = parsers.length;
        for (var i = 0; i < l; i++) {
          if (parsers[i].id.toLowerCase() == name.toLowerCase()) {
            return parsers[i];
          }
        }
        return false;
      }

      /* utils */
      function buildCache(table) {

        if (table.config.debug) {
          var cacheTime = new Date();
        }


        var totalRows = (table.tBodies[0] && table.tBodies[0].rows.length) || 0,
          totalCells = (table.tBodies[0].rows[0] && table.tBodies[0].rows[0].cells.length) || 0,
          parsers = table.config.parsers,
          cache = {
            row: [],
            normalized: []
          };

        for (var i = 0; i < totalRows; ++i) {

          /** Add the table data to main data array */
          var c = $(table.tBodies[0].rows[i]),
            cols = [];

          // if this is a child row, add it to the last row's children and continue to the next row
          if (c.hasClass(table.config.cssChildRow)) {
            cache.row[cache.row.length - 1] = cache.row[cache.row.length - 1].add(c);
            // go to the next for loop
            continue;
          }

          cache.row.push(c);

          for (var j = 0; j < totalCells; ++j) {
            cols.push(parsers[j].format(getElementText(table.config, c[0].cells[j]), table, c[0].cells[j]));
          }

          cols.push(cache.normalized.length); // add position for rowCache
          cache.normalized.push(cols);
          cols = null;
        };

        if (table.config.debug) {
          benchmark("Building cache for " + totalRows + " rows:", cacheTime);
        }

        return cache;
      };

      function getElementText(config, node) {

        if (!node) return "";

        var t = "";

        if (config.textExtraction == "simple") {
          if (node.childNodes[0] && node.childNodes[0].hasChildNodes()) {
            t = node.childNodes[0].innerHTML;
          } else {
            t = node.innerHTML;
          }
        } else {
          if (typeof(config.textExtraction) == "function") {
            t = config.textExtraction(node);
          } else {
            t = $(node).text();
          }
        }
        return t;
      }

      function appendToTable(table, cache) {

        if (table.config.debug) {
          var appendTime = new Date();
        }

        var c = cache,
          r = c.row,
          n = c.normalized,
          totalRows = n.length,
          checkCell = (n[0].length - 1),
          tableBody = $(table.tBodies[0]),
          rows = [];

        for (var i = 0; i < totalRows; i++) {
          var pos = n[i][checkCell];
          rows.push(r[pos]);
          if (!table.config.appender) {

            var o = r[pos];
            var l = o.length;
            for (var j = 0; j < l; j++) {
              tableBody[0].appendChild(o[j]);
            }

            //tableBody.append(r[n[i][checkCell]]);
          }
        }

        if (table.config.appender) {

          table.config.appender(table, rows);
        }

        rows = null;

        if (table.config.debug) {
          benchmark("Rebuilt table:", appendTime);
        }

        //apply table widgets
        applyWidget(table);

        // trigger sortend
        setTimeout(function() {
          $(table).trigger("sortEnd");
        }, 0);

      };

      function buildHeaders(table) {

        if (table.config.debug) {
          var time = new Date();
        }

        var config = table.config;
        var meta = ($.metadata) ? true : false; //, tableHeadersRows = [];

        //for(var i = 0; i < table.tHead.rows.length; i++) { tableHeadersRows[i]=0; };
        $tableHeaders = $(config.selectorHeaders, table).each(function(index) {

          this.column = index;
          this.order = formatSortingOrder(config.sortInitialOrder);
          this.count = this.order;

          if (checkHeaderMetadata(this) || checkHeaderOptions(table, index)) this.sortDisabled = true;

          if (!this.sortDisabled) {
            if (config.useUI) {
              // add span element
              $(this).prepend('<span></span>').children("span").addClass(config.cssUI.icon);
              $(this).hover(function() {
                $(this).addClass(config.cssUI.hover);
              }, function() {
                $(this).removeClass(config.cssUI.hover);
              });
            } else {
              if (config.onRenderHeader) config.onRenderHeader.apply(this);
            }
          }

          // add cell to headerList
          config.headerList[index] = this;
        });

        if (table.config.debug) {
          benchmark("Built headers:", time);
          log($tableHeaders);
        }

        return $tableHeaders;

      };

      function setupCss(table, $headers) {
        var c = table.config;

        if (c.useUI) {
          $(table).parent("div:first").addClass(c.cssUI["widget"]);
          $headers.addClass(c.cssUI.header);
          return [c.cssUI.iconDesc, c.cssUI.iconAsc, c.cssUI.iconBoth];

        } else {
          $headers.addClass(c.cssHeader);
          return [c.cssDesc, c.cssAsc];
        }
      }

      function checkCellColSpan(table, rows, row) {
        var arr = [],
          r = table.tHead.rows,
          c = r[row].cells;

        for (var i = 0; i < c.length; i++) {
          var cell = c[i];

          if (cell.colSpan > 1) {
            arr = arr.concat(checkCellColSpan(table, headerArr, row++));
          } else {
            if (table.tHead.length == 1 || (cell.rowSpan > 1 || !r[row + 1])) {
              arr.push(cell);
            }
            //headerArr[row] = (i+row);
          }
        }
        return arr;
      };

      function checkHeaderMetadata(cell) {
        if (($.metadata) && ($(cell).metadata().sorter === false)) {
          return true;
        };
        return false;
      }

      function checkHeaderOptions(table, i) {
        if ((table.config.headers[i]) && (table.config.headers[i].sorter === false)) {
          return true;
        };
        return false;
      }

      function applyWidget(table) {
        var c = table.config.widgets;
        var l = c.length;
        for (var i = 0; i < l; i++) {

          getWidgetById(c[i]).format(table);
        }

      }

      function getWidgetById(name) {
        var l = widgets.length;
        for (var i = 0; i < l; i++) {
          if (widgets[i].id.toLowerCase() == name.toLowerCase()) {
            return widgets[i];
          }
        }
      };

      function formatSortingOrder(v) {
        if (typeof(v) != "Number") {
          return (v.toLowerCase() == "desc") ? 1 : 0;
        } else {
          return (v == 1) ? 1 : 0;
        }
      }

      function isValueInArray(v, a) {
        var l = a.length;
        for (var i = 0; i < l; i++) {
          if (a[i][0] == v) {
            return true;
          }
        }
        return false;
      }

      function setHeadersCss(table, $headers, list, css) {
        var c = table.config;
        // remove all header information
        if (c.useUI) {
          $headers.children("span").removeClass(css[0]).removeClass(css[1]).addClass(css[2]);
        } else {
          $headers.removeClass(css[0]).removeClass(css[1]);
        }

        var h = [];
        $headers.each(function(offset) {
          if (!this.sortDisabled) {
            h[this.column] = $(this);
          }
        });

        var l = list.length;
        for (var i = 0; i < l; i++) {
          if (c.useUI) {
            h[list[i][0]].children("span").removeClass(css[2]).addClass(css[list[i][1]]);
          } else {
            h[list[i][0]].addClass(css[list[i][1]]);
          }
        }
      }

      function fixColumnWidth(table, $headers) {
        var c = table.config;
        if (c.widthFixed) {
          var colgroup = $('<colgroup>');
          $("tr:first td", table.tBodies[0]).each(function() {
            colgroup.append($('<col>').css('width', $(this).width()));
          });
          $(table).prepend(colgroup);
        };
      }

      function updateHeaderSortCount(table, sortList) {
        var c = table.config,
          l = sortList.length;
        for (var i = 0; i < l; i++) {
          var s = sortList[i],
            o = c.headerList[s[0]];
          o.count = s[1];
          o.count++;
        }
      }

      /* sorting methods */
      function multisort(table, sortList, cache) {

        if (table.config.debug) {
          var sortTime = new Date();
        }

        var dynamicExp = "var sortWrapper = function(a,b) {",
          l = sortList.length;

        // TODO: inline functions.  
        for (var i = 0; i < l; i++) {

          var c = sortList[i][0];
          var order = sortList[i][1];
          //var s = (getCachedSortType(table.config.parsers,c) == "text") ? ((order == 0) ? "sortText" : "sortTextDesc") : ((order == 0) ? "sortNumeric" : "sortNumericDesc");
          //var s = (table.config.parsers[c].type == "text") ? ((order == 0) ? makeSortText(c) : makeSortTextDesc(c)) : ((order == 0) ? makeSortNumeric(c) : makeSortNumericDesc(c));
          var s = (table.config.parsers[c].type == "text") ? ((order == 0) ? makeSortFunction("text", "asc", c) : makeSortFunction("text", "desc", c)) : ((order == 0) ? makeSortFunction("numeric", "asc", c) : makeSortFunction("numeric", "desc", c));
          var e = "e" + i;

          dynamicExp += "var " + e + " = " + s; // + "(a[" + c + "],b[" + c + "]); ";
          dynamicExp += "if(" + e + ") { return " + e + "; } ";
          dynamicExp += "else { ";

        }

        // if value is the same keep orignal order  
        var orgOrderCol = cache.normalized[0].length - 1;
        dynamicExp += "return a[" + orgOrderCol + "]-b[" + orgOrderCol + "];";

        for (var i = 0; i < l; i++) {
          dynamicExp += "}; ";
        }

        dynamicExp += "return 0; ";
        dynamicExp += "}; ";

        if (table.config.debug) {
          benchmark("Evaling expression:" + dynamicExp, new Date());
        }

        eval(dynamicExp);

        cache.normalized.sort(sortWrapper);

        if (table.config.debug) {
          benchmark("Sorting on " + sortList.toString() + " and dir " + order + " time:", sortTime);
        }

        return cache;
      };

      function makeSortFunction(type, direction, index) {
        var a = "a[" + index + "]",
          b = "b[" + index + "]";
        if (type == 'text' && direction == 'asc') {
          return "(" + a + " == " + b + " ? 0 : (" + a + " === null ? Number.POSITIVE_INFINITY : (" + b + " === null ? Number.NEGATIVE_INFINITY : (" + a + " < " + b + ") ? -1 : 1 )));";
        } else if (type == 'text' && direction == 'desc') {
          return "(" + a + " == " + b + " ? 0 : (" + a + " === null ? Number.POSITIVE_INFINITY : (" + b + " === null ? Number.NEGATIVE_INFINITY : (" + b + " < " + a + ") ? -1 : 1 )));";
        } else if (type == 'numeric' && direction == 'asc') {
          return "(" + a + " === null && " + b + " === null) ? 0 :(" + a + " === null ? Number.POSITIVE_INFINITY : (" + b + " === null ? Number.NEGATIVE_INFINITY : " + a + " - " + b + "));";
        } else if (type == 'numeric' && direction == 'desc') {
          return "(" + a + " === null && " + b + " === null) ? 0 :(" + a + " === null ? Number.POSITIVE_INFINITY : (" + b + " === null ? Number.NEGATIVE_INFINITY : " + b + " - " + a + "));";
        }
      };

      function makeSortText(i) {
        return "((a[" + i + "] < b[" + i + "]) ? -1 : ((a[" + i + "] > b[" + i + "]) ? 1 : 0));";
      };

      function makeSortTextDesc(i) {
        return "((b[" + i + "] < a[" + i + "]) ? -1 : ((b[" + i + "] > a[" + i + "]) ? 1 : 0));";
      };

      function makeSortNumeric(i) {
        return "a[" + i + "]-b[" + i + "];";
      };

      function makeSortNumericDesc(i) {
        return "b[" + i + "]-a[" + i + "];";
      };


      function sortText(a, b) {
        return ((a < b) ? -1 : ((a > b) ? 1 : 0));
      };

      function sortTextDesc(a, b) {
        return ((b < a) ? -1 : ((b > a) ? 1 : 0));
      };

      function sortNumeric(a, b) {
        return a - b;
      };

      function sortNumericDesc(a, b) {
        return b - a;
      };

      function getCachedSortType(parsers, i) {
        return parsers[i].type;
      };

      /* public methods */
      this.construct = function(settings) {

        return this.each(function() {

          if (!this.tHead || !this.tBodies) return;

          var $this, $document, $headers, cache, config, shiftDown = 0,
            sortOrder;

          this.config = {};

          config = $.extend(this.config, $.tablesorter.defaults, settings);

          // store common expression for speed  
          $this = $(this);

          // save the settings where they read
          $.data(this, "tablesorter", config);

          // build headers
          $headers = buildHeaders(this);

          // try to auto detect column type, and store in tables config
          this.config.parsers = buildParserCache(this, $headers);

          // build the cache for the tbody cells
          cache = buildCache(this);

          // get class names and setup UI if needed
          var sortCSS = setupCss(this, $headers);

          // fixate columns if the users supplies the fixedWidth option
          fixColumnWidth(this);

          // apply event handling to headers
          // this is to big, perhaps break it out?
          $headers.click(function(e) {

            var totalRows = ($this[0].tBodies[0] && $this[0].tBodies[0].rows.length) || 0;

            if (!this.sortDisabled && totalRows > 0) {

              // Only call sortStart if sorting is enabled.
              $this.trigger("sortStart");

              // store exp, for speed
              var $cell = $(this);

              // get current column index
              var i = this.column;

              // get current column sort order
              this.order = this.count++ % 2;

              // user only whants to sort on one column
              if (!e[config.sortMultiSortKey]) {

                // flush the sort list
                config.sortList = [];

                if (config.sortForce != null) {
                  var a = config.sortForce;
                  for (var j = 0; j < a.length; j++) {
                    if (a[j][0] != i) {
                      config.sortList.push(a[j]);
                    }
                  }
                }

                // add column to sort list
                config.sortList.push([i, this.order]);

                // multi column sorting
              } else {
                // the user has clicked on an all ready sortet column.
                if (isValueInArray(i, config.sortList)) {

                  // revers the sorting direction for all tables.
                  for (var j = 0; j < config.sortList.length; j++) {
                    var s = config.sortList[j],
                      o = config.headerList[s[0]];
                    if (s[0] == i) {
                      o.count = s[1];
                      o.count++;
                      s[1] = o.count % 2;
                    }
                  }
                } else {
                  // add column to sort list array
                  config.sortList.push([i, this.order]);
                }
              };
              setTimeout(function() {
                //set css for headers
                setHeadersCss($this[0], $headers, config.sortList, sortCSS);
                appendToTable($this[0], multisort($this[0], config.sortList, cache));
              }, 1);
              // stop normal event by returning false
              return false;
            }
            // cancel selection 
          }).mousedown(function() {
            if (config.cancelSelection) {
              this.onselectstart = function() {
                return false;
              };
              return false;
            }
          });

          // apply easy methods that trigger binded events
          $this.bind("update", function() {
            var me = this;
            setTimeout(function() {
              // rebuild parsers.
              me.config.parsers = buildParserCache(me, $headers);
              // rebuild the cache map
              cache = buildCache(me);
            }, 1);
          }).bind("updateCell", function(e, cell) {
            var config = this.config;
            // get position from the dom.
            var pos = [(cell.parentNode.rowIndex - 1), cell.cellIndex];
            // update cache
            cache.normalized[pos[0]][pos[1]] = config.parsers[pos[1]].format(getElementText(config, cell), cell);

          }).bind("sorton", function(e, list) {

            $(this).trigger("sortStart");

            config.sortList = list;

            // update and store the sortlist
            var sortList = config.sortList;

            // update header count index
            updateHeaderSortCount(this, sortList);

            //set css for headers
            setHeadersCss(this, $headers, sortList, sortCSS);

            // sort the table and append it to the dom
            appendToTable(this, multisort(this, sortList, cache));

          }).bind("appendCache", function() {

            appendToTable(this, cache);

          }).bind("applyWidgetId", function(e, id) {

            getWidgetById(id).format(this);

          }).bind("applyWidgets", function() {
            // apply widgets
            applyWidget(this);
          });

          if ($.metadata && ($(this).metadata() && $(this).metadata().sortlist)) {
            config.sortList = $(this).metadata().sortlist;
          }
          // if user has supplied a sort list to constructor.
          if (config.sortList.length > 0) {
            $this.trigger("sorton", [config.sortList]);
          } else {
            // apply widgets only if there is no sort list in constructor
            applyWidget(this);
          }
        });
      };

      this.addParser = function(parser) {
        var l = parsers.length,
          a = true;
        for (var i = 0; i < l; i++) {
          if (parsers[i].id.toLowerCase() == parser.id.toLowerCase()) {
            a = false;
          }
        }
        if (a) {
          parsers.push(parser);
        };
      };

      this.addWidget = function(widget) {
        widgets.push(widget);
      };

      this.formatDate = function(s, config) {
        if (config.locale != "us") {
          var datePoint = '\\' + config.format[config.locale]["date"];
          s = s.replace(new RegExp('[\\-' + datePoint + ']', 'g'), config.format["us"]["date"]);
        }
        return s;
      };
      this.formatDecimal = function(s, config) {
        if (config.locale != "us") {
          s = s.replace(config.format[config.locale]["decimal"], config.format["us"]["decimal"]);
        }
        return s;
      };
      this.formatFloat = function(s) {
        var i = parseFloat(s);
        return (isNaN(i)) ? 0 : i;
      };
      this.formatInt = function(s) {
        var i = parseInt(s);
        return (isNaN(i)) ? 0 : i;
      };

      this.isDigit = function(s, config) {
        var decimalPoint = '\\' + config.format[config.locale]["decimal"];
        var exp = '/(^[+]?0(' + decimalPoint + '0+)?$)|(^([-+]?[0-9]*)$)|(^([-+]?((0?[0-9]*)' + decimalPoint + '(0*[0-9]*)))$)|(^[-+]?[0-9]*' + decimalPoint + '0+$)/';
        return RegExp(exp).test($.trim(s));
      };

      this.clearTableBody = function(table) {
        if ($.browser.msie) {
          function empty() {
            while (this.firstChild) this.removeChild(this.firstChild);
          }
          empty.apply(table.tBodies[0]);
        } else {
          table.tBodies[0].innerHTML = "";
        }
      };
    }
  });

  // extend plugin scope
  $.fn.extend({
    tablesorter: $.tablesorter.construct
  });

  // make shortcut
  var ts = $.tablesorter;

  // add default parsers
  ts.addParser({
    id: "text",
    is: function(s) {
      return true;
    },
    format: function(s) {
      return $.trim(s.toLowerCase());
    },
    type: "text"
  });

  ts.addParser({
    id: "digit",
    is: function(s, table) {
      var c = table.config;
      return $.tablesorter.isDigit(s, c);
    },
    format: function(s, table) {
      var c = table.config;
      s = $.tablesorter.formatDecimal(s, c);
      return $.tablesorter.formatFloat(s);
    },
    type: "numeric"
  });

  ts.addParser({
    id: "currency",
    is: function(s) {
      return /^[Â£$â‚¬?.,]/.test(s);
    },
    format: function(s, table) {
      var c = table.config;
      s = $.tablesorter.formatDecimal(s, c);
      return $.tablesorter.formatFloat(s.replace(new RegExp(/[Â£$â‚¬]/g), ""));
    },
    type: "numeric"
  });

  ts.addParser({
    id: "ipAddress",
    is: function(s) {
      return /^\d{2,3}[\.]\d{1,3}[\.]\d{1,3}[\.]\d{1,3}$/.test(s);
    },
    format: function(s) {
      var a = s.split("."),
        r = "",
        l = a.length;
      for (var i = 0; i < l; i++) {
        var item = a[i];
        if (item.length == 2) {
          r += "0" + item;
        } else {
          r += item;
        }
      }
      return $.tablesorter.formatFloat(r);
    },
    type: "numeric"
  });

  ts.addParser({
    id: "url",
    is: function(s) {
      return /^(https?|ftp|file):\/\/$/.test(s);
    },
    format: function(s) {
      return jQuery.trim(s.replace(new RegExp(/(https?|ftp|file):\/\//), ''));
    },
    type: "text"
  });

  ts.addParser({
    id: "isoDate",
    is: function(s) {
      return /^\d{4}[\/-]\d{1,2}[\/-]\d{1,2}$/.test(s);
    },
    format: function(s) {
      return $.tablesorter.formatFloat((s != "") ? new Date(s.replace(new RegExp(/-/g), "/")).getTime() : "0");
    },
    type: "numeric"
  });

  ts.addParser({
    id: "percent",
    is: function(s) {
      return /\%$/.test($.trim(s));
    },
    format: function(s, table) {
      var c = table.config;
      s = $.tablesorter.formatDecimal(s, c);
      return $.tablesorter.formatFloat(s.replace(new RegExp(/%/g), ""));
    },
    type: "numeric"
  });

  ts.addParser({
    id: "usLongDate",
    is: function(s) {
      return s.match(new RegExp(/^[A-Za-z]{3,10}\.? [0-9]{1,2}, ([0-9]{4}|'?[0-9]{2}) (([0-2]?[0-9]:[0-5][0-9])|([0-1]?[0-9]:[0-5][0-9]\s(AM|PM)))$/));
    },
    format: function(s) {
      return $.tablesorter.formatFloat(new Date(s).getTime());
    },
    type: "numeric"
  });

  ts.addParser({
    id: "mediumDate",
    is: function(s, table) {
      var c = table.config;
      var datePoint = '\\' + c.format[c.locale]["date"];
      var expStr = '\\d{1,2}[\\-' + datePoint + ']\\d{1,2}[\\-' + datePoint + ']\\d{4}';
      return RegExp(expStr).test(s);
    },
    format: function(s, table) {
      var c = table.config;
      s = $.tablesorter.formatDate(s, c);
      if (c.locale == "us") {
        // reformat the string in ISO format
        s = s.replace(/(\d{1,2})[\/](\d{1,2})[\/](\d{4})/, "$3/$1/$2");
      } else if (c.locale == "en" || c.locale == "de" || c.locale == "eu") {
        //reformat the string in ISO format
        s = s.replace(/(\d{1,2})[\/](\d{1,2})[\/](\d{4})/, "$3/$2/$1");
      }
      return $.tablesorter.formatFloat((s != "") ? new Date(s).getTime() : 0);
    },
    type: "numeric"
  });

  ts.addParser({
    id: "shortDate",
    is: function(s, table) {
      var c = table.config;
      var datePoint = '\\' + c.format[c.locale]["date"];
      var expStr = '\\d{1,2}[\\-' + datePoint + ']\\d{1,2}[\\-' + datePoint + ']\\d{2}';
      return RegExp(expStr).test(s);
    },
    format: function(s, table) {
      var c = table.config;
      s = $.tablesorter.formatDate(s, c);
      if (c.locale == "us") {
        // reformat the string in non-ISO format
        s = s.replace(/(\d{1,2})[\/](\d{1,2})[\/](\d{2})/, "$1/$2/$3");
      } else if (c.locale == "en" || c.locale == "de" || c.locale == "eu") {
        //reformat the string in non-ISO format
        s = s.replace(/(\d{1,2})[\/](\d{1,2})[\/](\d{2})/, "$2/$1/$3");
      }
      return $.tablesorter.formatFloat((s != "") ? new Date(s).getTime() : 0);
    },
    type: "numeric"
  });


  ts.addParser({
    id: "time",
    is: function(s) {
      return /^(([0-2]?[0-9]:[0-5][0-9])|([0-1]?[0-9]:[0-5][0-9]\s(am|pm)))$/.test(s);
    },
    format: function(s) {
      return $.tablesorter.formatFloat(new Date("2000/01/01 " + s).getTime());
    },
    type: "numeric"
  });

  ts.addParser({
    id: "metadata",
    is: function(s) {
      return false;
    },
    format: function(s, table, cell) {
      var c = table.config,
        p = (!c.parserMetadataName) ? 'sortValue' : c.parserMetadataName;
      return $(cell).metadata()[p];
    },
    type: "numeric"
  });

  // add default widgets
  ts.addWidget({
    id: "zebra",
    format: function(table) {
      if (table.config.debug) {
        var time = new Date();
      }
      var $tr, row = -1,
        odd;
      // loop through the visible rows
      $("tr:visible", table.tBodies[0]).each(function(i) {
        $tr = $(this);
        // style children rows the same way the parent row was styled
        if (!$tr.hasClass(table.config.cssChildRow)) row++;
        odd = (row % 2 == 0);
        $tr.removeClass(table.config.widgetZebra.css[odd ? 0 : 1]).addClass(table.config.widgetZebra.css[odd ? 1 : 0]);
      });
      if (table.config.debug) {
        $.tablesorter.benchmark("Applying Zebra widget", time);
      }
    }
  });
})(jQuery);
