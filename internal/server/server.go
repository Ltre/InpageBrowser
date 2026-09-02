package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ltre/InpageBrowser/internal/auth"
	browserrt "github.com/Ltre/InpageBrowser/internal/runtime"
	"github.com/Ltre/InpageBrowser/internal/webauthn"
)

type challenge struct {
	Phone, Challenge, Origin, RPID, UserHandle string
	Expires                                    time.Time
}

type Server struct {
	auth     *auth.Store
	rt       *browserrt.Manager
	loginT   *template.Template
	browserT *template.Template
	mu       sync.Mutex
	pending  map[string]challenge
}

func New(dataDir string) (*Server, error) {
	st, err := auth.Open(filepath.Join(dataDir, "auth.json"))
	if err != nil {
		return nil, err
	}
	loginT, err := template.ParseFiles("web/login.html")
	if err != nil {
		return nil, err
	}
	browserT, err := template.ParseFiles("web/browser.html")
	if err != nil {
		return nil, err
	}
	return &Server{auth: st, rt: browserrt.NewManager(dataDir), loginT: loginT, browserT: browserT, pending: map[string]challenge{}}, nil
}

func (s *Server) Start(ctx context.Context) { s.rt.Start(ctx) }
func (s *Server) Close()                    { s.rt.Close() }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /static/app.js", staticJS)
	mux.HandleFunc("POST /api/register/start", s.registerStart)
	mux.HandleFunc("POST /api/register/finish", s.registerFinish)
	mux.HandleFunc("POST /api/login/start", s.loginStart)
	mux.HandleFunc("POST /api/login/finish", s.loginFinish)
	mux.HandleFunc("POST /api/logout", s.withUser(s.logout))
	mux.HandleFunc("POST /api/runtime/start", s.withUser(s.runtimeStart))
	mux.HandleFunc("POST /api/runtime/heartbeat", s.withUser(s.runtimeHeartbeat))
	mux.HandleFunc("POST /api/runtime/navigate", s.withUser(s.runtimeNavigate))
	mux.HandleFunc("GET /api/runtime/current", s.withUser(s.runtimeCurrent))
	mux.HandleFunc("/runtime/", s.withUser(s.runtimeProxy))
	mux.HandleFunc("/websockify", s.withUser(s.runtimeProxy))
	mux.HandleFunc("/vnc/", s.withUser(s.runtimeProxy))
	mux.HandleFunc("/static/", s.withUser(s.runtimeProxy))
	mux.HandleFunc("/", s.withUser(s.runtimeProxy))
	return securityHeaders(mux)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if u, ok := s.userFromRequest(r); ok {
		_ = s.browserT.ExecuteTemplate(w, "browser.html", map[string]any{"Phone": u.Phone})
		return
	}
	_ = s.loginT.ExecuteTemplate(w, "login.html", nil)
}

func (s *Server) registerStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, "请求无效", 400)
		return
	}
	phone, err := auth.NormalizePhone(in.Phone)
	if err != nil {
		fail(w, err.Error(), 400)
		return
	}
	if _, ok := s.auth.UserByPhone(phone); ok {
		fail(w, "该手机号已经注册，请直接使用 Passkey 登录", 409)
		return
	}
	origin, rpID := requestOriginRP(r)
	ch, handle, tx := randomToken(32), randomToken(24), randomToken(18)
	s.putChallenge(tx, challenge{Phone: phone, Challenge: ch, Origin: origin, RPID: rpID, UserHandle: handle, Expires: time.Now().Add(5 * time.Minute)})
	writeJSON(w, map[string]any{
		"tx": tx,
		"publicKey": map[string]any{
			"challenge":        ch,
			"rp":               map[string]string{"name": "InpageBrowser", "id": rpID},
			"user":             map[string]string{"id": handle, "name": phone, "displayName": phone},
			"pubKeyCredParams": []map[string]any{{"type": "public-key", "alg": -7}, {"type": "public-key", "alg": -257}},
			"timeout":          60000, "attestation": "none",
			"authenticatorSelection": map[string]string{"residentKey": "preferred", "userVerification": "preferred"},
		},
	})
}

