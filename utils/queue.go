package utils

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"time"
)

// ConfigureQueue Queue Access for environment is configured here
func ConfigureQueue(log *logrus.Entry) (stan.Conn, error) {

	queueURL := GetEnv("QUEUE_URL", nats.DefaultURL)
	clusterID := GetEnv("QUEUE_CLUSTER_ID", "smpp_cluster")
	clientID := GetEnv("QUEUE_CLIENT_ID", fmt.Sprintf("smpp_router", ))

	// Connect to a server
	nc, err := nats.Connect(queueURL,
		nats.ReconnectBufSize(50*1024*1024), nats.ReconnectWait(10*time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Infof("queue got disconnected! Reason: %q", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Infof("queue got reconnected to %v!\n", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			log.Infof("queue connection closed. Reason: %q\n", nc.LastError())
		}))

	if err != nil {
		return nil, err
	}

	return stan.Connect(clusterID, clientID, stan.NatsConn(nc))
}
