package sms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fiorix/go-smpp/smpp/pdu/pdufield"
	"github.com/nats-io/stan.go"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"
)

func (r *SmppRoute) processDLRMessage(dlr *DLR, queue bool) error {

	dlrMessage, err := json.Marshal(dlr)
	if err != nil {
		r.log.Errorf("Could not encode sms : %v hence dropping it", dlr)
		return nil
	}

	if queue {

		err := r.queue.Publish(GetSmsSendDLRQueueName(dlr.RouteID), dlrMessage)
		if err != nil {
			r.log.WithError(err).Errorf("unable to queue DLR %v hence just lost it", dlr)
		}
		return nil
	}

	r.log.Infof("Sending out DLR : %s on url : %s", string(dlrMessage), r.settingSmsSendDLRUrl)

	resp, err := http.Post(r.settingSmsSendDLRUrl, "application/json", bytes.NewBuffer(dlrMessage))
	if err != nil {
		r.log.WithError(err).Warnf("error occurred on attempting dlr webhook")
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

func subscribeForDLREvents(r *SmppRoute) error {

	aw, _ := time.ParseDuration("60s")

	if r.receiveDLRSubscription != nil {
		err := r.receiveDLRSubscription.Unsubscribe()
		if err != nil {
			r.log.WithError(err).Warn("failed to unsubscribe for dlr events")
		}
		r.receiveDLRSubscription = nil
	}

	// Async Subscriber to acknowledge messages queued on smsc
	subs, err := r.queue.QueueSubscribe(GetSmsSendDLRQueueName(r.ID()), GetQueueGroup(r.ID()), func(m *stan.Msg) {

		go func() {

			messageDlr := DLR{}
			err := json.Unmarshal(m.Data, &messageDlr)
			if err != nil {
				r.log.WithError(err).Errorf("error decoding message : [ %v  ] hence dropping it", m.Data)
				err = m.Ack()
				if err != nil {
					r.log.WithError(err).Error("error acknowledging message")
				}
			}

			err = r.processDLRMessage(&messageDlr, false)
			if err != nil {
				r.log.WithError(err).Errorf("error occurred when posting dlr to webhook")
			} else {
				err = m.Ack()
				if err != nil {
					r.log.WithError(err).Warnf("error occurred on attempting acknowledge successful webhook")
				}
			}

		}()

	}, stan.StartWithLastReceived(), stan.DurableName(fmt.Sprintf("%s_receive_dlr", r.ID())),
		stan.SetManualAckMode(), stan.AckWait(aw), stan.MaxInflight(50))

	if err != nil {
		return err
	}

	r.receiveDLRSubscription = subs

	return nil
}
