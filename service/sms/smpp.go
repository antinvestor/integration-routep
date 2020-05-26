package sms

import (
	"fmt"
	"github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/nats-io/stan.go"
	"github.com/sirupsen/logrus"
	"strconv"
	"time"
)

type SmppRoute struct {
	id         string
	active     bool
	log        *logrus.Entry
	queue      stan.Conn
	exitSignal chan int
	txConn     <-chan smpp.ConnStatus

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
	settingSmsCDeliveryRate     uint64

	settingOperatesSynchronously bool
}

func (r *SmppRoute) Stop() {
	r.exitSignal <- 1
}

func (r *SmppRoute) ID() string {
	return r.id
}

func (r *SmppRoute) IsActive() bool {
	return r.active
}

func (r *SmppRoute) CanQueue() bool {
	return !r.settingOperatesSynchronously
}

// Handler handles DeliverSM coming from a Transceiver SMPP connection.
// It broadcasts received delivery receipt to all registered peers.
func (r *SmppRoute) SmscHandler(p pdu.Body) {

	r.log.Infof("route %v received a message : %v ", r.ID(), p.Header())

	switch p.Header().ID {
	case pdu.DeliverSMID:
		dlr := r.parseForDlr(p.Fields())
		dlr.RouteID = r.ID()
		err := r.processDLRMessage(dlr, r.CanQueue())
		if err != nil {
			r.log.WithError(err).Errorf("error occurred post processing dlr")
		}
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
		err := r.processMTMessage(message, r.CanQueue())
		if err != nil {
			r.log.WithError(err).Errorf("error occurred post processing inbound message")
		}
	}
}

