package utils

import (
	"encoding/json"
	"fmt"
	health "github.com/InVisionApp/go-health/v2"
	"github.com/InVisionApp/go-logger"
	goLogLogrus "github.com/InVisionApp/go-logger/shims/logrus"

	"github.com/sirupsen/logrus"
	"net/http"
	"sync"
	"time"
)

type jsonStatus struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type mutexMap struct {
	sync.Mutex
	data map[string]interface{}
}

func ConfigureHealthChecker(realLog *logrus.Entry,  queueChecker *StanQueue) (*health.Health, error) {

	h := health.New()

	//Configure logging
	logger := goLogLogrus.New(realLog.Logger)

	fields := log.Fields{}
	for k, v := range realLog.Data {
		fields[k] = v
	}
	h.Logger = logger.WithFields(fields)

	// Start our defined checkers
	if err := h.AddChecks([]*health.Config{
		{
			Name:     "Stan queue",
			Checker:  queueChecker,
			Interval: time.Duration(5) * time.Second,
			Fatal:    true,
		},
	}); err != nil {
		return nil, err
	}

	if err := h.Start(); err != nil {
		return nil, err
	}

	return h, nil

}

func HealthCheckProcessing(logger *logrus.Entry, healthChecker *health.Health, ) (int, []byte) {

	states, failed, err := healthChecker.State()
	if err != nil {
		return http.StatusOK, buildResponse(logger, "error", fmt.Sprintf("Unable to fetch states: %v", err))
	}

	msg := "ok"
	statusCode := http.StatusOK

	// There may be an _initial_ delay in display healthcheck data as the
	// healthcheck will only begin firing at "initialTime + checkIntervalTime"
	if len(states) == 0 {
		return statusCode, buildResponse(logger, msg, "Healthcheck spinning up")
	}

	if failed {
		msg = "failed"
		statusCode = http.StatusInternalServerError
	}

	fullBody := mutexMap{}
	fullBody.Lock()
	fullBody.data = map[string]interface{}{
		"status":  msg,
		"details": states,
	}

	data, err := json.Marshal(fullBody.data)
	fullBody.Unlock()
	if err != nil {
		return http.StatusOK, buildResponse(logger, "error", fmt.Sprintf("Failed to marshal state data: %v", err))
	}

	return statusCode, data
}

func buildResponse(logger *logrus.Entry, status string, message string) []byte {
	resp, err := json.Marshal(&jsonStatus{
		Message: message,
		Status:  status,
	})

	if err != nil {
		logger.Warnf("could not formart healthcheck check response %s", err)
	}
	return resp
}
