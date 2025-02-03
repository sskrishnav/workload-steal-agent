package validate

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mattbaird/jsonpatch"
	"github.com/nats-io/nats.go"
	admission "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
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
	K8SNamespaces = []string{"default", "kube-system", "kube-public",
		"kube-node-lease", "kube-admission", "kube-proxy", "kube-controller-manager",
		"kube-scheduler", "kube-dns"}
)

type NATSConfig struct {
	NATSURL     string
	NATSSubject string
}

type Config struct {
	Nconfig          NATSConfig
	LableToFilter    string
	IgnoreNamespaces []string
}

type Validator interface {
	Validate(admission.AdmissionReview) *admission.AdmissionResponse
	Mutate(admission.AdmissionReview) *admission.AdmissionResponse
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
	slog.Info("No Validation for now")
	return &admission.AdmissionResponse{Allowed: true}
}

func (v *validator) Mutate(ar admission.AdmissionReview) *admission.AdmissionResponse {
	slog.Info("Make pod Invalid")
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		slog.Error("expect resource does not match", "expected", ar.Request.Resource, "received", podResource)
		return nil
	}
	if ar.Request.Operation != admission.Create {
		slog.Error("expect operation does not match", "expected", admission.Create, "received", ar.Request.Operation)
		return nil
	}
	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		slog.Error("failed to decode pod", "error", err)
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}
	isNotEligibleToSteal := isLableExists(&pod, v.vconfig.LableToFilter)
	if isNotEligibleToSteal {
		slog.Info("Pod is not eligible to steal as it has the label", "name", pod.Name, "label", v.vconfig.LableToFilter)
		return &admission.AdmissionResponse{Allowed: true}
	}
	inamespaces := v.vconfig.IgnoreNamespaces
	slog.Info("Pod create event", "namespace", pod.Namespace, "name", pod.Name)
	if contains(inamespaces, pod.Namespace) {
		slog.Info("Ignoring as Pod belongs to Ignored Namespaces", "namespaces", inamespaces)
		return nil
	}

	// Make the pod to be stolen
	go func() {
		Inform(&pod, v.vconfig.Nconfig.NATSSubject, v.vconfig.Nconfig.NATSURL)
	}()

	return mutatePod(&pod)
}

func isLableExists(pod *corev1.Pod, lable string) bool {
	value, ok := pod.Labels[lable]
	if !ok || strings.ToLower(value) == "false" {
		return false
	}
	return true
}

func Inform(pod *corev1.Pod, nsubject string, nurl string) error {
	// Connect to NATS server
	natsConnect, err := nats.Connect(nurl)
	if err != nil {
		slog.Error("Failed to connect to NATS server: ", "error", err)
		return err
	}
	defer natsConnect.Close()
	slog.Info("Connected to NATS server")

	// Serialize the entire Pod metadata to JSON
	metadataJSON, err := json.Marshal(pod)
	if err != nil {
		slog.Error("Failed to serialize Pod metadata", "error", err)
		return err
	}

	// Publish notification to NATS
	err = natsConnect.Publish(nsubject, metadataJSON)
	if err != nil {
		slog.Error("Failed to publish message to NATS", "error", err, "subject", nsubject)
		return err
	}
	slog.Info("Published Pod metadata to NATS", "subject", nsubject, "metadata", string(metadataJSON))
	return nil
}

func contains(list []string, item string) bool {
	for _, str := range mergeUnique(list, K8SNamespaces) {
		if str == item {
			return true
		}
	}
	return false
}

func mergeUnique(slice1, slice2 []string) []string {
	uniqueMap := make(map[string]bool)
	result := []string{}

	for _, item := range slice1 {
		uniqueMap[item] = true
	}
	for _, item := range slice2 {
		uniqueMap[item] = true
	}

	for key := range uniqueMap {
		result = append(result, key)
	}

	return result
}

func mutatePod(pod *corev1.Pod) *admission.AdmissionResponse {
	originalPod := pod.DeepCopy()
	modifiedPod := originalPod.DeepCopy()

	// // Replace all containers with our dummy container
	// for i := range modifiedPod.Spec.Containers {
	// 	container := &modifiedPod.Spec.Containers[i]
	// 	container.Image = "busybox"
	// 	container.Command = []string{
	// 		"/bin/sh",
	// 		"-c",
	// 		"echo 'Pod got stolen' && sleep infinity",
	// 	}
	// }
	modifiedPod.Spec.NodeSelector = map[string]string{
		"node-stolen": "true",
		"node-id":     "7388q9y8989qwyehadsbdf",
	}
	modifiedPod.Labels["pod-stolen"] = "true"

	// Marshal the modified pod to JSON
	originalJSON, err := json.Marshal(originalPod)
	if err != nil {
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: fmt.Sprintf("Failed to marshal original pod: %v", err),
				Code:    http.StatusInternalServerError,
			},
		}
	}

	// Marshal the modified pod to JSON
	modifiedJSON, err := json.Marshal(modifiedPod)
	if err != nil {
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: fmt.Sprintf("Failed to marshal modified pod: %v", err),
				Code:    http.StatusInternalServerError,
			},
		}
	}

	// Create JSON Patch
	patch, err := jsonpatch.CreatePatch(originalJSON, modifiedJSON)
	if err != nil {
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: fmt.Sprintf("Failed to create patch: %v", err),
				Code:    http.StatusInternalServerError,
			},
		}
	}

	// Marshal the patch to JSON
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return &admission.AdmissionResponse{
			Result: &metav1.Status{
				Message: fmt.Sprintf("Failed to marshal patch: %v", err),
				Code:    http.StatusInternalServerError,
			},
		}
	}

	// Return the AdmissionResponse with the mutated Pod
	return &admission.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admission.PatchType {
			pt := admission.PatchTypeJSONPatch
			return &pt
		}(),
	}
}
