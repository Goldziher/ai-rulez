// Progressive enhancement only: every page is complete without this script.
// Filters the overview table and adds copy buttons to digests. No network.
(function () {
  "use strict";
  var table = document.querySelector("#items");
  var q = document.querySelector("#q");
  var kind = document.querySelector("#kind");
  var status = document.querySelector("#status");
  var count = document.querySelector("#count");

  if (table && q && kind && status) {
    var rows = Array.prototype.slice.call(table.tBodies[0].rows);
    var apply = function () {
      var words = q.value.toLowerCase().split(/\s+/u).filter(Boolean);
      var shown = 0;
      rows.forEach(function (row) {
        var text = row.dataset.text || "";
        var ok =
          (!kind.value || row.dataset.kind === kind.value) &&
          (!status.value || row.dataset.status === status.value) &&
          words.every(function (w) {
            return text.indexOf(w) !== -1;
          });
        row.hidden = !ok;
        if (ok) {
          shown++;
        }
      });
      if (count) {
        count.textContent = shown + " of " + rows.length + " shown";
      }
    };
    [q, kind, status].forEach(function (el) {
      el.addEventListener("input", apply);
      el.addEventListener("change", apply);
    });
    q.form.addEventListener("submit", function (e) {
      e.preventDefault();
    });
    document.addEventListener("keydown", function (e) {
      var t = e.target && e.target.tagName;
      if (e.key === "/" && t !== "INPUT" && t !== "SELECT" && t !== "TEXTAREA") {
        e.preventDefault();
        q.focus();
      }
    });
    apply();
  }

  if (navigator.clipboard) {
    Array.prototype.forEach.call(document.querySelectorAll("code.digest"), function (code) {
      var button = document.createElement("button");
      button.type = "button";
      button.textContent = "Copy";
      button.setAttribute("aria-label", "Copy digest");
      button.addEventListener("click", function () {
        navigator.clipboard.writeText(code.textContent).then(function () {
          button.textContent = "Copied";
        });
      });
      code.parentNode.append(document.createTextNode(" "), button);
    });
  }
})();
