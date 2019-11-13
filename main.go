package main

import (
	"antinvestor.com/service/routep/service"
	"antinvestor.com/service/routep/utils"
	"log"
	"os"
	"time"
)

func main() {

	serviceName := "Sms Route"

	logger, err := utils.ConfigureLogging(serviceName)
	if err != nil {
		log.Fatal("Failed to configure logging: " + err.Error())
	}

	traceCloser, err := utils.ConfigureJuegler(serviceName)
	if err != nil {
		logger.Fatal("Failed to configure Juegler: " + err.Error())
	}

	defer traceCloser.Close()

	queue, queueChecker, err := utils.ConfigureQueue(logger)
	if err != nil {
		logger.Warnf("Configuring StanQueue experienced an error: %v", err)
		os.Exit(1)
	}
	defer queue.Close()

	healthChecker, err := utils.ConfigureHealthChecker(logger, queueChecker)
	if err != nil {
		logger.Warnf("Error configuring health checks: %v", err)
	}


	logger.Infof("Initiating the service at %v", time.Now())

	env := service.Env{
		Queue:      queue,
		Logger:     logger,
		Health:     healthChecker,
		ServerPort: utils.GetEnv("SERVER_PORT", "7000"),
	}

	service.RunServer(&env)

}
