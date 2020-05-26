package sms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nats-io/stan.go"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"time"
)

func (r *SmppRoute) processMTMessage(message *SMS, queue bool) error {

	respMessage, err := json.Marshal(message)
	if err != nil {
		r.log.Errorf("Could not encode sms : %v hence dropping it", message)
		return nil
	}

	if queue {
		return r.queue.Publish(GetSmsReceiveQueueName(message.RouteID), respMessage)
	}

	r.log.Infof("Sending out MT : %s on url : %s", string(respMessage), r.settingSmsReceiveUrl)

	resp, err := http.Post(r.settingSmsReceiveUrl, "application/json", bytes.NewBuffer(respMessage))
	if err != nil {
		return err
	}

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusNonAuthoritativeInfo {
		return nil
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		} else {
			return errors.New(string(body))
		}
	}


}




func subscribeForMTEvents(r *SmppRoute) error {

	aw, _ := time.ParseDuration("60s")

	if r.receiveMessageSubscription != nil {
		err := r.receiveMessageSubscription.Unsubscribe()
		if err != nil {
			r.log.WithError(err).Warn("failed to unsubscribe for mt events")
		}
		r.receiveMessageSubscription = nil

	}

	// Async Subscriber to acknowledge messages queued on smsc
	subs, err := r.queue.QueueSubscribe(GetSmsReceiveQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {

			message := SMS{}
			err := json.Unmarshal(m.Data, &message)
			if err != nil {
				r.log.WithError(err).Errorf("error decoding message : [ %v  ] hence dropping it", m.Data)
				err = m.Ack()
				if err != nil {
					r.log.WithError(err).Error("error acknowledging message")
				}
			}

			err = r.processMTMessage(&message, false)
			if err != nil {
				r.log.WithError(err).Errorf("error occurred when posting dlr to webhook")
			} else {
				err = m.Ack()
				if err != nil {
					r.log.WithError(err).Warnf("error occurred on attempting acknowledge successful webhook")
				}
			}


		}()

	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_receive_mt", r.ID())),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(50))

	if err != nil {
		return err
	}

	r.receiveMessageSubscription = subs
	return nil
}
