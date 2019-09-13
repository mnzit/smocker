package main

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/labstack/echo"
	log "github.com/sirupsen/logrus"
)

type MockServer interface {
	AddMock(Mock)
	Mocks() Mocks
	History(filterPath string) (History, error)
	Reset()
}

type mockServer struct {
	server  *echo.Echo
	mocks   Mocks
	history History
}

func NewMockServer(port int) MockServer {
	s := &mockServer{
		server:  echo.New(),
		mocks:   Mocks{},
		history: History{},
	}

	s.server.HideBanner = true
	s.server.HidePort = true
	s.server.Use(s.historyMiddleware())
	s.server.Any("/*", s.genericHandler)

	log.WithField("port", port).Info("Starting mock server")
	go func() {
		if err := s.server.Start(":" + strconv.Itoa(port)); err != nil {
			log.WithError(err).Error("Mock Server execution failed")
		}
	}()

	return s
}

func (s *mockServer) genericHandler(c echo.Context) error {
	actualRequest := HTTPRequestToRequest(c.Request())

	// Query Params
	var response *MockResponse
	for _, mock := range s.mocks {
		isSameMethod := mock.Request.Method == actualRequest.Method
		isMatchingPath := mock.Request.Path == actualRequest.Path
		isMatchingQuery := mock.Request.QueryParams.Equals(actualRequest.QueryParams)
		if isSameMethod && isMatchingPath && isMatchingQuery {
			if mock.DynamicResponse != nil {
				response = mock.DynamicResponse.ToMockResponse(actualRequest)
			} else {
				response = mock.Response
			}
			break
		}
	}
	if response == nil {
		return c.JSON(http.StatusNotFound, echo.Map{
			"message": "No mock found matching the request",
			"request": actualRequest,
		})
	}

	// Headers
	for key, values := range response.Headers {
		for _, value := range values {
			c.Response().Header().Add(key, value)
		}
	}

	// Delay
	time.Sleep(response.Delay)

	// Status
	c.Response().WriteHeader(response.Status)

	// Body
	if _, err := c.Response().Write([]byte(response.Body)); err != nil {
		log.WithError(err).Error("Failed to write response body")
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return nil
}

func (s *mockServer) AddMock(newMock Mock) {
	s.mocks = append(Mocks{newMock}, s.mocks...)
}

func (s *mockServer) Mocks() Mocks {
	return s.mocks
}

func (s *mockServer) History(filterPath string) (History, error) {
	res := History{}
	regex, err := regexp.Compile(filterPath)
	if err != nil {
		return res, err
	}
	for _, entry := range s.history {
		if regex.Match([]byte(entry.Request.Path)) {
			res = append(res, entry)
		}
	}
	return res, nil
}

func (s *mockServer) Reset() {
	s.mocks = Mocks{}
	s.history = History{}
}
