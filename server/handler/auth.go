package handler

import (
	"encoding/json"
	"net/http"

	"github.com/seongJae/owlmon/server/auth"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	username     string
	passwordHash string // bcrypt 해시
	jwtSecret    string
}

func NewAuthHandler(username, passwordHash, jwtSecret string) *AuthHandler {
	return &AuthHandler{username: username, passwordHash: passwordHash, jwtSecret: jwtSecret}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// Login은 아이디/비밀번호를 확인하고 JWT를 발급합니다.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}

	// 아이디 확인
	if req.Username != h.username {
		http.Error(w, "아이디 또는 비밀번호가 올바르지 않습니다", http.StatusUnauthorized)
		return
	}

	// 비밀번호 확인 (bcrypt)
	if err := bcrypt.CompareHashAndPassword([]byte(h.passwordHash), []byte(req.Password)); err != nil {
		http.Error(w, "아이디 또는 비밀번호가 올바르지 않습니다", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(req.Username, h.jwtSecret)
	if err != nil {
		http.Error(w, "토큰 생성 실패", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResponse{Token: token})
}
