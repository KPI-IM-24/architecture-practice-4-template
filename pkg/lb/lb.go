package lb

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	Timeout            time.Duration
	ServersPool        []string
	HealthyServersPool []string
	HealthCheckerV     HealthChecker
	RequestSenderV     RequestSender
	ServerTraffic      = make(map[string]int64)
	LockPool           sync.Mutex
	HealthStatus       sync.Map
	Https              bool
	TraceEnabled       bool
)

type HealthChecker interface {
	Check(string) bool
}

type DefaultHealthChecker struct{}

func (hc *DefaultHealthChecker) Check(dst string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s://%s/health", scheme(), dst), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	return true
}

type RequestSender interface {
	Send(*http.Request) (*http.Response, error)
}

type DefaultRequestSender struct{}

func (rs *DefaultRequestSender) Send(fwdRequest *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(fwdRequest)
}

func scheme() string {
	if Https {
		return "https"
	}
	return "http"
}

func HealthCheck(ctx context.Context, servers []string) {
	var wg sync.WaitGroup

	for _, server := range servers {
		HealthStatus.Store(server, true)
		HealthyServersPool = append(HealthyServersPool, server)
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
					isHealthy := HealthCheckerV.Check(server)
					LockPool.Lock()
					HealthStatus.Store(server, isHealthy)

					if isHealthy {
						if !Contains(HealthyServersPool, server) {
							HealthyServersPool = append(HealthyServersPool, server)
						}
					} else {
						HealthyServersPool = Remove(HealthyServersPool, server)
					}

					LockPool.Unlock()
					log.Println(server, isHealthy)
				}
			}
		}(server)
	}

	wg.Wait()
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func Remove(slice []string, item string) []string {
	for i, s := range slice {
		if s == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func Forward(dst string, rw http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), Timeout)
	defer cancel()
	fwdRequest := r.Clone(ctx)
	fwdRequest.RequestURI = ""
	fwdRequest.URL.Host = dst
	fwdRequest.URL.Scheme = scheme()
	fwdRequest.Host = dst

	resp, err := RequestSenderV.Send(fwdRequest)
	if err == nil {
		defer resp.Body.Close()

		for k, values := range resp.Header {
			for _, value := range values {
				rw.Header().Add(k, value)
			}
		}
		if TraceEnabled {
			rw.Header().Set("lb-from", dst)
		}
		log.Println("fwd", resp.StatusCode, resp.Request.URL)
		rw.WriteHeader(resp.StatusCode)

		n, err := io.Copy(rw, resp.Body)
		if err != nil {
			log.Printf("Failed to write response: %s", err)
			return err
		}

		LockPool.Lock()
		ServerTraffic[dst] += n
		LockPool.Unlock()

		return nil
	} else {
		log.Printf("Failed to get response from %s: %s", dst, err)
		rw.WriteHeader(http.StatusServiceUnavailable)
		return err
	}
}

func ChooseServer() string {
	LockPool.Lock()
	defer LockPool.Unlock()

	var minTrafficServer string
	var minTraffic int64 = int64(^uint64(0) >> 1) // initialize to max int64 value

	for _, server := range HealthyServersPool {
		traffic := ServerTraffic[server]
		if traffic < minTraffic {
			minTraffic = traffic
			minTrafficServer = server
		}
	}

	return minTrafficServer
}
