// Минимальный AJAX-хелпер в стиле htmx: атрибуты hx-get/hx-post/hx-put/hx-delete/hx-patch,
// hx-target (CSS-селектор цели подмены), hx-swap (innerHTML|outerHTML), hx-confirm, hx-push-url.
// Совместим по атрибутам с настоящим htmx — при желании static/app.js можно заменить на htmx.min.js
// без изменения разметки (используются только эти базовые атрибуты).
(function () {
  "use strict";

  function targetFor(el) {
    var sel = el.getAttribute("hx-target");
    if (!sel || sel === "this") return el;
    return document.querySelector(sel) || el;
  }

  function swapInto(target, mode, html) {
    if (mode === "outerHTML") {
      target.outerHTML = html;
    } else {
      target.innerHTML = html;
    }
  }

  // startPending — блокирует кнопки формы и подменяет их подпись на "hx-pending-text"
  // (или «Обработка…»), чтобы пользователь видел, что запрос ушёл. Возвращает функцию отката.
  function startPending(el) {
    var buttons = el.tagName === "FORM"
      ? el.querySelectorAll('button[type="submit"], button:not([type])')
      : [el];
    var pendingText = el.getAttribute("hx-pending-text") || "Обработка…";
    var saved = [];
    buttons.forEach(function (btn) {
      saved.push({ btn: btn, text: btn.textContent, disabled: btn.disabled });
      btn.textContent = pendingText;
      btn.disabled = true;
    });
    if (el.tagName === "FORM") el.setAttribute("aria-busy", "true");
    return function () {
      saved.forEach(function (s) { s.btn.textContent = s.text; s.btn.disabled = s.disabled; });
      if (el.tagName === "FORM") el.removeAttribute("aria-busy");
    };
  }

  async function doRequest(method, url, body, target, swapMode, source) {
    var stopPending = source ? startPending(source) : function () {};
    var resp;
    try {
      resp = await fetch(url, {
        method: method,
        headers: { "HX-Request": "true" },
        body: body || undefined,
      });
    } catch (err) {
      stopPending();
      alert("Ошибка сети: " + err);
      return null;
    }
    var text = await resp.text();
    stopPending();
    swapInto(target, swapMode, text);
    bind(document);
    return resp;
  }

  function handleLink(el, e) {
    e.preventDefault();
    var url = el.getAttribute("hx-get");
    var target = targetFor(el);
    var swapMode = el.getAttribute("hx-swap") || "innerHTML";
    doRequest("GET", url, null, target, swapMode, el).then(function (resp) {
      if (resp) history.pushState({}, "", url);
    });
  }

  function handleAction(el, verb, e) {
    e.preventDefault();
    var confirmMsg = el.getAttribute("hx-confirm");
    if (confirmMsg && !window.confirm(confirmMsg)) return;

    var url = el.getAttribute("hx-" + verb);
    var target = targetFor(el);
    var swapMode = el.getAttribute("hx-swap") || "innerHTML";
    var body = null;
    if (el.tagName === "FORM") {
      body = new URLSearchParams(new FormData(el));
    }
    doRequest(verb.toUpperCase(), url, body, target, swapMode, el).then(function (resp) {
      if (!resp) return;
      var pushUrl = el.getAttribute("hx-push-url");
      if (pushUrl) {
        history.pushState({}, "", pushUrl === "true" ? url : pushUrl);
      }
    });
  }

  function bind(root) {
    root.querySelectorAll("[hx-get]").forEach(function (el) {
      if (el.__hxBound) return;
      el.__hxBound = true;
      el.addEventListener("click", function (e) { handleLink(el, e); });
    });
    ["post", "put", "delete", "patch"].forEach(function (verb) {
      root.querySelectorAll("[hx-" + verb + "]").forEach(function (el) {
        if (el.__hxBound) return;
        el.__hxBound = true;
        var evt = el.tagName === "FORM" ? "submit" : "click";
        el.addEventListener(evt, function (e) { handleAction(el, verb, e); });
      });
    });
  }

  window.addEventListener("popstate", function () {
    var main = document.getElementById("main");
    doRequest("GET", location.pathname + location.search, null, main, "innerHTML");
  });

  document.addEventListener("DOMContentLoaded", function () { bind(document); });

  // toggleDayColumn — галочка в шапке таблицы "Недоступные пары" у преподавателя:
  // отмечает/снимает разом все пары одного дня (name="unavailable" value="day:pairNum").
  window.toggleDayColumn = function (headerCheckbox, day) {
    var checked = headerCheckbox.checked;
    document.querySelectorAll('input[name="unavailable"][value^="' + day + ':"]').forEach(function (cb) {
      cb.checked = checked;
    });
  };
})();
