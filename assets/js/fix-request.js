/**
 * /create-fix-request/ — submit the fix form via fetch (multipart) and
 * downsize photos in the browser first so phone-camera shots upload fast on
 * cellular. Without JS the form still posts natively to /fix-request.
 */
(function () {
  "use strict";

  var form = document.getElementById("fix-form");
  if (!form) return;

  var MAX_PHOTOS = 6;
  var MAX_DIM = 2000; // px on the long edge
  var MAX_ORIGINAL_BYTES = 10 * 1024 * 1024; // server limit per photo

  var input = document.getElementById("fix-photos");
  var list = document.getElementById("fix-photo-list");
  var photoError = document.getElementById("fix-photo-error");
  var formError = document.getElementById("fix-error");
  var submitBtn = form.querySelector('button[type="submit"]');
  var photos = []; // { file: File, url: string }
  var pending = 0;

  function setError(el, msg) {
    el.textContent = msg || "";
    if (el === formError) el.hidden = !msg;
  }

  function resize(file) {
    if (!window.createImageBitmap || !HTMLCanvasElement.prototype.toBlob) return Promise.resolve(file);
    return createImageBitmap(file, { imageOrientation: "from-image" })
      .then(function (bmp) {
        var scale = Math.min(1, MAX_DIM / Math.max(bmp.width, bmp.height));
        var canvas = document.createElement("canvas");
        canvas.width = Math.round(bmp.width * scale);
        canvas.height = Math.round(bmp.height * scale);
        canvas.getContext("2d").drawImage(bmp, 0, 0, canvas.width, canvas.height);
        if (bmp.close) bmp.close();
        return new Promise(function (resolve) {
          canvas.toBlob(
            function (blob) {
              if (!blob || (scale === 1 && blob.size > file.size)) return resolve(file);
              var name = (file.name || "photo").replace(/\.[^.]+$/, "") + ".jpg";
              resolve(new File([blob], name, { type: "image/jpeg" }));
            },
            "image/jpeg",
            0.85,
          );
        });
      })
      .catch(function () {
        return file; // e.g. browser can't decode HEIC; send the original
      });
  }

  function render() {
    list.textContent = "";
    photos.forEach(function (p, i) {
      var li = document.createElement("li");
      li.className = "fix-photo__item";
      var img = document.createElement("img");
      img.src = p.url;
      img.alt = "Selected photo " + (i + 1);
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "fix-photo__remove";
      btn.setAttribute("aria-label", "Remove photo " + (i + 1));
      btn.textContent = "×";
      btn.addEventListener("click", function () {
        URL.revokeObjectURL(p.url);
        photos.splice(i, 1);
        setError(photoError, "");
        render();
      });
      li.append(img, btn);
      list.appendChild(li);
    });
  }

  input.addEventListener("change", function () {
    var chosen = Array.prototype.slice.call(input.files);
    input.value = ""; // allow re-picking the same photo; we keep our own list
    if (!chosen.length) return;
    setError(photoError, "");
    pending++;
    submitBtn.disabled = true;
    Promise.all(chosen.map(resize))
      .then(function (files) {
        files.forEach(function (f) {
          if (photos.length >= MAX_PHOTOS) {
            setError(photoError, "You can attach up to " + MAX_PHOTOS + " photos.");
          } else if (f.size > MAX_ORIGINAL_BYTES) {
            setError(photoError, "One of those photos is too large to upload.");
          } else {
            photos.push({ file: f, url: URL.createObjectURL(f) });
          }
        });
        render();
      })
      .then(function () {
        if (--pending === 0) submitBtn.disabled = false;
      });
  });

  function typesValid() {
    var ok = form.querySelector('input[name="fixType"]:checked') !== null;
    setError(form.querySelector(".fix-form__types .field-error"), ok ? "" : "Please choose at least one.");
    return ok;
  }
  form.querySelectorAll('input[name="fixType"]').forEach(function (cb) {
    cb.addEventListener("change", typesValid);
  });

  form.addEventListener("submit", function (e) {
    e.preventDefault();
    setError(formError, "");

    var typesOk = typesValid();
    if (!form.checkValidity() || !typesOk) {
      form.reportValidity();
      var firstBad = form.querySelector(":invalid");
      if (firstBad) firstBad.focus();
      return;
    }

    var data = new FormData(form);
    data.delete("photos");
    photos.forEach(function (p) {
      data.append("photos", p.file, p.file.name);
    });

    var label = submitBtn.textContent;
    submitBtn.disabled = true;
    submitBtn.textContent = "Sending…";

    fetch(form.action, { method: "POST", body: data, headers: { Accept: "application/json" } })
      .then(function (res) {
        return res
          .json()
          .catch(function () { return {}; })
          .then(function (body) { return { ok: res.ok && body.success, body: body }; });
      })
      .then(function (r) {
        if (!r.ok) {
          throw new Error(r.body.error || "We couldn't send your request. Please try again.");
        }
        if (typeof umami === "object" && typeof umami.track === "function") {
          try { umami.track("fix_request_submit"); } catch (_) {}
        }
        photos.forEach(function (p) { URL.revokeObjectURL(p.url); });
        location.hash = "fix-received";
        var done = document.getElementById("fix-received");
        if (done) done.focus({ preventScroll: true });
      })
      .catch(function (err) {
        setError(formError, err.message === "Failed to fetch"
          ? "Couldn't reach the server. Check your connection and try again."
          : err.message);
        submitBtn.disabled = false;
        submitBtn.textContent = label;
      });
  });
})();