func (s *Server) registerFinish(w http.ResponseWriter, r *http.Request) {
	var in credentialPayload
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, "请求无效", 400)
		return
	}
	ch, ok := s.takeChallenge(in.Tx)
	if !ok {
		fail(w, "注册挑战已失效，请重新开始", 400)
		return
	}
	client, e1 := webauthn.DecodeBase64URL(in.Response.ClientDataJSON)
	att, e2 := webauthn.DecodeBase64URL(in.Response.AttestationObject)
	rawID, e3 := webauthn.DecodeBase64URL(in.RawID)
	if firstErr(e1, e2, e3) != nil {
		fail(w, "Passkey 数据编码无效", 400)
		return
	}
	res, err := webauthn.VerifyRegistration(client, att, ch.Challenge, ch.Origin, ch.RPID)
	if err != nil {
		fail(w, err.Error(), 400)
		return
	}
	if !equalBytes(rawID, res.CredentialID) {
		fail(w, "Passkey credential id 不一致", 400)
		return
	}
	u, err := s.auth.CreateUser(ch.Phone, ch.UserHandle, webauthn.EncodeBase64URL(res.CredentialID), base64.RawStdEncoding.EncodeToString(res.PublicKeyDER), res.SignCount)
	if err != nil {
		fail(w, err.Error(), 409)
		return
	}
	sess, err := s.auth.NewSession(u.ID)
	if err != nil {
		fail(w, err.Error(), 500)
		return
	}
	setSessionCookie(w, r, sess)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) loginStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, "请求无效", 400)
		return
	}
	phone, err := auth.NormalizePhone(in.Phone)
	if err != nil {
		fail(w, err.Error(), 400)
		return
	}
	u, ok := s.auth.UserByPhone(phone)
	if !ok {
		fail(w, "该手机号尚未注册", 404)
		return
	}
	origin, rpID := requestOriginRP(r)
	ch, tx := randomToken(32), randomToken(18)
	s.putChallenge(tx, challenge{Phone: phone, Challenge: ch, Origin: origin, RPID: rpID, Expires: time.Now().Add(5 * time.Minute)})
	allow := make([]map[string]any, 0, len(u.Credentials))
	for _, c := range u.Credentials {
		allow = append(allow, map[string]any{"type": "public-key", "id": c.ID})
	}
	writeJSON(w, map[string]any{"tx": tx, "publicKey": map[string]any{"challenge": ch, "rpId": rpID, "allowCredentials": allow, "timeout": 60000, "userVerification": "preferred"}})
}

func (s *Server) loginFinish(w http.ResponseWriter, r *http.Request) {
	var in credentialPayload
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, "请求无效", 400)
		return
	}
	ch, ok := s.takeChallenge(in.Tx)
	if !ok {
		fail(w, "登录挑战已失效，请重新开始", 400)
		return
	}
	u, ok := s.auth.UserByPhone(ch.Phone)
	if !ok {
		fail(w, "用户不存在", 404)
		return
	}
	var stored *auth.Credential
	for i := range u.Credentials {
		if u.Credentials[i].ID == in.RawID || u.Credentials[i].ID == in.ID {
			stored = &u.Credentials[i]
			break
		}
	}
	if stored == nil {
		fail(w, "Passkey 不属于该手机号", 403)
		return
	}
	client, e1 := webauthn.DecodeBase64URL(in.Response.ClientDataJSON)
	ad, e2 := webauthn.DecodeBase64URL(in.Response.AuthenticatorData)
	sig, e3 := webauthn.DecodeBase64URL(in.Response.Signature)
	der, e4 := base64.RawStdEncoding.DecodeString(stored.PublicKeyDER)
	if firstErr(e1, e2, e3, e4) != nil {
		fail(w, "Passkey 数据编码无效", 400)
		return
	}
	count, err := webauthn.VerifyAssertion(client, ad, sig, ch.Challenge, ch.Origin, ch.RPID, der)
	if err != nil {
		fail(w, err.Error(), 403)
		return
	}
	_ = s.auth.UpdateSignCount(u.ID, stored.ID, count)
	sess, err := s.auth.NewSession(u.ID)
	if err != nil {
		fail(w, err.Error(), 500)
		return
	}
	setSessionCookie(w, r, sess)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, u auth.User) {
	s.rt.StopUser(u.ID)
	if c, _ := r.Cookie("inpage_session"); c != nil {
		_ = s.auth.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "inpage_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) runtimeStart(w http.ResponseWriter, r *http.Request, u auth.User) {
	rt, err := s.rt.Ensure(r.Context(), u.ID)
	if err != nil {
		fail(w, err.Error(), 503)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "started_at": rt.StartedAt})
}

