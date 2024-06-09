package lb

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
)

// Fake implementations for testing
type FakeAlwaysTrueHealthChecker struct{}

func (hc *FakeAlwaysTrueHealthChecker) Check(server string) bool {
	return true
}

type FakeAlwaysFalseHealthChecker struct{}

func (hc *FakeAlwaysFalseHealthChecker) Check(server string) bool {
	return false
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

func setup() {
	// Reset global variables for each test
	HealthCheckerV = &FakeAlwaysTrueHealthChecker{}
	RequestSenderV = &FakeReturnRequestBodyRequestSender{}
	ServersPool = []string{"server1:8080", "server2:8080", "server3:8080"}
	HealthyServersPool = []string{}
	ServerTraffic = make(map[string]int64)
	Timeout = 3 * time.Second
	Https = false
	TraceEnabled = false
}

func TestHealthCheck_AllServersHealthy(t *testing.T) {
	setup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	HealthCheckerV = &FakeAlwaysTrueHealthChecker{}
	go HealthCheck(ctx, ServersPool)

	// Allow some time for the health checks to run
	time.Sleep(1 * time.Second)
	cancel() // Ensure the goroutines stop after the test

	assert.Equal(t, len(HealthyServersPool), len(ServersPool), "All servers should be healthy")
	for _, server := range ServersPool {
		healthy, ok := HealthStatus.Load(server)
		assert.True(t, ok)
		assert.True(t, healthy.(bool))
	}
}

func TestForward_Success(t *testing.T) {
	setup()
	server := "server1:8080"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://example.com", strings.NewReader("body length 14"))

	err := Forward(server, recorder, request)
	assert.NoError(t, err, "Expected no error during request forwarding")

	response := recorder.Result()
	body, _ := ioutil.ReadAll(response.Body)
	assert.Equal(t, 200, response.StatusCode, "Expected 200 OK status")
	assert.Equal(t, "body length 14", string(body), "Expected body to be echoed back")
}

func TestChooseServer(t *testing.T) {
	setup()
	HealthyServersPool = []string{"server1:8080", "server2:8080", "server3:8080"}
	ServerTraffic["server1:8080"] = 100
	ServerTraffic["server2:8080"] = 50
	ServerTraffic["server3:8080"] = 75

	server := ChooseServer()
	assert.Equal(t, "server2:8080", server, "Expected server with least traffic to be chosen")
}

func TestScheme(t *testing.T) {
	Https = true
	assert.Equal(t, "https", scheme(), "Expected scheme to be https")

	Https = false
	assert.Equal(t, "http", scheme(), "Expected scheme to be http")
}
