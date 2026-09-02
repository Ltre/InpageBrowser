package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Credential struct {
	ID           string `json:"id"`
	PublicKeyDER string `json:"public_key_der"`
	SignCount    uint32 `json:"sign_count"`
	CreatedAt    string `json:"created_at"`
}

type User struct {
	ID          string       `json:"id"`
	Phone       string       `json:"phone"`
	UserHandle  string       `json:"user_handle"`
	Credentials []Credential `json:"credentials"`
	CreatedAt   string       `json:"created_at"`
}

type Session struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	ExpiresAt string `json:"expires_at"`
}

type state struct {
	Users    []User    `json:"users"`
	Sessions []Session `json:"sessions"`
}

type Store struct {
	mu   sync.Mutex
	path string
	st   state
}

const SessionTTL = 30 * 24 * time.Hour

func Open(path string) (*Store, error) {
	s := &Store{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.st); err != nil {
			return nil, fmt.Errorf("load auth store: %w", err)
		}
	}
	return s, nil
}

func NormalizePhone(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("请输入手机号")
	}
	if v[0] == '+' {
		v = "+" + strings.ReplaceAll(v[1:], " ", "")
	} else {
		v = strings.ReplaceAll(v, " ", "")
	}
	digits := strings.TrimPrefix(v, "+")
	if len(digits) < 6 || len(digits) > 20 {
		return "", errors.New("手机号长度无效")
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", errors.New("手机号只能包含数字和可选的 + 前缀")
		}
	}
	return v, nil
}

func (s *Store) CreateUser(phone, userHandle, credID, pubDER string, signCount uint32) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.st.Users {
		if u.Phone == phone {
			return User{}, errors.New("该手机号已经注册")
		}
	}
	uid, err := randomToken(18)
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	u := User{ID: uid, Phone: phone, UserHandle: userHandle, CreatedAt: now, Credentials: []Credential{{ID: credID, PublicKeyDER: pubDER, SignCount: signCount, CreatedAt: now}}}
	s.st.Users = append(s.st.Users, u)
	return u, s.saveLocked()
}

func (s *Store) UserByPhone(phone string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.st.Users {
		if u.Phone == phone {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) UpdateSignCount(userID, credID string, count uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ui := range s.st.Users {
		if s.st.Users[ui].ID != userID {
			continue
		}
		for ci := range s.st.Users[ui].Credentials {
			if s.st.Users[ui].Credentials[ci].ID == credID && count > s.st.Users[ui].Credentials[ci].SignCount {
				s.st.Users[ui].Credentials[ci].SignCount = count
				return s.saveLocked()
			}
		}
	}
	return nil
}

func (s *Store) NewSession(userID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	keep := s.st.Sessions[:0]
	for _, sess := range s.st.Sessions {
		exp, _ := time.Parse(time.RFC3339Nano, sess.ExpiresAt)
		if exp.After(now) {
			keep = append(keep, sess)
		}
	}
	s.st.Sessions = append(keep, Session{Token: token, UserID: userID, ExpiresAt: now.Add(SessionTTL).Format(time.RFC3339Nano)})
	return token, s.saveLocked()
}

func (s *Store) UserFromSession(token string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var userID string
	for _, sess := range s.st.Sessions {
		exp, _ := time.Parse(time.RFC3339Nano, sess.ExpiresAt)
		if sess.Token == token && exp.After(now) {
			userID = sess.UserID
			break
		}
	}
	if userID == "" {
		return User{}, false
	}
	for _, u := range s.st.Users {
		if u.ID == userID {
			return u, true
		}
	}
	return User{}, false
}

func (s *Store) DeleteSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.st.Sessions[:0]
	for _, sess := range s.st.Sessions {
		if sess.Token != token {
			out = append(out, sess)
		}
	}
	s.st.Sessions = out
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
