package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/seongJae/owlmon/server/audit"
	"github.com/seongJae/owlmon/server/auth"
	"github.com/seongJae/owlmon/server/db"
	"golang.org/x/crypto/bcrypt"
)

var validUsername = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

type UsersHandler struct {
	store *db.UserStore
}

func NewUsersHandler(store *db.UserStore) *UsersHandler {
	return &UsersHandler{store: store}
}

func validRole(r string) bool { return r == auth.RoleAdmin || r == auth.RoleViewer }

// List GET /api/users
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	if users == nil {
		users = []db.User{}
	}
	writeJSON(w, users)
}

// Create POST /api/users {username, password, role}
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}
	if !validUsername.MatchString(req.Username) {
		http.Error(w, "아이디는 영문·숫자·._- 3~32자여야 합니다", http.StatusBadRequest)
		return
	}
	if !validRole(req.Role) {
		http.Error(w, "역할은 admin 또는 viewer여야 합니다", http.StatusBadRequest)
		return
	}
	if msg := passwordPolicy(req.Password); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	u, err := h.store.Create(r.Context(), req.Username, string(hash), req.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "이미 있는 아이디입니다", http.StatusConflict)
			return
		}
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	audit.Log(r.Context(), "user.create", "user", u.Username, map[string]any{"role": u.Role})
	writeJSON(w, u)
}

// Update PATCH /api/users/{id} {role, disabled, password?} — password가 있으면 초기화
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "잘못된 ID", http.StatusBadRequest)
		return
	}
	var req struct {
		Role     string `json:"role"`
		Disabled bool   `json:"disabled"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청입니다", http.StatusBadRequest)
		return
	}
	if !validRole(req.Role) {
		http.Error(w, "역할은 admin 또는 viewer여야 합니다", http.StatusBadRequest)
		return
	}
	name, err := h.store.UsernameByID(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "계정을 찾을 수 없습니다", http.StatusNotFound)
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	if req.Password != "" {
		if msg := passwordPolicy(req.Password); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
	}

	if err := h.store.Update(r.Context(), id, req.Role, req.Disabled); err != nil {
		if errors.Is(err, db.ErrLastAdmin) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
		return
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err == nil {
			keep := ""
			if name == auth.UsernameFromContext(r.Context()) { // 자기 비밀번호를 초기화해도 지금 화면은 유지
				keep = auth.SessionFromContext(r.Context())
			}
			err = h.store.SetPassword(r.Context(), name, string(hash), keep)
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, "서버 내부 오류", err)
			return
		}
	}
	audit.Log(r.Context(), "user.update", "user", name, map[string]any{
		"role": req.Role, "disabled": req.Disabled, "password_reset": req.Password != "",
	})
	w.WriteHeader(http.StatusNoContent)
}
