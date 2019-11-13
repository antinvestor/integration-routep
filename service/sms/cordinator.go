package sms

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/nats-io/stan.go"
	"github.com/spf13/viper"
	"strings"
)

type DLR struct {
	From       string `json:"from"`
	To         string `json:"to"`
	RouteID    string `json:"route_id"`
	SmscID     string `json:"smsc_id"`
	SmscStatus string `json:"smsc_status"`

	Sub           string `json:"sub,omitempty"`
	Dlvrd         string `json:"dlvrd,omitempty"`
	SubmittedDate string `json:"submitted_date,omitempty"`
	DoneDate      string `json:"done_date,omitempty"`
	Text          string `json:"text,omitempty"`
	Err           string `json:"err,omitempty"`
	SmscExtra     string `json:"smsc_extra"`
}

type ACK struct {
	From       string `json:"from"`
	To         string `json:"to"`
	MessageID  string `json:"message_id"`
	RouteID    string `json:"route_id"`
	SmscID     string `json:"smsc_id"`
	SmscStatus string `json:"smsc_status"`
}

type SMS struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Data       string `json:"data"`
	MessageID  string `json:"message_id"`
	RouteID    string `json:"route_id,omitempty"`
	SmscID     string `json:"smsc_id,omitempty"`
	SmscStatus string `json:"smsc_status,omitempty"`
	SmscExtra  string `json:"smsc_extra,omitempty"`
}

type Route interface {
	ID() string
	Init()
	Status() string
}

type Server struct {
	availableRoutes map[string][]Route
	activeRoutes    []string
}

func (s *Server) Stop() {

}

func (s *Server) NewRoute(queue stan.Conn, log *logrus.Entry, route string) error {

	if s.availableRoutes[route] != nil {
		return nil
	}

	//Allow the smpp routes to bind to multiple servers at once
	var smppRouteSlice []Route
	hostAddresses := GetSetting(fmt.Sprintf("%s.addresses", route), "")
	hostAddressSlice := strings.Split(hostAddresses, ",")

	for _, hostAddress := range hostAddressSlice {

		smppRoute := SmppRoute{
			id:             route,
			queue:          queue,
			status:         "Create",
			log:            log.WithField("Route ID", route),
			settingAddress: hostAddress,
		}

		smppRouteSlice = append(smppRouteSlice, &smppRoute)

	}

	s.availableRoutes[route] = smppRouteSlice

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
		availableRoutes: make(map[string][]Route, len(routes)),
	}

	for _, route := range routes {
		err := smsServer.NewRoute(queue, log, route)
		if err != nil {
			return nil, err
		}
	}

	for id, routesSlice := range smsServer.availableRoutes {

		for _, route := range routesSlice {

			go func() {
				log.Infof(" Initiating smpp route : %s ", id)
				route.Init()
			}()
		}
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
