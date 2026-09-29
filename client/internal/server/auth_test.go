package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// call sends a request with an optional session cookie
func call(s *server, method string, url string, body string, cookie string, prepare func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, url, strings.NewReader(body))
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if prepare != nil {
		prepare(request)
	}
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, request)
	return rec
}

func asJSON(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
}

func sessionCookieOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookie {
			return cookie
		}
	}
	return nil
}

func TestDataNeedsLogin(t *testing.T) {
	s, _ := newTestServer(t, nil)
	for _, url := range []string{"/api/v1/fleet", "/api/v1/config", "/api/v1/nothing"} {
		for _, cookie := range []string{"", "stale"} {
			if rec := call(s, "GET", url, "", cookie, nil); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s with cookie %q: expected 401, got %d %s", url, cookie, rec.Code, rec.Body)
			}
		}
	}
	if rec := call(s, "GET", "/api/v1/fleet", "", testSession, nil); rec.Code != http.StatusOK {
		t.Errorf("expected the fleet with a session, got %d %s", rec.Code, rec.Body)
	}
}

func TestAppNeedsNoLogin(t *testing.T) {
	s, _ := newTestServer(t, fstest.MapFS{"index.html": {Data: []byte("<html>")}})
	for _, url := range []string{"/", "/hosts/web1", "/login"} {
		if rec := call(s, "GET", url, "", "", nil); rec.Code != http.StatusOK {
			t.Errorf("%s: expected the app without a login, got %d", url, rec.Code)
		}
	}
}

func TestSession(t *testing.T) {
	s, fake := newTestServer(t, nil)
	if rec := call(s, "GET", "/api/v1/session", "", testSession, nil); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"user":"tester"}` {
		t.Errorf("expected the logged in user, got %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, "GET", "/api/v1/session", "", "", nil); rec.Code != 401 || !strings.Contains(rec.Body.String(), `"hasUsers":true`) {
		t.Errorf("expected 401 with users, got %d %s", rec.Code, rec.Body)
	}
	fake.noUsers.Store(true)
	if rec := call(s, "GET", "/api/v1/session", "", "", nil); rec.Code != 401 || !strings.Contains(rec.Body.String(), `"hasUsers":false`) {
		t.Errorf("expected 401 without users, got %d %s", rec.Code, rec.Body)
	}
}

func TestLogin(t *testing.T) {
	s, _ := newTestServer(t, nil)
	good := `{"user":"alice","password":"correct horse battery"}`

	rec := call(s, "POST", "/api/v1/login", good, "", func(r *http.Request) {
		asJSON(r)
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	cookie := sessionCookieOf(rec)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"user":"alice"}` || cookie == nil {
		t.Fatalf("expected a login, got %d %s", rec.Code, rec.Body)
	}
	if cookie.Value != "new-token" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("unexpected cookie %+v", cookie)
	}
	if rec := call(s, "GET", "/api/v1/fleet", "", cookie.Value, nil); rec.Code != 200 {
		t.Errorf("expected the new session to work, got %d", rec.Code)
	}
	// over plain HTTP the cookie cannot be Secure, or the browser would drop it
	if cookie := sessionCookieOf(call(s, "POST", "/api/v1/login", good, "", asJSON)); cookie == nil || cookie.Secure {
		t.Errorf("expected a cookie without Secure over HTTP, got %+v", cookie)
	}

	tests := []struct {
		body        string
		prepare     func(*http.Request)
		code        int
		wantMessage string
	}{
		{`{"user":"alice","password":"wrong"}`, asJSON, http.StatusUnauthorized, "wrong user name or password"},
		{`{"user":"locked","password":"x"}`, asJSON, http.StatusTooManyRequests, "too many failed logins"},
		{`not json`, asJSON, http.StatusBadRequest, "send user and password"},
		// a form another site posts is not JSON
		{"user=alice&password=correct+horse+battery", func(r *http.Request) {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}, http.StatusUnsupportedMediaType, "JSON"},
	}
	for _, tt := range tests {
		rec := call(s, "POST", "/api/v1/login", tt.body, "", tt.prepare)
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.wantMessage) || sessionCookieOf(rec) != nil {
			t.Errorf("%s: got %d %s, want %d", tt.body, rec.Code, rec.Body, tt.code)
		}
	}
}

func TestLogout(t *testing.T) {
	s, fake := newTestServer(t, nil)
	rec := call(s, "POST", "/api/v1/logout", "{}", testSession, asJSON)
	cookie := sessionCookieOf(rec)
	if rec.Code != 200 || cookie == nil || cookie.Value != "" || cookie.MaxAge >= 0 {
		t.Errorf("expected the cookie to be cleared, got %d %+v", rec.Code, cookie)
	}
	if fake.loggedOut.Load() != testSession {
		t.Errorf("expected the collector to end %q, got %v", testSession, fake.loggedOut.Load())
	}
	if rec := call(s, "POST", "/api/v1/logout", "", testSession, nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 for a logout that is not JSON, got %d", rec.Code)
	}
}

func TestSessionIsCached(t *testing.T) {
	s, fake := newTestServer(t, nil)
	for i := 0; i < 3; i++ {
		call(s, "GET", "/api/v1/fleet", "", testSession, nil)
	}
	if checks := fake.sessionChecks.Load(); checks != 1 {
		t.Errorf("expected the collector to be asked once, got %d", checks)
	}
}

func TestMetricsAuth(t *testing.T) {
	s, fake := newTestServer(t, nil)
	s.metricsAuth = true
	basic := func(user, password string) func(*http.Request) {
		return func(r *http.Request) { r.SetBasicAuth(user, password) }
	}

	rec := call(s, "GET", "/metrics", "", "", nil)
	if rec.Code != 401 || !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Errorf("expected a basic auth challenge, got %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	if rec := call(s, "GET", "/metrics", "", "", basic("alice", "wrong")); rec.Code != 401 {
		t.Errorf("expected 401 for a wrong password, got %d", rec.Code)
	}
	// a right password is checked once, then remembered
	for i := 0; i < 2; i++ {
		if rec := call(s, "GET", "/metrics", "", "", basic("alice", "correct horse battery")); rec.Code != 200 {
			t.Errorf("expected metrics with alice's password, got %d", rec.Code)
		}
	}
	if checks := fake.passwordChecks.Load(); checks != 2 {
		t.Errorf("expected one check for the wrong and one for the right password, got %d", checks)
	}
	if rec := call(s, "GET", "/metrics", "", testSession, nil); rec.Code != 200 {
		t.Errorf("expected metrics for a logged in browser, got %d", rec.Code)
	}
}
