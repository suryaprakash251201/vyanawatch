package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vyanawatch/vyanawatch/config"
	"golang.org/x/crypto/bcrypt"
)

// sessionToken represents a signed session.
type sessionToken struct {
	Username  string `json:"u"`
	ExpiresAt int64  `json:"e"`
}

var (
	sessionSecret     []byte
	sessionSecretOnce sync.Once
)

// getSessionSecret returns a per-process random secret for signing tokens.
func getSessionSecret() []byte {
	sessionSecretOnce.Do(func() {
		sessionSecret = make([]byte, 32)
		if _, err := rand.Read(sessionSecret); err != nil {
			log.Fatal().Err(err).Msg("Failed to generate session secret")
		}
	})
	return sessionSecret
}

// createSession generates a signed session cookie value.
func createSession(username string, ttl time.Duration) string {
	tok := sessionToken{
		Username:  username,
		ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	payload, _ := json.Marshal(tok)
	mac := hmac.New(sha256.New, getSessionSecret())
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	return hex.EncodeToString(payload) + "." + sig
}

// validateSession checks a session cookie value and returns the username if valid.
func validateSession(value string) (string, bool) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	payload, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	sig, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", false
	}

	mac := hmac.New(sha256.New, getSessionSecret())
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}

	var tok sessionToken
	if err := json.Unmarshal(payload, &tok); err != nil {
		return "", false
	}
	if time.Now().Unix() > tok.ExpiresAt {
		return "", false
	}
	return tok.Username, true
}

// authMiddleware returns middleware that enforces authentication when auth is enabled.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := config.Get()
		if cfg == nil || !cfg.Auth.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Check session cookie
		cookie, err := r.Cookie("vyanawatch_session")
		if err == nil {
			if _, valid := validateSession(cookie.Value); valid {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check Authorization header (for API clients)
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if _, valid := validateSession(token); valid {
				next.ServeHTTP(w, r)
				return
			}
		}

		// API calls get JSON error, browser requests get redirect
		if strings.HasPrefix(r.URL.Path, "/api/") {
			respondError(w, http.StatusUnauthorized, "Authentication required")
		} else {
			http.Redirect(w, r, "/login", http.StatusFound)
		}
	})
}

// handleLogin serves the login page (GET) or processes login (POST).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(loginPageHTML())
		return
	}
	// POST — process login
	cfg := config.Get()
	if cfg == nil || !cfg.Auth.Enabled {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	} else {
		creds.Username = r.FormValue("username")
		creds.Password = r.FormValue("password")
	}

	if !checkCredentials(cfg, creds.Username, creds.Password) {
		// Rate-limit brute force attempts
		time.Sleep(500 * time.Millisecond)
		if strings.Contains(contentType, "application/json") {
			respondError(w, http.StatusUnauthorized, "Invalid username or password")
		} else {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(loginPageHTMLWithError("Invalid username or password"))
		}
		return
	}

	token := createSession(creds.Username, 24*time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     "vyanawatch_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	if strings.Contains(contentType, "application/json") {
		respondJSON(w, http.StatusOK, map[string]string{"token": token, "message": "Login successful"})
	} else {
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// handleLogout clears the session cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "vyanawatch_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// checkCredentials validates username/password against config.
func checkCredentials(cfg *config.Config, username, password string) bool {
	if cfg.Auth.Username == "" || cfg.Auth.Password == "" {
		return false
	}
	if username != cfg.Auth.Username {
		return false
	}
	// Support bcrypt-hashed passwords (starting with $2)
	if strings.HasPrefix(cfg.Auth.Password, "$2") {
		err := bcrypt.CompareHashAndPassword([]byte(cfg.Auth.Password), []byte(password))
		return err == nil
	}
	// Plain text comparison (for simple setups)
	return cfg.Auth.Password == password
}

// loginPageHTML returns the login page HTML.
func loginPageHTML() []byte {
	return loginPageHTMLWithError("")
}

// loginPageHTMLWithError returns the login page with an optional error message.
func loginPageHTMLWithError(errMsg string) []byte {
	errorBlock := ""
	if errMsg != "" {
		errorBlock = `<div class="error">` + errMsg + `</div>`
	}
	html := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<title>VyanaWatch — Login</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f5f5f5;min-height:100vh;display:flex;align-items:center;justify-content:center}
.login-card{background:#fff;border-radius:12px;padding:40px;width:100%;max-width:400px;box-shadow:0 4px 24px rgba(0,0,0,.08);border:1px solid #e0e0e0}
.logo{text-align:center;margin-bottom:28px}
.logo svg{width:48px;height:48px;margin-bottom:10px;display:block;margin-left:auto;margin-right:auto}
.logo h1{font-size:22px;font-weight:700;color:#333}
.logo h1 span{color:#5cdd8b}
.logo p{font-size:13px;color:#888;margin-top:4px}
.form-group{margin-bottom:18px}
.form-group label{display:block;font-size:13px;font-weight:600;color:#333;margin-bottom:6px}
.form-group input{width:100%;padding:10px 14px;border:1px solid #e0e0e0;border-radius:6px;font-size:14px;outline:none;background:#fff;color:#333;transition:border-color .15s}
.form-group input:focus{border-color:#5cdd8b;box-shadow:0 0 0 3px rgba(92,221,139,.15)}
.btn-login{width:100%;padding:12px;border:none;border-radius:6px;font-size:14px;font-weight:600;cursor:pointer;background:#5cdd8b;color:#fff;transition:background .15s}
.btn-login:hover{background:#4bc67d}
.error{background:#fde8e8;color:#c42b2b;padding:10px 14px;border-radius:6px;font-size:13px;margin-bottom:18px;border:1px solid #f5c6cb}
</style>
</head>
<body>
<div class="login-card">
<div class="logo">
<svg viewBox="0 0 32 32" fill="none"><circle cx="16" cy="16" r="14" stroke="#5cdd8b" stroke-width="3"/><polyline points="8,18 14,12 18,20 24,10" stroke="#5cdd8b" stroke-width="2.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
<h1>Vyana<span>Watch</span></h1>
<p>Sign in to your dashboard</p>
</div>
` + errorBlock + `
<form method="POST" action="/login" autocomplete="on">
<div class="form-group">
<label for="username">Username</label>
<input id="username" name="username" type="text" placeholder="admin" required autofocus autocomplete="username">
</div>
<div class="form-group">
<label for="password">Password</label>
<input id="password" name="password" type="password" placeholder="••••••••" required autocomplete="current-password">
</div>
<button type="submit" class="btn-login">Sign In</button>
</form>
</div>
</body>
</html>`
	return []byte(html)
}
