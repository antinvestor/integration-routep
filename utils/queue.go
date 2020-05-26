package utils

import (
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
	"github.com/sirupsen/logrus"
	"time"
)

// ConfigureQueue StanQueue Access for environment is configured here
func ConfigureQueue(log *logrus.Entry) (stan.Conn, error) {

	queueURL := GetEnv("QUEUE_URL", nats.DefaultURL)

	// Connect to a server
	nc, err := nats.Connect(queueURL,
		nats.ReconnectBufSize(50*1024*1024), nats.ReconnectWait(1*time.Second),
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
		return nil,  err
	}

	stanQueue, err := NewQue(log, nc)
	if err != nil {
		return nil,  err
	}

	return stanQueue.connection, nil
}


// StanQueue implements the "ICheckable" interface,
// this is our gateway to health checking
type StanQueue struct {
	connection stan.Conn
	log *logrus.Entry
	disconnected bool
}

func(q *StanQueue) ConnectionLostListener(conn stan.Conn, reason error) {
	q.log.Errorf("Connection lost, reason: %v", reason)
	q.disconnected = true
}


func NewQue(log *logrus.Entry, conn *nats.Conn) (*StanQueue, error) {

	clusterID := GetEnv("QUEUE_CLUSTER_ID", "smpp_cluster")
	clientID := GetEnv("QUEUE_CLIENT_ID", fmt.Sprintf("smpp_router", ))

	stanQueue := StanQueue{
		log: log,
		disconnected: false,
	}

	stanConnection, err := stan.Connect(clusterID, clientID, stan.NatsConn(conn),
		stan.Pings(10, 5),
		stan.SetConnectionLostHandler(stanQueue.ConnectionLostListener))

	if err != nil {
		return nil, err
	}

	stanQueue.connection = stanConnection
	return &stanQueue, nil
}

