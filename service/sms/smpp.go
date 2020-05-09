package sms

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/nats-io/stan.go"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
	"time"
)

type SmppRoute struct {
	id     string
	status string
	log    *logrus.Entry
	queue  stan.Conn

	txConn <-chan smpp.ConnStatus

	trx *smpp.Transceiver
	tr  *smpp.Transmitter

	sendSubscription           stan.Subscription
	sendAckSubscription        stan.Subscription
	receiveMessageSubscription stan.Subscription
	receiveDLRSubscription     stan.Subscription

	settingAddress        string
	settingUser           string
	settingPassword       string
	settingBindType       string
	settingSystemType     string
	settingSourceTon      uint8
	settingSourceNpi      uint8
	settingDestinationTon uint8
	settingDestinationNpi uint8
	settingDLRLevel       uint8

	settingSmsSendAckUrl string
	settingSmsSendDLRUrl string
	settingSmsReceiveUrl string

	settingDisableTLVTrackingID bool
}

func (r *SmppRoute) ID() string {
	return r.id
}

func startSendingMessages(route *SmppRoute) error {

	err := subscribeForMOEvents(route)

	if err != nil {
		return err
	}

	route.status = "Connected"

	return nil

}

// Handler handles DeliverSM coming from a Transceiver SMPP connection.
// It broadcasts received delivery receipt to all registered peers.
func (r *SmppRoute) SmscHandler(p pdu.Body) {

	r.log.Infof("route %v received a message : %v ", r.ID(), p.Header())

	switch p.Header().ID {
	case pdu.DeliverSMID:
		dlr := r.parseForDlr(p.Fields())
		dlr.RouteID = r.ID()
		r.handleReceivedDlr(dlr)
		break
	case pdu.DataSMID:
		f := p.Fields()

		message := &SMS{
			From:    f[pdufield.SourceAddr].String(),
			To:      f[pdufield.DestinationAddr].String(),
			Data:    f[pdufield.ShortMessage].String(),
			SmscID:  f[pdufield.MessageID].String(),
			RouteID: r.ID(),
		}
		r.handleReceivedMessage(message)
	}
}

func (r *SmppRoute) handleReceivedMessage(message *SMS) {

	respMessage, err := json.Marshal(message)
	if err != nil {
		r.log.Errorf("Could not encode sms : %v hence dropping it", message)
		return
	}
	err = r.queue.Publish(GetSmsReceiveQueueName(message.RouteID), respMessage)
	if err != nil {
		r.log.Errorf("Messages %v was just lost", message)
	}

}

func (r *SmppRoute) handleReceivedDlr(dlr *DLR) {

	dlrMessage, err := json.Marshal(dlr)
	if err != nil {
		r.log.Errorf("Could not encode sms : %v hence dropping it", dlr)
		return
	}

	err = r.queue.Publish(GetSmsSendDLRQueueName(dlr.RouteID), dlrMessage)
	if err != nil {
		r.log.Errorf("DLR %v was just lost", dlr)
	}
}

func (r *SmppRoute) parseForDlr(fields pdufield.Map) *DLR {

	dlrText := fields[pdufield.ShortMessage].String()

	dlr := &DLR{
		From:      fields[pdufield.SourceAddr].String(),
		To:        fields[pdufield.DestinationAddr].String(),
		SmscID:    fields[pdufield.SMDefaultMsgID].String(),
		SmscExtra: dlrText,
	}

	regex := regexp.MustCompile(`id:(?P<id>.*) sub:(?P<sub>.*) dlvrd:(?P<dlvrd>.*) submit date:(?P<submitdate>.*) done date:(?P<donedate>.*) stat:(?P<stat>.*) err:(?P<err>.*) text:(?P<text>.*)`)
	match := regex.FindStringSubmatch(dlrText)

	for i, name := range match {
		switch regex.SubexpNames()[i] {
		case "id":
			dlr.SmscID = name
		case "sub":
			dlr.Sub = name
		case "dlvrd":
			dlr.Dlvrd = name
		case "submitdate":
			dlr.SubmittedDate = name
		case "donedate":
			dlr.DoneDate = name
		case "stat":
			dlr.SmscStatus = name
		case "err":
			dlr.Err = name
		case "text":
			dlr.Err = name

		}
	}


	return dlr
}

func (r *SmppRoute) Init() {

	r.log.Infof("Starting up smpp routes %v", r.ID())
	r.status = "Init"
	r.getSettings()

	for {
		err := r.Run()
		if err != nil {
			r.log.WithError(err).Warnf("Route stopping error occurred, app will reattempt connection in 2 minutes")
		}

		<-time.After(5 * time.Minute)

	}

}

