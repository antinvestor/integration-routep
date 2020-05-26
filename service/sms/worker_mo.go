package sms

import (
	"encoding/json"
	"fmt"
	"github.com/fiorix/go-smpp/smpp"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutext"
	"github.com/fiorix/go-smpp/smpp/pdu/pdutlv"
	"github.com/nats-io/stan.go"
	"time"
)

func (r *SmppRoute) processMOMessage(message *SMS) (ACK, error) {

	var dlrLvl pdufield.DeliverySetting
	switch r.settingDLRLevel{
	case 2:
		dlrLvl = pdufield.FinalDeliveryReceipt
	case 3:
		dlrLvl = pdufield.FailureDeliveryReceipt
	default:
		dlrLvl = pdufield.NoDeliveryReceipt
	}


	sms := smpp.ShortMessage{
		Src:           message.From,
		Dst:           message.To,
		Text:          pdutext.Raw(message.Data),
		SourceAddrNPI: r.settingSourceNpi,
		SourceAddrTON: r.settingSourceTon,
		DestAddrNPI:   r.settingDestinationNpi,
		DestAddrTON:   r.settingDestinationTon,
		Register:      dlrLvl,

	}
	if ! r.settingDisableTLVTrackingID {
		sms.TLVFields = pdutlv.Fields{
			pdutlv.TagReceiptedMessageID: pdutlv.CString(message.MessageID),
		}
	}
	ack := ACK{
		From: message.From,
		To: message.To,
		RouteID: message.RouteID,
		MessageID: message.MessageID,
	}

	var sm *smpp.ShortMessage
	var err error

	if r.trx != nil {
		sm, err = r.trx.Submit(&sms)
	} else {
		sm, err = r.tr.Submit(&sms)
	}

	if err != nil{
		return ack, err
	}

	if sm != nil {
		ack.SmscID = sm.RespID()
		ack.SmscStatus = "Submitted"
	}
	return ack, nil

}

func unSubscribeForMOEvents(route *SmppRoute) error {

	if route.sendSubscription != nil {
		err := route.sendSubscription.Unsubscribe()
		route.sendSubscription = nil
		route.status = "Disconnected"
		return err
	}
	return nil
}

func subscribeForMOEvents(r *SmppRoute) error {

	aw, _ := time.ParseDuration("60s")

	if r.sendSubscription != nil {
		if r.status == "Connected" {
			return nil
		} else {
			return unSubscribeForMOEvents(r)
		}
	}

	// Async Subscriber to send queued messages
	subs, err := r.queue.QueueSubscribe(GetSmsSendQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {
			message := &SMS{}
			err := json.Unmarshal(m.Data, message)
			if err != nil {
				r.log.WithError(err).Errorf("error decoding message : [ %v  ] hence dropping it", m.Data)
				err = m.Ack()
				if err != nil{
					r.log.WithError(err).Error("error acknowledging message")
				}
			}

			messageAck, err := r.processMOMessage(message)
			if err != nil {
				r.log.Infof("rescheduling message with id : %s for later because : %v", message.MessageID, err)
				return
			}

			err = r.processAckEvent(messageAck, r.CanQueue())
			if err != nil {
				r.log.WithError(err).Infof("failed to process ack %s hence dropping it because : %v", messageAck.MessageID, err)
			}
			err = m.Ack()
			if err != nil {
				r.log.WithError(err).Warn("error occurred on attempting acknowledge successful MO")
			}

		}()
	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_send_sub", r.ID())),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(int(r.settingSmsCDeliveryRate)))

	if err != nil {
		return err
	}

	r.sendSubscription = subs

	return nil
}
