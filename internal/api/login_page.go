package api

import (
	"html/template"
	"net/http"
)

// loginPage is a real document, not a form drawn later by JavaScript.
// Chrome, Firefox, and Safari offer to save a password only when the
// fields are in the page that loaded and the user submits that form.
var loginPageTmpl = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sign in · Siroc</title>
  <style>
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 24px;
      background:
        radial-gradient(600px 280px at 70% 20%, rgba(226, 120, 58, 0.16), transparent 60%),
        #0c0b0a;
      color: #f4eadc;
      font: 16px/1.4 Outfit, "Segoe UI", sans-serif;
    }
    form {
      width: min(420px, 100%);
      box-sizing: border-box;
      padding: 28px;
      background: #171512;
      border: 1px solid rgba(244, 234, 220, 0.1);
      border-radius: 12px;
    }
    h1 { margin: 0; text-align: center; font-size: 30px; font-weight: 600; }
    p { margin: 10px 0 22px; text-align: center; color: #a89884; font-size: 12px; }
    label { display: block; margin: 0 0 8px; color: #a89884; font-size: 14px; }
    input[type="text"], input[type="password"] {
      display: block;
      width: 100%;
      box-sizing: border-box;
      height: 32px;
      margin: 0 0 24px;
      padding: 4px 11px;
      border: 1px solid rgba(244, 234, 220, 0.1);
      border-radius: 6px;
      background: #0e0d0c;
      color: #f4eadc;
      font: inherit;
    }
    input:focus { outline: none; border-color: #e2783a; }
    button[type="submit"] {
      display: block;
      width: 100%;
      height: 32px;
      margin-top: 8px;
      border: 0;
      border-radius: 6px;
      background: #e2783a;
      color: #fff;
      font: inherit;
      cursor: pointer;
    }
    .error {
      margin: 0 0 16px;
      padding: 8px 12px;
      border-radius: 8px;
      background: rgba(180, 40, 40, 0.25);
      color: #ffb4b4;
      font-size: 14px;
    }
    .captcha { display: flex; gap: 12px; align-items: center; margin-bottom: 16px; }
    .captcha[hidden] { display: none; }
    .captcha img { width: 140px; height: 48px; background: #fff; border-radius: 6px; }
    .captcha input { margin: 0; }
  </style>
</head>
<body>
  <form method="post" action="/api/login" autocomplete="on">
    <h1>Siroc</h1>
    <p>Siroc control panel</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <label for="username">Username</label>
    <input id="username" name="username" type="text" autocomplete="username" autocapitalize="none" spellcheck="false" required autofocus>
    <label for="password">Password</label>
    <input id="password" name="password" type="password" autocomplete="current-password" minlength="8" required>
    <div class="captcha" id="captcha-box" hidden>
      <img id="captcha-img" alt="Captcha" width="140" height="48">
      <div>
        <label for="captcha">Captcha</label>
        <input id="captcha" name="captcha" type="text" autocomplete="off">
        <input id="captchaId" name="captchaId" type="hidden" value="">
      </div>
    </div>
    <button type="submit">Sign in</button>
  </form>
  <script>
    fetch("/api/me", {credentials: "same-origin"}).then(function (r) {
      if (r.ok) location.replace("/");
    }).catch(function () {});
    fetch("/api/login/captcha", {credentials: "same-origin"}).then(function (r) { return r.json(); }).then(function (st) {
      if (!st || !st.required || !st.captcha) return;
      document.getElementById("captcha-box").hidden = false;
      document.getElementById("captcha-img").src = st.captcha.image;
      document.getElementById("captchaId").value = st.captcha.id;
      document.getElementById("captcha").required = true;
    }).catch(function () {});
  </script>
</body>
</html>
`))

func loginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = loginPageTmpl.Execute(w, struct{ Error string }{Error: r.URL.Query().Get("error")})
}
