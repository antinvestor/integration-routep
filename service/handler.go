package service

import (
	"antinvestor.com/service/routep/service/sms"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/thedevsaddam/govalidator"
	"go.opentelemetry.io/otel/api/global"

	"net/http"
	"time"
)

// Logger -
func Logger(inner http.Handler, name string, logger *logrus.Entry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		inner.ServeHTTP(w, r)

		logger.Printf(
			"%s %s %s %s",
			r.Method,
			r.RequestURI,
			name,
			time.Since(start),
		)
	})
}

func addHandler(env *Env, router *mux.Router,
	f func(env *Env, w http.ResponseWriter, r *http.Request) error, path string, name string, method string) {

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		err := f(env, w, r)
		if err != nil {
			switch e := err.(type) {
			case Error:
				// We can retrieve the status here and write out a specific
				// HTTP status code.
				env.Logger.WithError(e).Warnf("request failed with  %d - %q", e.Status(), e)
				http.Error(w, e.Error(), e.Status())
			default:

				env.Logger.WithError(e).Warn("request is in error")
				// Any error types we don't specifically look out for default
				// to serving a HTTP 500
				http.Error(w, http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError)
			}
		}

	})
	loggedHandler := Logger(handler, name, env.Logger)

	router.Methods(method).Path(path).Name(name).Handler(limit(loggedHandler))

}

// NewRouter -
func NewRouter(env *Env) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	addHandler(env, router, SendSms, "/", "SendSms", "POST")
	addHandler(env, router, Healthz, "/healthz", "Healthz", "GET")

	return router
}

// SendSms -
func SendSms(env *Env, w http.ResponseWriter, r *http.Request) error {

	tracer := global.Tracer(env.ServiceName)
	span, _ := tracer.Start(r.Context(), "SendSms")
	defer span.Done()

	rules := govalidator.MapData{
		"from":       []string{"required", "max:20"},
		"to":         []string{"required", "digits_between:12,14"},
		"data":       []string{"required", "max:1000"},
		"message_id": []string{"required", "max:30"},
		"route_id":   []string{"required", "max:30"},
	}

	messages := govalidator.MapData{
		"to":         []string{"required: A phone number is required", "digits:Give a valid MSISDN e.g. 254723549100"},
		"from":       []string{"required: Sender of message is required", "max:The maximum size of sender is 20 chars long"},
		"data":       []string{"required: A message to send to the receiver is required", "max:The maximum size of message is 1000 chars long"},
		"message_id": []string{"required: What is the reference id for this message?"},
		"route_id":   []string{"required:What is the route to use for this message?"},
	}

	opts := govalidator.Options{
		Request:         r,        // request object
		Rules:           rules,    // rules map
		Messages:        messages, // custom message map (Optional)
		RequiredDefault: true,     // all the field to be pass the rules
	}
	validator := govalidator.New(opts)

	e := validator.Validate()
	if len(e) != 0 {
		err, _ := json.Marshal(e)
		return StatusError{400, errors.New(string(err))}
	}

	messageMO := sms.SMS{
		From:      r.FormValue("from"),
		To:        r.FormValue("to"),
		Data:      r.FormValue("data"),
		MessageID: r.FormValue("message_id"),
		RouteID:   r.FormValue("route_id"),
	}

	smsRoute := env.SMSServer.GetRoute(messageMO.RouteID)
	if smsRoute == nil {
		return StatusError{500, errors.New("No active routes were found")}
	}

	ack, err := smsRoute.SendMOMessage(&messageMO)
	if err != nil {
		return StatusError{500, err}
	}

	message := []byte("Queued")
	if ack != nil {

		message, err = json.Marshal(ack)
		if err != nil {
			return StatusError{500, err}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(message)
	return nil

}

// Healthz -
func Healthz(env *Env, w http.ResponseWriter, r *http.Request) error {

	tracer := global.Tracer(env.ServiceName)
	span, _ := tracer.Start(r.Context(), "healthz")
	defer span.Done()

	msg := "ok"
	statusCode := http.StatusOK
	if !env.SMSServer.IsActive() {
		msg = "failed"
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(statusCode)
	w.Write([]byte(msg))
	return nil

}