func (r *SmppRoute) Run() error {

	err := subscribeForAckEvents(r)
	if err != nil {
		return err
	}
	err = subscribeForDLREvents(r)
	if err != nil {
		return err
	}
	err = subscribeForMTEvents(r)
	if err != nil {
		return err
	}
	return startSmppConnection(r)

}

func (r *SmppRoute) Status() string {
	return r.status
}

func (r *SmppRoute) getSettings() {

	// Obtain all the configs required to make an smpp connection
	// These should be prefixed with route name from configs

	r.settingUser = GetSetting(fmt.Sprintf("%s.user", r.ID()), "")
	r.settingPassword = GetSetting(fmt.Sprintf("%s.password", r.ID()), "")
	r.settingBindType = GetSetting(fmt.Sprintf("%s.bindType", r.ID()), "transceiver")
	r.settingSystemType = GetSetting(fmt.Sprintf("%s.systemType", r.ID()), "")

	srcNpi := GetSetting(fmt.Sprintf("%s.source_npi", r.ID()), "0")

	sett, err := strconv.ParseUint(srcNpi, 10, 8)
	if err != nil {
		sett = 0
	}
	r.settingSourceNpi = uint8(sett)

	srcTon := GetSetting(fmt.Sprintf("%s.source_ton", r.ID()), "5")

	sett, err = strconv.ParseUint(srcTon, 10, 8)
	if err != nil {
		sett = 5
	}
	r.settingSourceTon = uint8(sett)

	destNpi := GetSetting(fmt.Sprintf("%s.destination_npi", r.ID()), "1")

	sett, err = strconv.ParseUint(destNpi, 10, 8)
	if err != nil {
		sett = 1
	}
	r.settingDestinationNpi = uint8(sett)

	destTon := GetSetting(fmt.Sprintf("%s.destination_ton", r.ID()), "1")

	sett, err = strconv.ParseUint(destTon, 10, 8)
	if err != nil {
		sett = 1
	}
	r.settingDestinationTon = uint8(sett)

	dlrLvl := GetSetting(fmt.Sprintf("%s.dlr_level", r.ID()), "3")

	sett, err = strconv.ParseUint(dlrLvl, 10, 8)
	if err != nil {
		sett = 8
	}
	r.settingDLRLevel = uint8(sett)

	r.settingSmsReceiveUrl = GetSetting(fmt.Sprintf("%s.sms_receive_url", r.ID()), "")
	r.settingSmsSendDLRUrl = GetSetting(fmt.Sprintf("%s.sms_send_dlr_url", r.ID()), "")
	r.settingSmsSendAckUrl = GetSetting(fmt.Sprintf("%s.sms_send_ack_url", r.ID()), "")

	disableTlv := GetSetting(fmt.Sprintf("%s.disable_tlv_options", r.ID()), "False")
	settTlv, err := strconv.ParseBool(disableTlv)
	if err != nil {
		settTlv = false
	}
	r.settingDisableTLVTrackingID = settTlv

}

func startSmppConnection(r *SmppRoute) error {

	var connStat <-chan smpp.ConnStatus

	switch r.settingBindType {

	case "transmitter":

		r.tr = &smpp.Transmitter{
			Addr:       r.settingAddress,
			User:       r.settingUser,
			Passwd:     r.settingPassword,
			SystemType: r.settingSystemType,
		}
		connStat = r.tr.Bind()
		break

	default:

		r.trx = &smpp.Transceiver{
			Addr:       r.settingAddress,
			User:       r.settingUser,
			Passwd:     r.settingPassword,
			SystemType: r.settingSystemType,
		}
		connStat = r.trx.Bind()

		//Register inbound messages handler
		r.trx.Handler = r.SmscHandler

		break
	}

	for c := range connStat {

		if err := c.Error(); err != nil {
			r.log.Warnf("Smsc connection has error : %v", err)
			continue
		}

		r.log.Infof("Smsc updated status : %v", c.Status())

		switch c.Status() {
		case smpp.Connected:
			err := startSendingMessages(r)
			if err != nil {
				r.log.Warnf("Error happened when initiating messages sending %v ", err)
			}
			break
		case smpp.Disconnected:
			err := unSubscribeForMOEvents(r)
			if err != nil {
				r.log.Warnf("Error on stoping messages sending %v ", err)
			}
			break
		case smpp.ConnectionFailed:
			err := unSubscribeForMOEvents(r)
			if err != nil {
				r.log.Warnf("Error initiating messages sending %v ", err)
			}
			break
		case smpp.BindFailed:
			err := unSubscribeForMOEvents(r)
			if err != nil {
				r.log.Warnf("Error initiating messages sending %v ", err)
			}
			break

		}

	}

	r.status = "Disconnected"
	return errors.New("SMPP connection could not be setup")

}
