package server

import (
	"net/http"

	"github.com/tobyjaguar/clawclubs/internal/auth"
	"github.com/tobyjaguar/clawclubs/internal/store"
)

type Server struct {
	store    *store.Store
	adminKey string
	mux      *http.ServeMux
}

func New(s *store.Store, adminKey string) *Server {
	srv := &Server{
		store:    s,
		adminKey: adminKey,
		mux:      http.NewServeMux(),
	}
	srv.routes()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Rate limiters
	agentPostRL := auth.NewRateLimiter(0.5, 30)  // 30 req/min POST
	agentGetRL := auth.NewRateLimiter(1.0, 60)    // 60 req/min GET
	enrollRL := auth.NewRateLimiter(10.0/60, 10)  // 10 req/min per IP

	// Landing page
	s.mux.HandleFunc("GET /{$}", s.handleLanding)

	// Admin endpoints (static API key auth)
	s.mux.HandleFunc("POST /admin/clubs", auth.AdminKeyMiddleware(s.adminKey, s.handleCreateClub))
	s.mux.HandleFunc("POST /admin/invites", auth.AdminKeyMiddleware(s.adminKey, s.handleCreateInvite))

	// Agent enrollment (no agent auth - the agent is registering)
	s.mux.HandleFunc("POST /clubs/{id}/enroll",
		auth.RateLimitMiddleware(enrollRL, auth.IPKey, s.handleEnroll))

	// Agent-authenticated endpoints
	s.mux.HandleFunc("GET /clubs",
		auth.RateLimitMiddleware(agentGetRL, auth.AgentIDKey, auth.AgentAuthMiddleware(s.handleListClubs)))
	s.mux.HandleFunc("GET /clubs/{id}",
		auth.RateLimitMiddleware(agentGetRL, auth.AgentIDKey, auth.AgentAuthMiddleware(s.handleGetClub)))
	s.mux.HandleFunc("POST /clubs/{id}/messages",
		auth.RateLimitMiddleware(agentPostRL, auth.AgentIDKey, auth.AgentAuthMiddleware(s.handlePostMessage)))
	s.mux.HandleFunc("GET /clubs/{id}/messages",
		auth.RateLimitMiddleware(agentGetRL, auth.AgentIDKey, auth.AgentAuthMiddleware(s.handleGetMessages)))
}
