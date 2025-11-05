package server

import (
	"encoding/json"
	"io"
	"kv-server/internal/cache"
	"kv-server/internal/database"
	"net/http"
	"strings"
)

type KVServer struct {
	cache *cache.LRUCache
	db    *database.PostgresDB
}

type Request struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Response struct {
	Success bool   `json:"success"`
	Value   string `json:"value,omitempty"`
	Error   string `json:"error,omitempty"`
}

func NewKVServer(cacheSize int, db *database.PostgresDB) *KVServer {
	return &KVServer{
		cache: cache.NewLRUCache(cacheSize),
		db:    db,
	}
}

func (s *KVServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/kv/")

	switch r.Method {
	case http.MethodPost:
		s.handleCreate(w, r)
	case http.MethodGet:
		s.handleRead(w, r, path)
	case http.MethodDelete:
		s.handleDelete(w, r, path)
	default:
		s.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *KVServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendError(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		s.sendError(w, "invalid json", http.StatusBadRequest)
		return
	}

	if req.Key == "" {
		s.sendError(w, "key is required", http.StatusBadRequest)
		return
	}

	// Store in database first
	if err := s.db.Create(req.Key, req.Value); err != nil {
		s.sendError(w, "database error", http.StatusInternalServerError)
		return
	}

	// Then update cache
	s.cache.Put(req.Key, req.Value)

	s.sendSuccess(w, "", http.StatusCreated)
}

func (s *KVServer) handleRead(w http.ResponseWriter, r *http.Request, key string) {
	if key == "" {
		s.sendError(w, "key is required", http.StatusBadRequest)
		return
	}

	// Check cache first
	if value, ok := s.cache.Get(key); ok {
		s.sendSuccess(w, value, http.StatusOK)
		return
	}

	// Cache miss - read from database
	value, err := s.db.Read(key)
	if err != nil {
		s.sendError(w, "key not found", http.StatusNotFound)
		return
	}

	// Add to cache
	s.cache.Put(key, value)

	s.sendSuccess(w, value, http.StatusOK)
}

func (s *KVServer) handleDelete(w http.ResponseWriter, r *http.Request, key string) {
	if key == "" {
		s.sendError(w, "key is required", http.StatusBadRequest)
		return
	}

	// Delete from database
	if err := s.db.Delete(key); err != nil {
		s.sendError(w, "key not found", http.StatusNotFound)
		return
	}

	// Delete from cache if exists
	s.cache.Delete(key)

	s.sendSuccess(w, "", http.StatusOK)
}

func (s *KVServer) sendSuccess(w http.ResponseWriter, value string, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Value:   value,
	})
}

func (s *KVServer) sendError(w http.ResponseWriter, errMsg string, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Response{
		Success: false,
		Error:   errMsg,
	})
}

func (s *KVServer) GetCacheStats() (hits, misses uint64) {
	return s.cache.GetStats()
}
