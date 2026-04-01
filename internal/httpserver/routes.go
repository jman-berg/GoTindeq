package httpserver

// import (
// 	"net/http"
// )

func (s *Server) routes() {
	s.mux.Handle("GET /health", s.handleGetHealth())
	s.mux.Handle("GET /debug/", s.handleGetDebug())
}
