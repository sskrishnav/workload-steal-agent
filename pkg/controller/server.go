package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	admission "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"workloadstealagent/pkg/validate"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecFactory  = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecFactory.UniversalDeserializer()
)

type Server interface {
	Start(stopChan chan<- bool) error
}

type Config struct {
	Port        int
	TLSKeyPath  string
	TLSCertPath string
}

type server struct {
	address   string
	config    Config
	validator validate.Validator
}

func New(config Config, validator validate.Validator) Server {
	addr := fmt.Sprintf(":%d", config.Port)
	return &server{
		address:   addr,
		config:    config,
		validator: validator,
	}
}

func (s *server) Start(stopChan chan<- bool) error {
	defer func() { stopChan <- true }()

	http.HandleFunc("/validate", s.validateHandler)
	slog.Info("Admission Webhook Server started ...")

	return http.ListenAndServeTLS(s.address, s.config.TLSCertPath, s.config.TLSKeyPath, nil)
	// Start the server in a goroutine
	// go func() {
	// 	err := http.ListenAndServeTLS(s.address, s.config.TLSCertPath, s.config.TLSKeyPath, nil)
	// 	if err != nil {
	// 		slog.Error("Failed to start admission webhook server", "error", err)
	// 		//stopChan <- true // Signal that the server has stopped
	// 	}
	// }()
	// return fmt.Errorf("exiting Admission Webhook Server")
}

func (s *server) validateHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("Validating Request")

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		slog.Error("error: unexpected content type, expected application/json", "content-type", contentType)
		msg := fmt.Sprintf("contentType=%s, expect application/json", contentType)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	if r.Body == nil {
		slog.Error("Request with no body")
		http.Error(w, "Request with no body", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Unable to read request data")
		http.Error(w, "Request with unreadable body", http.StatusBadRequest)
		return
	}

	slog.Info(fmt.Sprintf("handling request: %s", data))
	obj, gvk, err := deserializer.Decode(data, nil, nil)
	if err != nil {
		slog.Error("Request could not be decoded", "error", err)
		msg := fmt.Sprintf("Request could not be decoded: %v", err)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	requestedAdmissionReview, ok := obj.(*admission.AdmissionReview)
	if !ok {
		slog.Error("unexpected object type, expected v1.AdmissionReview", "object", obj)
		msg := fmt.Sprintf("unexpected object type, expected v1.AdmissionReview but got: %T", obj)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	responseAdmissionReview := &admission.AdmissionReview{}
	responseAdmissionReview.SetGroupVersionKind(*gvk)
	responseAdmissionReview.Response = s.validator.Validate(*requestedAdmissionReview)
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
	responseObj := responseAdmissionReview

	slog.Info(fmt.Sprintf("sending response: %v", responseObj))
	respBytes, err := json.Marshal(responseObj)
	if err != nil {
		slog.Error("response marshal failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}
