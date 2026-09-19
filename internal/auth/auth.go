package auth

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/siroc-dev/siroc/internal/store"
)

const CookieName = "cp_session"
const ReturnCookie = "cp_return"

type Service struct {
	Store *store.Store
	mu    sync.Mutex
	fails map[string]failState
}

type failState struct {
	n    int
	until time.Time
}

func New(st *store.Store) *Service {
	return &Service{Store: st, fails: map[string]failState{}}
}

func Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(b), err
}

func Check(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) AllowLogin(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.fails[ip]
	if !ok {
		return true
	}
	if time.Now().After(st.until) {
		delete(s.fails, ip)
		return true
	}
	return st.n < 8
}

func (s *Service) Fail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fails[ip]
	st.n++
	if st.n >= 5 {
		st.until = time.Now().Add(time.Minute * time.Duration(st.n-4))
	}
	s.fails[ip] = st
}

func (s *Service) OK(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fails, ip)
}

func (s *Service) SetCookie(w http.ResponseWriter, sid string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   86400 * 7,
	})
}

func (s *Service) ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   -1,
	})
	s.ClearReturnCookie(w, secure)
}

func (s *Service) SetReturnCookie(w http.ResponseWriter, sid string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     ReturnCookie,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   86400 * 7,
	})
}

func (s *Service) ClearReturnCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     ReturnCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   -1,
	})
}

func ReturnSessionID(r *http.Request) string {
	c, err := r.Cookie(ReturnCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func SessionID(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func ValidUsername(name string) bool {
	if len(name) < 3 || len(name) > 32 {
		return false
	}
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func ValidPassword(pw string) bool {
	return len(pw) >= 8
}
