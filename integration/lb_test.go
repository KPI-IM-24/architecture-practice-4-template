package integration

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	lb "github.com/KPI-IM-24/architecture-practice-4-template/pkg/lb"
)

// Fake implementations for testing
type FakeAlwaysTrueHealthChecker struct{}

func (hc *FakeAlwaysTrueHealthChecker) Check(server string) bool {
	return true
}

type FakeReturnRequestBodyRequestSender struct{}

func (rs *FakeReturnRequestBodyRequestSender) Send(request *http.Request) (*http.Response, error) {
	bodyBytes, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(bytes.NewBuffer(bodyBytes)),
		Request:    &http.Request{},
		Close:      false,
	}, nil
}

type LoadBalancerIntegrationSuite struct {
	suite.Suite
}

func (s *LoadBalancerIntegrationSuite) SetupTest() {
	// Reset global variables for each test
	lb.HealthCheckerV = &FakeAlwaysTrueHealthChecker{}
	lb.RequestSenderV = &FakeReturnRequestBodyRequestSender{}
	lb.ServersPool = []string{"server1:8080", "server2:8080", "server3:8080"}
	lb.HealthyServersPool = []string{}
	lb.ServerTraffic = make(map[string]int64)
	lb.Https = false
	lb.TraceEnabled = false
}

func (s *LoadBalancerIntegrationSuite) TestLoadBalancer() {
	// Setup context with cancellation to limit the test duration
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run health checks in a separate goroutine
	go func() {
		lb.HealthCheck(ctx, lb.ServersPool)
	}()

	// Allow some time for the health checks to run
	time.Sleep(3 * time.Second)

	// Test request forwarding
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://example.com", strings.NewReader("body length 14"))

	server := lb.ChooseServer()
	assert.NotNil(s.T(), server, "Expected non-nil server")

	err := lb.Forward(server, recorder, request)
	assert.NoError(s.T(), err, "Expected no error during request forwarding")

	// Ensure response is as expected
	response := recorder.Result()
	body, _ := ioutil.ReadAll(response.Body)
	assert.Equal(s.T(), 200, response.StatusCode, "Expected 200 OK status")
	assert.Equal(s.T(), "body length 14", string(body), "Expected body to be echoed back")

	// Test server selection logic after some requests
	server = lb.ChooseServer()
	assert.NotNil(s.T(), server, "Expected non-nil server")
	assert.Contains(s.T(), lb.ServersPool, server, "Expected chosen server to be in the pool")
}

func TestLoadBalancerIntegrationSuite(t *testing.T) {
	suite.Run(t, new(LoadBalancerIntegrationSuite))
}
