package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/worklingo/backend/internal/auth"
	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/lifecycle"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/session"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/task"
)

type Handlers struct {
	Q         *store.Queries
	JWT       *auth.JWT
	Sessions  *session.Manager
	Chat      *chat.Service
	Engine    *persona.Engine
	Deadlines *deadline.Service
	Tasks     *task.Service
	Registry  *lifecycle.Registry
	Log       *slog.Logger
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

func (h *Handlers) internal(w http.ResponseWriter, err error) {
	h.Log.Error("internal error", "err", err)
	Error(w, http.StatusInternalServerError, "internal", "internal server error")
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- auth ----

type authResp struct {
	Token string   `json:"token"`
	User  dto.User `json:"user"`
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !usernameRe.MatchString(in.Username) {
		Error(w, http.StatusBadRequest, "invalid_username", "username must be 3-32 characters: letters, digits, underscore")
		return
	}
	if _, err := mail.ParseAddress(in.Email); err != nil {
		Error(w, http.StatusBadRequest, "invalid_email", "invalid email address")
		return
	}
	if len(in.Password) < 8 {
		Error(w, http.StatusBadRequest, "weak_password", "password must be at least 8 characters")
		return
	}
	if _, ok := h.Engine.Catalog().ByMention(strings.ToLower(in.Username)); ok {
		Error(w, http.StatusConflict, "username_taken", "username is reserved")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		h.internal(w, err)
		return
	}
	u, err := h.Q.CreateUser(r.Context(), store.CreateUserParams{Username: in.Username, Email: in.Email, PasswordHash: hash})
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			Error(w, http.StatusConflict, "already_exists", "username or email already registered")
			return
		}
		h.internal(w, err)
		return
	}
	tok, err := h.JWT.Issue(u.ID)
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusCreated, authResp{Token: tok, User: dto.FromUser(u)})
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	u, err := h.Q.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil || !auth.CheckPassword(u.PasswordHash, in.Password) {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			h.internal(w, err)
			return
		}
		Error(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	tok, err := h.JWT.Issue(u.ID)
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, authResp{Token: tok, User: dto.FromUser(u)})
}

func (h *Handlers) currentUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	id, _ := auth.UserID(r.Context())
	u, err := h.Q.GetUserByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			Error(w, http.StatusUnauthorized, "unauthorized", "user not found")
		} else {
			h.internal(w, err)
		}
		return u, false
	}
	return u, true
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	JSON(w, http.StatusOK, dto.FromUser(u))
}

func (h *Handlers) Personas(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, h.Engine.Catalog().DTOs())
}

// ---- sessions ----

func (h *Handlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	s, err := h.Sessions.Create(r.Context(), u)
	if errors.Is(err, session.ErrActiveExists) {
		Error(w, http.StatusConflict, "active_session_exists", "you already have an active session")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusCreated, dto.FromSession(s))
}

func (h *Handlers) ActiveSession(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	s, err := h.Q.GetActiveSessionForUser(r.Context(), uid)
	if errors.Is(err, pgx.ErrNoRows) {
		Error(w, http.StatusNotFound, "not_found", "no active session")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, dto.FromSession(s))
}

func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r.Context())
	ss, err := h.Q.ListSessionsForUser(r.Context(), uid)
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, dto.Map(ss, dto.FromSession))
}

// loadSession fetches the {id} session and checks ownership.
func (h *Handlers) loadSession(w http.ResponseWriter, r *http.Request) (store.Session, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid session id")
		return store.Session{}, false
	}
	s, err := h.Q.GetSession(r.Context(), id)
	uid, _ := auth.UserID(r.Context())
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && s.UserID != uid) {
		Error(w, http.StatusNotFound, "not_found", "session not found")
		return s, false
	}
	if err != nil {
		h.internal(w, err)
		return s, false
	}
	return s, true
}

