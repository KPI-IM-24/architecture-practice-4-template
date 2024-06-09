package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/KPI-IM-24/architecture-practice-4-template/httptools"
	"github.com/KPI-IM-24/architecture-practice-4-template/pkg/lb"
	"github.com/KPI-IM-24/architecture-practice-4-template/signal"
)

var (
	port       = flag.Int("port", 8090, "load balancer port")
	timeoutSec = flag.Int("timeout-sec", 3, "request timeout time in seconds")
	https      = flag.Bool("https", false, "whether backends support HTTPs")

	traceEnabled = flag.Bool("trace", false, "whether to include tracing information into responses")
)

func main() {
	flag.Parse()

	lb.HealthCheckerV = &lb.DefaultHealthChecker{}
	lb.RequestSenderV = &lb.DefaultRequestSender{}
	lb.ServersPool = []string{"server1:8080", "server2:8080", "server3:8080"}
	lb.Timeout = time.Duration(*timeoutSec) * time.Second
	lb.Https = *https
	lb.TraceEnabled = *traceEnabled

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(1 * time.Minute)
		cancel()
	}()

	lb.HealthCheck(ctx, lb.ServersPool)

	frontend := httptools.CreateServer(*port, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		server := lb.ChooseServer()
		if server == "" {
			http.Error(rw, "No healthy servers available", http.StatusServiceUnavailable)
			return
		}

		lb.Forward(server, rw, r)
	}))

	log.Println("Starting load balancer...")
	log.Printf("Tracing support enabled: %t", lb.TraceEnabled)
	frontend.Start()
	signal.WaitForTerminationSignal()
}
