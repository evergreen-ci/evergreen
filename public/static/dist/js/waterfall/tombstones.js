"use strict";

function TaskTombstones(num) {
  var out = [];
  for (var i = 0; i < num; ++i) {
    out.push(React.createElement("a", { className: "waterfall-box inactive" }));
  }
  return out;
}

function VariantTombstone() {
  return React.createElement(
    "div",
    { className: "row variant-row" },
    React.createElement(
      "div",
      { className: "col-xs-2 build-variants" },
      React.createElement(
        "span",
        { className: "waterfall-tombstone" },
        "\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0\xA0"
      )
    ),
    React.createElement(
      "div",
      { className: "col-xs-10" },
      React.createElement(
        "div",
        { className: "row build-cells", style: { 'height': '100px' } },
        React.createElement(
          "div",
          { className: "waterfall-build" },
          React.createElement(
            "div",
            { className: "active-build" },
            React.createElement(
              "div",
              { className: "waterfall-tombstone" },
              TaskTombstones(80)
            )
          )
        )
      )
    )
  );
}

function GridTombstone() {
  return React.createElement(
    "div",
    { className: "waterfall-grid" },
    React.createElement(VariantTombstone, null)
  );
}
//# sourceMappingURL=tombstones.js.map