func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	s, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	chs, err1 := h.Q.ListChannels(ctx, s.ID)
	ts, err2 := h.Q.ListTasks(ctx, s.ID)
	ds, err3 := h.Q.ListDeadlines(ctx, s.ID)
	ws, err4 := h.Q.ListWarnings(ctx, s.ID)
	tps, err5 := h.Q.ListTalkingPointsForSession(ctx, s.ID)
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, dto.SessionDetail{
		Session:   dto.FromSession(s),
		Channels:  dto.Map(chs, dto.FromChannel),
		Tasks:     dto.FromTasks(ts, tps),
		Deadlines: dto.Map(ds, dto.FromDeadline),
		Warnings:  dto.Map(ws, dto.FromWarning),
	})
}

func (h *Handlers) EndSession(w http.ResponseWriter, r *http.Request) {
	s, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	if s.Status != "ACTIVE" {
		JSON(w, http.StatusOK, dto.FromSession(s)) // idempotent
		return
	}
	out, err := h.Sessions.End(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		s, _ = h.Q.GetSession(r.Context(), s.ID)
		JSON(w, http.StatusOK, dto.FromSession(s))
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, dto.FromSession(out))
}

func (h *Handlers) Evaluation(w http.ResponseWriter, r *http.Request) {
	s, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	e, err := h.Q.GetEvaluation(r.Context(), s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		Error(w, http.StatusNotFound, "not_found", "evaluation not started")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, dto.FromEvaluation(e))
}

// ---- messages ----

func (h *Handlers) loadChannel(w http.ResponseWriter, r *http.Request, s store.Session) (store.Channel, bool) {
	cid, err := uuid.Parse(chi.URLParam(r, "channelId"))
	if err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid channel id")
		return store.Channel{}, false
	}
	ch, err := h.Q.GetChannel(r.Context(), cid)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && ch.SessionID != s.ID) {
		Error(w, http.StatusNotFound, "not_found", "channel not found")
		return ch, false
	}
	if err != nil {
		h.internal(w, err)
		return ch, false
	}
	return ch, true
}

func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request) {
	s, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	ch, ok := h.loadChannel(w, r, s)
	if !ok {
		return
	}
	before := time.Now().Add(time.Hour)
	if b := r.URL.Query().Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			Error(w, http.StatusBadRequest, "bad_request", "before must be RFC3339")
			return
		}
		before = t
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			Error(w, http.StatusBadRequest, "bad_request", "invalid limit")
			return
		}
		limit = min(n, 200)
	}
	ms, err := h.Q.ListMessagesBefore(r.Context(), store.ListMessagesBeforeParams{ChannelID: ch.ID, CreatedAt: before, Limit: int32(limit)})
	if err != nil {
		h.internal(w, err)
		return
	}
	JSON(w, http.StatusOK, dto.Map(ms, dto.FromMessage))
}

func (h *Handlers) PostMessage(w http.ResponseWriter, r *http.Request) {
	s, ok := h.loadSession(w, r)
	if !ok {
		return
	}
	ch, ok := h.loadChannel(w, r, s)
	if !ok {
		return
	}
	if s.Status != "ACTIVE" {
		Error(w, http.StatusConflict, "session_not_active", "session is not active")
		return
	}
	var in struct {
		Content  string  `json:"content"`
		ParentID *string `json:"parentId"`
	}
	if err := decode(r, &in); err != nil {
		Error(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" || utf8.RuneCountInString(in.Content) > 4000 {
		Error(w, http.StatusBadRequest, "invalid_content", "content must be 1-4000 characters")
		return
	}
	var parent *uuid.UUID
	if in.ParentID != nil && *in.ParentID != "" {
		pid, err := uuid.Parse(*in.ParentID)
		if err != nil {
			Error(w, http.StatusBadRequest, "bad_request", "invalid parentId")
			return
		}
		pm, err := h.Q.GetMessage(r.Context(), pid)
		if err != nil || pm.ChannelID != ch.ID {
			Error(w, http.StatusBadRequest, "bad_request", "parent message not in this channel")
			return
		}
		if pm.ParentID != nil { // flatten nested threads to the root
			pid = *pm.ParentID
		}
		parent = &pid
	}
	u, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	m, err := h.Chat.PostUser(r.Context(), ch, u.ID, in.Content, parent)
	if err != nil {
		h.internal(w, err)
		return
	}
	h.Tasks.OnUserMessage(ch, u, m)
	JSON(w, http.StatusCreated, dto.FromMessage(m))
}
