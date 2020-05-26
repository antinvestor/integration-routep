package sms

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/stan.go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"math/rand"
	"strings"
	"time"
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
	id        string
	queue     stan.Conn
	subRoutes []SubRoute
}

func (r *Route) ID() string {
	return r.id
}

// IsActive
//only returns true immediately after initialization if it is working asynchronousl otherwise
//it activates when one of its subroutes becomes active
func (r *Route) IsActive() bool {

	if r.CanQueue() {
		return true
	} else {

		for _, subRoute := range r.subRoutes {
			if subRoute.IsActive() {
				return true
			}
		}

		return false
	}

}

func (r *Route) CanQueue() bool {
	if len(r.subRoutes) > 0 {
		return r.subRoutes[0].CanQueue()
	}
	return false
}

func (r *Route) SendMOMessage(message *SMS) (*ACK, error) {

	if r.CanQueue() {

		binMessage, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}

		err = r.queue.Publish(GetSmsSendQueueName(message.RouteID), binMessage)
		if err != nil {
			return nil, err
		}

		return nil, nil
	}

	if len(r.subRoutes) == 0 || !r.IsActive() {
		return nil, errors.New("can't route message without external network connection")
	}

	if len(r.subRoutes) == 1 {
		return r.subRoutes[0].SendMOMessage(message)
	}

	for i := 0; i < len(r.subRoutes)*2; i++ {
		s := rand.NewSource(time.Now().Unix())
		randomSeed := rand.New(s) // initialize local pseudorandom generator
		randomIndex := randomSeed.Intn(len(r.subRoutes))

		if r.subRoutes[randomIndex].IsActive() {
			return r.subRoutes[randomIndex].SendMOMessage(message)
		}
	}

	return nil, errors.New("can't route message as no active routes were determined")

}

func (r *Route) init(log *logrus.Entry) {
	for _, subRoute := range r.subRoutes {
		log.Infof(" Initiating sub route : %s ", r.ID())
		go subRoute.Init()
	}
}

func (r *Route) Stop() {
	for _, subRoute := range r.subRoutes {
		subRoute.Stop()
	}
}

type SubRoute interface {
	ID() string
	Init()
	Status() string
	IsActive() bool
	CanQueue() bool
	SendMOMessage(message *SMS) (*ACK, error)
	Stop()
}

type Server struct {
	availableRoutes map[string]*Route
}

func (s *Server)IsActive() bool {
	for _, route := range s.availableRoutes{
		if route.IsActive(){
			return true
		}
	}

	return false
}

func (s *Server) GetRoute(id string) *Route {
	if route, ok := s.availableRoutes[id]; ok {
		return route
	} else {
		return nil
	}
}

func (s *Server) Stop() {
	for _, route := range s.availableRoutes {
		route.Stop()
	}
}

func (s *Server) newRoute(queue stan.Conn, log *logrus.Entry, routeID string) error {

	if s.availableRoutes[routeID] != nil {
		return nil
	}

	//Allow routes to bind to multiple servers at once
	var subRouteSlice []SubRoute
	hostAddresses := GetSetting(fmt.Sprintf("%s.addresses", routeID), "")
	hostAddressSlice := strings.Split(hostAddresses, ",")

	for _, hostAddress := range hostAddressSlice {

		smppRoute := SmppRoute{
			id:             routeID,
			queue:          queue,
			status:         "Create",
			log:            log.WithField("SubRoute ID", routeID),
			settingAddress: hostAddress,
			exitSignal:     make(chan int, 1),
		}

		subRouteSlice = append(subRouteSlice, &smppRoute)

	}

	s.availableRoutes[routeID] = &Route{routeID, queue, subRouteSlice}

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
	if err != nil {             // Handle errors reading the config file
		return nil, err
	}

	routes := viper.GetStringSlice("active_routes")

	smsServer := Server{
		availableRoutes: make(map[string]*Route, len(routes)),
	}

	for _, route := range routes {
		err := smsServer.newRoute(queue, log, route)
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
