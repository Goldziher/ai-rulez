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
    var bodies = Array.prototype.slice.call(table.tBodies);
    var rows = [];
    bodies.forEach(function (body) {
      rows.push.apply(rows, Array.prototype.slice.call(body.rows));
    });
    var page = 0;
    var pager = null;
    var paginate = function (filtering) {
      bodies.forEach(function (body, i) {
        body.hidden = bodies.length > 1 && !filtering && i !== page;
      });
      if (pager) {
        pager.hidden = filtering;
        pager.status.textContent = "Page " + (page + 1) + " of " + bodies.length;
        pager.prev.disabled = page === 0;
        pager.next.disabled = page === bodies.length - 1;
      }
    };
    if (bodies.length > 1) {
      var nav = document.createElement("p");
      nav.className = "pager";
      var pagerButton = function (label, step) {
        var b = document.createElement("button");
        b.type = "button";
        b.textContent = label;
        b.addEventListener("click", function () {
          page = Math.min(Math.max(page + step, 0), bodies.length - 1);
          paginate(false);
        });
        return b;
      };
      pager = nav;
      pager.prev = pagerButton("Previous page", -1);
      pager.next = pagerButton("Next page", 1);
      pager.status = document.createElement("span");
      pager.status.setAttribute("role", "status");
      nav.append(pager.prev, document.createTextNode(" "), pager.status, document.createTextNode(" "), pager.next);
      table.parentNode.insertBefore(nav, table.nextSibling);
    }
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
      var filtering = Boolean(q.value || kind.value || status.value);
      if (filtering) {
        // A filter looks across every page; clearing it starts again from page 1.
        page = 0;
      }
      paginate(filtering);
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
