package sms

import (
	"bytes"
	"fmt"
	"github.com/nats-io/stan.go"
	"io/ioutil"
	"net/http"
	"time"
)

func subscribeForAckEvents(r *SmppRoute) error  {

	aw, _ := time.ParseDuration("60s")

	if r.sendAckSubscription != nil {
		err := r.sendAckSubscription.Unsubscribe()
		if err != nil{
			r.log.WithError(err).Warn("failed to unsubscribe for send ack events")
		}
		r.sendAckSubscription = nil

	}

	// Async Subscriber to acknowledge messages queued on smsc
	subs, err := r.queue.QueueSubscribe(GetSmsSendAckQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {

			r.log.Infof("Sent out ACK with ID : %s on url : %s", string(m.Data), r.settingSmsSendAckUrl)

			resp, err := http.Post(r.settingSmsSendAckUrl, "application/json", bytes.NewBuffer(m.Data))
			if err != nil{
				r.log.WithError(err).Warnf("error occurred on attempting ack webhook")
			}

			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNonAuthoritativeInfo {
				m.Ack()
			}else{
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil{
					r.log.WithError(err).Infof(" reschedule ACK message with status : %d ", resp.StatusCode)
				}else {
					r.log.Infof(" reschedule ACK message with status : %d and content : %v", resp.StatusCode, string(body))
				}
			}

		}()


	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_send_ack")),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(50))

	if err != nil{
		return err
	}

	r.sendAckSubscription = subs

	return nil
}

func subscribeForDLREvents(r *SmppRoute) error  {


	aw, _ := time.ParseDuration("60s")

	if r.receiveDLRSubscription != nil {
		err := r.receiveDLRSubscription.Unsubscribe()
		if err != nil{
			r.log.WithError(err).Warn("failed to unsubscribe for dlr events")
		}
		r.receiveDLRSubscription = nil
	}

	// Async Subscriber to acknowledge messages queued on smsc
	subs, err := r.queue.QueueSubscribe(GetSmsSendDLRQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {

			r.log.Infof("Sending out DLR : %s on url : %s", string(m.Data), r.settingSmsSendDLRUrl)

			resp, err := http.Post(r.settingSmsSendDLRUrl, "application/json", bytes.NewBuffer(m.Data))
			if err != nil{
				r.log.WithError(err).Warnf("error occurred on attempting dlr webhook")
			}

			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNonAuthoritativeInfo {
				m.Ack()
			}else{
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil{
					r.log.WithError(err).Infof(" reschedule DLR message with status : %d ", resp.StatusCode)
				}else {
					r.log.Infof(" reschedule DLR message with status : %d and content : %v", resp.StatusCode, string(body))
				}
			}

		}()

	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_receive_dlr")),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(50))

	if err != nil{
		return err
	}

	r.receiveDLRSubscription = subs

	return nil
}

func subscribeForMTEvents(r *SmppRoute) error  {

	aw, _ := time.ParseDuration("60s")

	if r.receiveMessageSubscription != nil {
		err := r.receiveMessageSubscription.Unsubscribe()
		if err != nil{
			r.log.WithError(err).Warn("failed to unsubscribe for mt events")
		}
		r.receiveMessageSubscription = nil

	}

	// Async Subscriber to acknowledge messages queued on smsc
	subs, err := r.queue.QueueSubscribe(GetSmsReceiveQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {

			r.log.Infof("Sending out MT : %s on url : %s", string(m.Data), r.settingSmsReceiveUrl)

			resp, err := http.Post(r.settingSmsReceiveUrl, "application/json", bytes.NewBuffer(m.Data))
			if err != nil{
				r.log.WithError(err).Warnf("error occurred on attempting MT webhook")
			}

			if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNonAuthoritativeInfo {
				m.Ack()
			}else{
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil{
					r.log.WithError(err).Infof(" reschedule MT message with status : %d ", resp.StatusCode)
				}else {
					r.log.Infof(" reschedule MT message with status : %d and content : %s", resp.StatusCode, string(body))
				}
			}

		}()

	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_receive_mt")),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(50))

	if err != nil{
		return err
	}

	r.receiveMessageSubscription = subs
	return nil
}
