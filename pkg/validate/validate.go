package validate

import (
	"fmt"
	"log/slog"
	"strings"

	admission "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecFactory  = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecFactory.UniversalDeserializer()
)

type Config struct {
	LableToFilter string
}

type Validator interface {
	Validate(admission.AdmissionReview) *admission.AdmissionResponse
}

type validator struct {
	vconfig Config
	cli     *kubernetes.Clientset
}

func getClientSet() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, err
}

func New(config Config) (Validator, error) {
	clientset, err := getClientSet()
	if err != nil {
		return nil, err
	}
	return &validator{
		cli:     clientset,
		vconfig: config,
	}, nil
}

func (v *validator) Validate(ar admission.AdmissionReview) *admission.AdmissionResponse {
	slog.Info("Make deplopoyment Invalid")
	deploymentResource := metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deplopoyments"}
	if ar.Request.Resource != deploymentResource {
		slog.Error(fmt.Sprintf("expect resource to be %s", deploymentResource))
		return nil
	}
	raw := ar.Request.Object.Raw
	deployment := appsv1.Deployment{}
	if _, _, err := deserializer.Decode(raw, nil, &deployment); err != nil {
		slog.Error("failed to decode deployment", "error", err)
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}
	isEligibleToSteal := isLableExists(&deployment, v.vconfig.LableToFilter)
	if !isEligibleToSteal {
		return &admission.AdmissionResponse{Allowed: true}
	}
	message := fmt.Sprintf("deployment %s was stolen", &deployment)
	return &admission.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Message: message,
		},
		UID: ar.Request.UID,
	}
}

func isLableExists(deployment *appsv1.Deployment, lable string) bool {
	value, ok := deployment.Labels[lable]
	if !ok || strings.ToLower(value) == "false" {
		return false
	}
	return true
}
