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

type Route struct {
	id string
	synchronous bool
	subRoutes []SubRoute

}

func (r *Route) ID() string  {
	return r.id
}

// IsActive
//only returns true immediately after initialization if it is working asynchronousl otherwise
//it activates when one of its subroutes becomes active
func (r *Route) IsActive() bool  {

	if r.hasQueue(){
		return true
	}else{

		for _, subRoute := range r.subRoutes{
			subRoute.IsActive()
		}

		return false
	}

}


func (r *Route) hasQueue() bool{
	return !r.synchronous
}

func (r *Route) init(log *logrus.Entry)  {
	for _, subRoute := range r.subRoutes {
		log.Infof(" Initiating sub route : %s ", r.ID())
		go subRoute.Init()
	}
}

func (r *Route) Submit( )  {




}

type SubRoute interface {
	ID() string
	Init()
	Status() string
	IsActive() bool
}

type Server struct {
	availableRoutes map[string]*Route
}

func (s *Server) Stop() {

}

func (s *Server) NewRoute(queue stan.Conn, log *logrus.Entry, route string) error {

	if s.availableRoutes[route] != nil {
		return nil
	}

	//Allow routes to bind to multiple servers at once
	var subRouteSlice []SubRoute
	hostAddresses := GetSetting(fmt.Sprintf("%s.addresses", route), "")
	hostAddressSlice := strings.Split(hostAddresses, ",")

	for _, hostAddress := range hostAddressSlice {

		smppRoute := SmppRoute{
			id:             route,
			queue:          queue,
			status:         "Create",
			log:            log.WithField("SubRoute ID", route),
			settingAddress: hostAddress,
		}

		subRouteSlice = append(subRouteSlice, &smppRoute)

	}

	s.availableRoutes[route] = &Route{route, subRouteSlice}

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

func NewServer(queue stan.Conn, log *logrus.Entry) (*Server, error) {

	viper.SetConfigName("routes")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil { // Handle errors reading the config file
		return nil, err
	}

	routes := viper.GetStringSlice("active_routes")

	smsServer := Server{
		availableRoutes: make(map[string]*Route, len(routes)),
	}

	for _, route := range routes {
		err := smsServer.NewRoute(queue, log, route)
		if err != nil {
			return nil, err
		}
	}

	for _, route := range smsServer.availableRoutes {
		route.init(log)
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
