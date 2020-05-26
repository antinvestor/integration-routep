package sms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nats-io/stan.go"
	"io/ioutil"
	"net/http"
	"time"
)

func (r *SmppRoute) processAckEvent(messageAck *ACK, queue bool) error {

	message, err := json.Marshal(messageAck)
	if err != nil {
		return err
	}

	if queue {
		return r.queue.Publish(GetSmsSendAckQueueName(r.ID()), message)
	}

	r.log.Infof("Sending out ACK with ID : %s on url : %s", string(message), r.settingSmsSendAckUrl)

	resp, err := http.Post(r.settingSmsSendAckUrl, "application/json", bytes.NewBuffer(message))
	if err != nil {
		r.log.WithError(err).Warnf("error occurred on attempting invoke webhook")
		return err
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNonAuthoritativeInfo {
		return nil
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			r.log.WithError(err).Infof(" reschedule ACK message with status : %d ", resp.StatusCode)
		} else {
			r.log.Infof(" reschedule ACK message with status : %d and content : %v", resp.StatusCode, string(body))
		}
	}

	return nil
}

func subscribeForAckEvents(r *SmppRoute) error {

	aw, _ := time.ParseDuration("60s")

	if r.sendAckSubscription != nil {
		err := r.sendAckSubscription.Unsubscribe()
		if err != nil {
			r.log.WithError(err).Warn("failed to unsubscribe for send ack events")
		}
		r.sendAckSubscription = nil

	}

	// Async Subscriber to acknowledge messages queued on smsc
	subs, err := r.queue.QueueSubscribe(GetSmsSendAckQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {

			messageAck := ACK{}
			err := json.Unmarshal(m.Data, &messageAck)
			if err != nil {
				r.log.WithError(err).Errorf("error decoding message : [ %v  ] hence dropping it", m.Data)
				err = m.Ack()
				if err != nil{
					r.log.WithError(err).Error("error acknowledging message")
				}
			}

			err = r.processAckEvent(messageAck, false)
			if err != nil {
				r.log.WithError(err).Errorf("error occurred when posting ack to webhook")
			} else {

				err = m.Ack()
				if err != nil {
					r.log.WithError(err).Warnf("error occurred on attempting acknowledge successful webhook")
				}
			}

		}()

	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_send_ack", r.ID())),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(50))

	if err != nil {
		return err
	}

	r.sendAckSubscription = subs

	return nil
}