func (s *Server) runtimeHeartbeat(w http.ResponseWriter, _ *http.Request, u auth.User) {
	s.rt.Heartbeat(u.ID)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) runtimeNavigate(w http.ResponseWriter, r *http.Request, u auth.User) {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, "请求无效", 400)
		return
	}
	target, err := normalizeURL(in.URL)
	if err != nil {
		fail(w, err.Error(), 400)
		return
	}
	rt, ok := s.rt.Get(u.ID)
	if !ok {
		fail(w, "浏览器实例尚未启动", 409)
		return
	}
	if err := s.rt.Navigate(rt, target); err != nil {
		fail(w, err.Error(), 502)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "url": target})
}

func (s *Server) runtimeCurrent(w http.ResponseWriter, _ *http.Request, u auth.User) {
	rt, ok := s.rt.Get(u.ID)
	if !ok {
		writeJSON(w, map[string]string{"url": ""})
		return
	}
	v, err := s.rt.CurrentURL(rt)
	if err != nil {
		fail(w, err.Error(), 502)
		return
	}
	writeJSON(w, map[string]string{"url": v})
}

func (s *Server) runtimeProxy(w http.ResponseWriter, r *http.Request, u auth.User) {
	rt, ok := s.rt.Get(u.ID)
	if !ok {
		fail(w, "浏览器实例已经回收，请刷新页面重新启动", 410)
		return
	}
	s.rt.Heartbeat(u.ID)
	if strings.HasPrefix(r.URL.Path, "/runtime/") {
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/runtime")
		if r2.URL.Path == "" {
			r2.URL.Path = "/"
		}
		s.rt.Proxy(rt).ServeHTTP(w, r2)
		return
	}
	s.rt.Proxy(rt).ServeHTTP(w, r)
}

func (s *Server) withUser(next func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.userFromRequest(r)
		if !ok {
			fail(w, "请先登录", 401)
			return
		}
		next(w, r, u)
	}
}

func (s *Server) userFromRequest(r *http.Request) (auth.User, bool) {
	c, err := r.Cookie("inpage_session")
	if err != nil {
		return auth.User{}, false
	}
	return s.auth.UserFromSession(c.Value)
}

func (s *Server) putChallenge(tx string, c challenge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.pending {
		if v.Expires.Before(now) {
			delete(s.pending, k)
		}
	}
	s.pending[tx] = c
}

func (s *Server) takeChallenge(tx string) (challenge, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.pending[tx]
	delete(s.pending, tx)
	return c, ok && c.Expires.After(time.Now())
}

type credentialPayload struct {
	Tx       string `json:"tx"`
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
	} `json:"response"`
}

func requestOriginRP(r *http.Request) (string, string) {
	host := r.Host
	if xf := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); xf != "" {
		host = xf
	}
	rp := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		rp = h
	}
	proto := "http"
	if isHTTPS(r) {
		proto = "https"
	}
	return proto + "://" + host, strings.ToLower(rp)
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if p := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); strings.EqualFold(p, "https") {
		return true
	}
	var cf struct {
		Scheme string `json:"scheme"`
	}
	return json.Unmarshal([]byte(r.Header.Get("CF-Visitor")), &cf) == nil && strings.EqualFold(cf.Scheme, "https")
}

func normalizeURL(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("请输入网址")
	}
	if !strings.Contains(v, "://") {
		isIP := net.ParseIP(strings.Trim(v, "[]")) != nil
		if strings.Contains(v, " ") || (!strings.Contains(v, ".") && !strings.Contains(v, ":") && !isIP && !strings.EqualFold(v, "localhost")) {
			return "https://www.google.com/search?q=" + url.QueryEscape(v), nil
		}
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("网址无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("只支持 http/https")
	}
	return u.String(), nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: "inpage_session", Value: token, Path: "/", MaxAge: int(auth.SessionTTL.Seconds()), HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var x byte
	for i := range a {
		x |= a[i] ^ b[i]
	}
	return x == 0
}
func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
func fail(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