func (r *SmppRoute) Init() {

	r.log.Infof("Starting up smpp routes %v", r.ID())
	r.getSettings()

	for {
		err := r.Run()
		if err != nil {
			r.log.WithError(err).Warnf("SubRoute stopping error occurred, app will reattempt connection in 2 minutes")
			<-time.After(5 * time.Minute)
		}else{
			r.log.Info("Exiting route gracefully")
		}



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
	return r.startSmppConnection()

}


func (r *SmppRoute) getSettings() {

	// Obtain all the configs required to make an smpp connection
	// These should be prefixed with route name from configs

	r.settingUser = GetSetting(fmt.Sprintf("%s.user", r.ID()), "")
	r.log.Infof("Route [%v] setting :  settingUser = %s", r.ID(), r.settingUser)

	r.settingPassword = GetSetting(fmt.Sprintf("%s.password", r.ID()), "")

	r.settingBindType = GetSetting(fmt.Sprintf("%s.bindType", r.ID()), "transceiver")
	r.log.Infof("Route [%v] setting :  settingBindType = %s", r.ID(), r.settingBindType)

	r.settingSystemType = GetSetting(fmt.Sprintf("%s.systemType", r.ID()), "")
	r.log.Infof("Route [%v] setting :  settingSystemType = %s", r.ID(), r.settingSystemType)

	srcNpi := GetSetting(fmt.Sprintf("%s.source_npi", r.ID()), "0")

	sett, err := strconv.ParseUint(srcNpi, 10, 8)
	if err != nil {
		sett = 0
	}
	r.settingSourceNpi = uint8(sett)
	r.log.Infof("Route [%v] setting :  settingSourceNpi = %d", r.ID(), r.settingSourceNpi)

	srcTon := GetSetting(fmt.Sprintf("%s.source_ton", r.ID()), "5")

	sett, err = strconv.ParseUint(srcTon, 10, 8)
	if err != nil {
		sett = 5
	}
	r.settingSourceTon = uint8(sett)
	r.log.Infof("Route [%v] setting :  settingSourceTon = %d", r.ID(), r.settingSourceTon)

	destNpi := GetSetting(fmt.Sprintf("%s.destination_npi", r.ID()), "1")

	sett, err = strconv.ParseUint(destNpi, 10, 8)
	if err != nil {
		sett = 1
	}
	r.settingDestinationNpi = uint8(sett)
	r.log.Infof("Route [%v] setting :  settingDestinationNpi = %d", r.ID(), r.settingDestinationNpi)

	destTon := GetSetting(fmt.Sprintf("%s.destination_ton", r.ID()), "1")

	sett, err = strconv.ParseUint(destTon, 10, 8)
	if err != nil {
		sett = 1
	}
	r.settingDestinationTon = uint8(sett)
	r.log.Infof("Route [%v] setting :  settingDestinationTon = %d", r.ID(), r.settingDestinationTon)

	dlrLvl := GetSetting(fmt.Sprintf("%s.dlr_level", r.ID()), "3")

	sett, err = strconv.ParseUint(dlrLvl, 10, 8)
	if err != nil {
		sett = 8
	}
	r.settingDLRLevel = uint8(sett)
	r.log.Infof("Route [%v] setting :  settingDLRLevel = %d", r.ID(), r.settingDLRLevel)

	r.settingSmsReceiveUrl = GetSetting(fmt.Sprintf("%s.sms_receive_url", r.ID()), "")
	r.log.Infof("Route [%v] setting :  settingSmsReceiveUrl = %s", r.ID(), r.settingSmsReceiveUrl)

	r.settingSmsSendDLRUrl = GetSetting(fmt.Sprintf("%s.sms_send_dlr_url", r.ID()), "")
	r.log.Infof("Route [%v] setting :  settingSmsSendDLRUrl = %s", r.ID(), r.settingSmsSendDLRUrl)

	r.settingSmsSendAckUrl = GetSetting(fmt.Sprintf("%s.sms_send_ack_url", r.ID()), "")
	r.log.Infof("Route [%v] setting :  settingSmsSendAckUrl = %v", r.ID(), r.settingSmsSendAckUrl)

	disableTlv := GetSetting(fmt.Sprintf("%s.disable_tlv_options", r.ID()), "False")
	settTlv, err := strconv.ParseBool(disableTlv)
	if err != nil {
		settTlv = false
	}
	r.settingDisableTLVTrackingID = settTlv
	r.log.Infof("Route [%v] setting :  settingDisableTLVTrackingID = %v", r.ID(), r.settingDisableTLVTrackingID)

	smscDeliveryRate := GetSetting(fmt.Sprintf("%s.smsc_delivery_rate", r.ID()), "50")
	r.settingSmsCDeliveryRate, err = strconv.ParseUint(smscDeliveryRate, 10, 8)
	if err != nil {
		r.settingSmsCDeliveryRate = 50
	}
	r.log.Infof("Route [%v] setting :  settingSmsCDeliveryRate = %d", r.ID(), r.settingSmsCDeliveryRate)

	operatesSynchronously := GetSetting(fmt.Sprintf("%s.operates_synchronously", r.ID()), "True")
	settOperatesSynchronously, err := strconv.ParseBool(operatesSynchronously)
	if err != nil {
		settOperatesSynchronously = true
	}
	r.settingOperatesSynchronously = settOperatesSynchronously
	r.log.Infof("Route [%v] setting :  settingOperatesSynchronously = %v", r.ID(), r.settingOperatesSynchronously)

}

func (r *SmppRoute) startSmppConnection() error {

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

	for {
		select {

		case c := <-connStat:

			if err := c.Error(); err != nil {
				r.log.Warnf("Smsc connection has error : %v", err)
				continue
			}

			r.log.Infof("Smsc updated status : %v", c.Status())

			switch c.Status() {
			case smpp.Connected:

				err := subscribeForMOEvents(r)
				if err != nil {
					r.log.Warnf("Error happened when initiating messages sending %v ", err)
				}
				r.active = true
				break
			case smpp.Disconnected:
				err := unSubscribeForMOEvents(r)
				if err != nil {
					r.log.Warnf("Error on stoping messages sending %v ", err)
				}
				r.active = false
				break
			case smpp.ConnectionFailed:
				err := unSubscribeForMOEvents(r)
				if err != nil {
					r.log.Warnf("Error initiating messages sending %v ", err)
				}
				r.active = false
				break
			case smpp.BindFailed:
				err := unSubscribeForMOEvents(r)
				if err != nil {
					r.log.Warnf("Error initiating messages sending %v ", err)
				}
				r.active = false
				break

			}


		case <- r.exitSignal:
			r.log.Info("Received an exit signal ")
			return nil
		}

	}

}
