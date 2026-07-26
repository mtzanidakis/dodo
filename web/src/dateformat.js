// Token date formatting for the browser, mirroring internal/dateformat.
//
// This is a classic script, not an ES module: the browser loads it before
// app.js and it publishes itself on the global object. The test suite imports
// the file for its side effect and reads the same global, so there is nothing
// to transform between what ships and what is tested.
(function (root) {
  "use strict";

  // Longest-match-first so MMMM beats MMM and YYYY beats YY.
  var TOKEN_RE = /YYYY|YY|MMMM|MMM|MM|M|DD|D|HH|mm/g;

  // English month names, matching what the server renders for MMM/MMMM.
  var MONTHS = [
    "January", "February", "March", "April", "May", "June",
    "July", "August", "September", "October", "November", "December",
  ];

  function pad(n) {
    return (n < 10 ? "0" : "") + n;
  }

  function escapeRegex(s) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  // format renders a Date through a token pattern.
  function format(d, pattern) {
    if (!pattern) return "";
    return pattern.replace(TOKEN_RE, function (tok) {
      switch (tok) {
        case "YYYY": return String(d.getFullYear());
        case "YY":   return String(d.getFullYear()).slice(-2);
        case "MMMM": return MONTHS[d.getMonth()];
        case "MMM":  return MONTHS[d.getMonth()].slice(0, 3);
        case "MM":   return pad(d.getMonth() + 1);
        case "M":    return String(d.getMonth() + 1);
        case "DD":   return pad(d.getDate());
        case "D":    return String(d.getDate());
        case "HH":   return pad(d.getHours());
        case "mm":   return pad(d.getMinutes());
        default:     return tok;
      }
    });
  }

  // compile turns a pattern into an anchored regexp plus the tokens matching
  // its capture groups, in order. Case-insensitive so "12 jan 2026" is not
  // rejected over a capital letter.
  function compile(pattern) {
    var tokens = [];
    var re = "";
    var last = 0;
    var m;
    TOKEN_RE.lastIndex = 0;
    while ((m = TOKEN_RE.exec(pattern)) !== null) {
      if (m.index > last) re += escapeRegex(pattern.slice(last, m.index));
      tokens.push(m[0]);
      switch (m[0]) {
        case "YYYY": re += "(\\d{4})"; break;
        case "MMMM": re += "(" + MONTHS.join("|") + ")"; break;
        case "MMM":  re += "(" + MONTHS.map(function (x) { return x.slice(0, 3); }).join("|") + ")"; break;
        case "M":
        case "D":    re += "(\\d{1,2})"; break;
        default:     re += "(\\d{2})"; break;
      }
      last = m.index + m[0].length;
    }
    if (last < pattern.length) re += escapeRegex(pattern.slice(last));
    return { re: new RegExp("^" + re + "$", "i"), tokens: tokens };
  }

  function monthIndex(name) {
    for (var i = 0; i < MONTHS.length; i++) {
      if (MONTHS[i].slice(0, name.length).toLowerCase() === name.toLowerCase()) return i;
    }
    return -1;
  }

  // parse is the reverse of format, returning null when the text does not
  // match the pattern exactly (ignoring surrounding space).
  function parse(s, pattern) {
    if (!s || !pattern) return null;
    var c = compile(pattern);
    var match = c.re.exec(String(s).trim());
    if (!match) return null;

    var year = null, month = 0, day = 1, hour = 0, minute = 0;
    for (var i = 0; i < c.tokens.length; i++) {
      var v = match[i + 1];
      switch (c.tokens[i]) {
        case "YYYY": year = parseInt(v, 10); break;
        case "YY":   year = 2000 + parseInt(v, 10); break;
        case "MMMM":
        case "MMM":
          month = monthIndex(v);
          if (month < 0) return null;
          break;
        case "MM":
        case "M":    month = parseInt(v, 10) - 1; break;
        case "DD":
        case "D":    day = parseInt(v, 10); break;
        case "HH":   hour = parseInt(v, 10); break;
        case "mm":   minute = parseInt(v, 10); break;
      }
    }
    if (year === null) return null;

    var d = new Date(year, month, day, hour, minute, 0, 0);
    // Reject dates the calendar rolled over (31 February and friends), so a
    // typo highlights nothing rather than a different day.
    if (d.getFullYear() !== year || d.getMonth() !== month || d.getDate() !== day) return null;
    return d;
  }

  // hasTime reports whether a pattern carries a clock.
  function hasTime(pattern) {
    return /HH|mm/.test(pattern || "");
  }

  root.dodoDateFormat = {
    MONTHS: MONTHS,
    pad: pad,
    format: format,
    parse: parse,
    hasTime: hasTime,
  };
})(typeof globalThis !== "undefined" ? globalThis : window);
