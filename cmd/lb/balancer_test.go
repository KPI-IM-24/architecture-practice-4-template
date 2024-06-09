package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
)

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

type LoadBalancerSuite struct {
	suite.Suite
}

func (s *LoadBalancerSuite) TestScheme() {
	*https = true
	assert.Equal(s.T(), "https", scheme(), "Expected scheme to be https")

	*https = false
	assert.Equal(s.T(), "http", scheme(), "Expected scheme to be http")
}

func (s *LoadBalancerSuite) TestBalancer() {
	const maxAttempts = 4
	attempts := 0

	healthChecker = &FakeAlwaysTrueHealthChecker{}
	requestSender = &FakeReturnRequestBodyRequestSender{}
	serversPool = []string{"http://server1:1", "http://server2:1", "http://server3:1"}

	ctx, cancel := context.WithCancel(context.Background())

	// Create a goroutine to stop the health check after maxAttempts
	go func() {
		for range time.Tick(10 * time.Second) {
			attempts++
			if attempts >= maxAttempts {
				cancel()
				return
			}
		}
	}()

	healthCheck(ctx, serversPool)

	server := chooseServer()
	assert.NotNil(s.T(), server, "Expected non-nil server")
	assert.Contains(s.T(), server, "http://server", "Expected server to be part of the pool")

	err := forward("http://server1:1", httptest.NewRecorder(), httptest.NewRequest("GET", "http://server1:1", strings.NewReader("body length 14")))
	assert.NoError(s.T(), err, "Expected no error for server1")
	err = forward("http://server3:1", httptest.NewRecorder(), httptest.NewRequest("GET", "http://server3:1", strings.NewReader("body length 14")))
	assert.NoError(s.T(), err, "Expected no error for server3")

	server = chooseServer()
	assert.Equal(s.T(), "http://server2:1", server, "Expected server2 to be chosen")
}

func TestLoadBalancerSuite(t *testing.T) {
	suite.Run(t, new(LoadBalancerSuite))
}
