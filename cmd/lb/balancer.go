package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/KPI-IM-24/architecture-practice-4-template/httptools"
	"github.com/KPI-IM-24/architecture-practice-4-template/signal"
)

var (
	port       = flag.Int("port", 8090, "load balancer port")
	timeoutSec = flag.Int("timeout-sec", 3, "request timeout time in seconds")
	https      = flag.Bool("https", false, "whether backends support HTTPs")

	traceEnabled = flag.Bool("trace", false, "whether to include tracing information into responses")
)

var (
	timeout            = time.Duration(*timeoutSec) * time.Second
	serversPool        = []string{"server1:8080", "server2:8080", "server3:8080"}
	healthyServersPool []string
	lockPool           sync.Mutex
	healthStatus       sync.Map
)

func scheme() string {
	if *https {
		return "https"
	}
	return "http"
}

func health(dst string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s://%s/health", scheme(), dst), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	return true
}

func healthCheck(ctx context.Context, servers []string) {
	var wg sync.WaitGroup

	for _, server := range servers {
		healthStatus.Store(server, true)
		healthyServersPool = append(healthyServersPool, server)
	}

	for _, server := range servers {
		wg.Add(1)
		go func(server string) {
			defer wg.Done()
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					isHealthy := health(server)
					lockPool.Lock()
					healthStatus.Store(server, isHealthy)

					if isHealthy {
						if !contains(healthyServersPool, server) {
							healthyServersPool = append(healthyServersPool, server)
						}
					} else {
						healthyServersPool = remove(healthyServersPool, server)
					}

					lockPool.Unlock()
					log.Println(server, isHealthy)
				}
			}
		}(server)
	}

	wg.Wait()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func remove(slice []string, item string) []string {
	for i, s := range slice {
		if s == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func forward(dst string, rw http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	fwdRequest := r.Clone(ctx)
	fwdRequest.RequestURI = ""
	fwdRequest.URL.Host = dst
	fwdRequest.URL.Scheme = scheme()
	fwdRequest.Host = dst

	resp, err := http.DefaultClient.Do(fwdRequest)
	if err == nil {
		for k, values := range resp.Header {
			for _, value := range values {
				rw.Header().Add(k, value)
			}
		}
		if *traceEnabled {
			rw.Header().Set("lb-from", dst)
		}
		log.Println("fwd", resp.StatusCode, resp.Request.URL)
		rw.WriteHeader(resp.StatusCode)
		defer resp.Body.Close()
		_, err := io.Copy(rw, resp.Body)
		if err != nil {
			log.Printf("Failed to write response: %s", err)
		}
		return nil
	} else {
		log.Printf("Failed to get response from %s: %s", dst, err)
		rw.WriteHeader(http.StatusServiceUnavailable)
		return err
	}
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go healthCheck(ctx, serversPool)

	frontend := httptools.CreateServer(*port, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		lockPool.Lock()
		defer lockPool.Unlock()

		if len(healthyServersPool) == 0 {
			http.Error(rw, "No healthy servers available", http.StatusServiceUnavailable)
			return
		}

		// Simple round-robin load balancing
		server := healthyServersPool[0]
		healthyServersPool = append(healthyServersPool[1:], server)

		if err := forward(server, rw, r); err != nil {
			log.Printf("Failed to forward request to %s: %s", server, err)
		}
	}))

	log.Println("Starting load balancer...")
	log.Printf("Tracing support enabled: %t", *traceEnabled)
	frontend.Start()
	signal.WaitForTerminationSignal()
}
