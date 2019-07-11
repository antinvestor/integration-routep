package sms

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/nats-io/stan.go"
	"github.com/spf13/viper"
)

type DLR struct {
	From          string
	To            string
	RouteID       string
	SmscID        string
	SmscStatus    string
	Sub           string
	Dlvrd         string
	SubmittedDate string
	DoneDate      string
	Text          string
	Err           string
	SmscExtra     string
}

type ACK struct {
	From       string
	To         string
	MessageID  string
	RouteID    string
	SmscID     string
	SmscStatus string
}

type SMS struct {
	From       string
	To         string
	Data       string
	MessageID  string
	RouteID    string
	SmscID     string
	SmscStatus string
	SmscExtra  string
}

type Route interface {
	ID() string
	Init()
	Status() string
}

type Server struct {
	availableRoutes map[string]Route
	activeRoutes    []string
}

func (s *Server) Stop() {

}

func (s *Server) NewRoute(queue stan.Conn, log *logrus.Entry, route string) error {

	if s.availableRoutes[route] != nil {
		return nil
	}

	log = log.WithField("Route ID", route)

	smpp := SmppRoute{id: route, log: log, queue: queue, status: "Create"}
	s.availableRoutes[route] = &smpp

	return nil
}

func GetSmsSendQueueName(routeID string) string {
	return fmt.Sprintf("%s.message.send", routeID)
}

func GetSmsSendAckQueueName(routeID string) string {
	return fmt.Sprintf("%s.message.ack", routeID)
}
func GetSmsReceiveQueueName(routeID string) string {
	return fmt.Sprintf("%s.message.receive", routeID)
}
func GetSmsSendDLRQueueName(routeID string) string {
	return fmt.Sprintf("%s.message.dlr", routeID)
}

func GetQueueGroup(routeID string) string {
	return fmt.Sprintf("smpp-%s", routeID)
}

func Init(queue stan.Conn, log *logrus.Entry, configFile string) (*Server, error) {

	viper.SetConfigName("routes")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil { // Handle errors reading the config file
		return nil, err
	}

	routes := viper.GetStringSlice("active_routes")

	smsServer := Server{
		availableRoutes: make(map[string]Route, len(routes)),
	}

	for _, route := range routes {
		err := smsServer.NewRoute(queue, log, route)
		if err != nil {
			return nil, err
		}
	}

	for id, route := range smsServer.availableRoutes {
		go func() {
			log.Infof(" Initiating smpp route : %s ", id)
			route.Init()
		}()
	}

	return &smsServer, nil
}

func GetSetting(key, fallback string) string {

	value := viper.GetString(key)

	if value != "" {
		return value
	}
	return fallback
}
