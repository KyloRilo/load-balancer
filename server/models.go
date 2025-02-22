package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type Backend struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

func (b *Backend) CheckAlive() {
	log.Print("LoadBalancer.ServerPool.Backend.CheckAlive() => Checking ", b.URL.Host)
	var isAlive bool = true
	timeout := 2 * time.Second
	if conn, err := net.DialTimeout("tcp", b.URL.String(), timeout); err != nil {
		log.Println("Site unreachable, error: ", err)
		isAlive = false
	} else {
		isAlive = true
		defer conn.Close()
	}

	b.SetAlive(isAlive)
}

func (b *Backend) SetAlive(alive bool) {
	log.Printf("LoadBalancer.ServerPool.Backend.SetAlive() => %s [%v]\n", b.URL, alive)
	b.mux.Lock()
	b.Alive = alive
	b.mux.Unlock()
}

func (b *Backend) IsAlive() (alive bool) {
	b.mux.RLock()
	alive = b.Alive
	b.mux.RUnlock()
	return
}

type ServerPool struct {
	backends []*Backend
	current  uint64
}

func (s *ServerPool) AddBackend(backend *Backend) {
	s.backends = append(s.backends, backend)
}

func (s *ServerPool) NextIndex() int {
	return int(atomic.AddUint64(&s.current, uint64(1)) % uint64(len(s.backends)))
}

func (s *ServerPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	for _, backend := range s.backends {
		if backend.URL.String() == backendUrl.String() {
			backend.SetAlive(alive)
			break
		}
	}
}

func (s *ServerPool) GetNextPeer() *Backend {
	next := s.NextIndex()
	l := len(s.backends) + next
	for i := next; i < l; i++ {
		idx := i % len(s.backends)
		if s.backends[idx].IsAlive() {
			if i != next {
				atomic.StoreUint64(&s.current, uint64(idx))
			}
			return s.backends[idx]
		}
	}
	return nil
}

func (s *ServerPool) HealthCheck() {
	for _, backend := range s.backends {
		backend.CheckAlive()
	}
}

type LoadBalancer struct {
	serverPool ServerPool
}

func (lb *LoadBalancer) getAttemptsFromContext(req *http.Request) int {
	attempts := 1
	if val, ok := req.Context().Value(Attempts).(int); ok {
		attempts = val
	}

	return attempts
}

func (lb *LoadBalancer) getRetryFromContext(req *http.Request) int {
	retries := 0
	if val, ok := req.Context().Value(Retry).(int); ok {
		retries = val
	}

	return retries
}

func (lb *LoadBalancer) balance(writer http.ResponseWriter, req *http.Request) {
	attempts := lb.getAttemptsFromContext(req)
	if attempts > 3 {
		log.Printf("%s(%s) Max attempts reached, terminating\n", req.RemoteAddr, req.URL.Path)
		http.Error(writer, "Service not available", http.StatusServiceUnavailable)
		return
	}

	peer := lb.serverPool.GetNextPeer()
	if peer != nil {
		peer.ReverseProxy.ServeHTTP(writer, req)
		return
	}
	http.Error(writer, "Service not available", http.StatusServiceUnavailable)
}

func (lb *LoadBalancer) healthCheck() {
	ticker := time.NewTicker(time.Minute * 2)
	for {
		select {
		case <-ticker.C:
			log.Println("Starting health check...")
			lb.serverPool.HealthCheck()
		}
	}
}

func (lb *LoadBalancer) AddConnection(tok string) error {
	log.Print("Adding Connection => ", tok)
	serverUrl, err := url.Parse(tok)
	if err != nil {
		log.Printf("LoadBalancer.AddConnection() => token: %s, error: %s", tok, err)
		return err
	}

	log.Print("Parsed url => ", serverUrl)

	proxy := httputil.NewSingleHostReverseProxy(serverUrl)
	proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
		log.Printf("[%s] %s\n", serverUrl.Host, e.Error())
		retries := lb.getRetryFromContext(request)
		if retries < 3 {
			select {
			case <-time.After(10 * time.Millisecond):
				ctx := context.WithValue(request.Context(), Retry, retries+1)
				proxy.ServeHTTP(writer, request.WithContext(ctx))
			}
			return
		}

		// after 3 retries, mark this backend as down
		lb.serverPool.MarkBackendStatus(serverUrl, false)

		// if the same request routing for few attempts with different backends, increase the count
		attempts := lb.getAttemptsFromContext(request)
		log.Printf("%s(%s) Attempting retry %d\n", request.RemoteAddr, request.URL.Path, attempts)
		ctx := context.WithValue(request.Context(), Attempts, attempts+1)
		lb.balance(writer, request.WithContext(ctx))
	}

	backend := &Backend{
		URL:          serverUrl,
		Alive:        true,
		ReverseProxy: proxy,
	}

	backend.CheckAlive()
	if backend.IsAlive() {
		log.Printf("Configured server: %s\n", serverUrl)
		lb.serverPool.AddBackend(backend)
	} else {
		return fmt.Errorf("LoadBalancer.AddConnection() => Unable to healthcheck server, token: %s", tok)
	}

	return nil
}

func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		serverPool: ServerPool{},
	}
}
