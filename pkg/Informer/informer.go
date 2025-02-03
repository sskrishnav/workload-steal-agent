package informer

import (
	"context"
	"encoding/json"
	"log/slog"

	nats "github.com/nats-io/nats.go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var K8SNamespaces = []string{"default", "kube-system"}

type NATSConfig struct {
	NATSURL     string
	NATSSubject string
}

type Config struct {
	Nconfig          NATSConfig
	IgnoreNamespaces []string
}

type notify struct {
	config Config
	cli    *kubernetes.Clientset
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

func New(config Config) (*notify, error) {
	clientset, err := getClientSet()
	if err != nil {
		return nil, err
	}
	return &notify{
		cli:    clientset,
		config: config,
	}, nil
}

func (n *notify) Start(stopChan chan<- bool) error {
	defer func() { stopChan <- true }()

	nsubject := n.config.Nconfig.NATSSubject
	nurl := n.config.Nconfig.NATSURL
	inamespace := n.config.IgnoreNamespaces
	// Connect to NATS server
	natsConnect, err := nats.Connect(nurl)
	if err != nil {
		slog.Error("Failed to connect to NATS server: ", "error", err)
		return err
	}
	defer natsConnect.Close()
	slog.Info("Connected to NATS server")

	slog.Info("Watching for Pod events")
	watch, err := n.cli.CoreV1().Pods("").Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to watch pods: ", "error", err)
		return err
	}
	defer watch.Stop()

	slog.Info("Listening for Pod creation events...")
	for event := range watch.ResultChan() {
		slog.Info("Received", "event", event)
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}

		if event.Type == "ADDED" {
			slog.Info("Pod create event", "namespace", pod.Namespace, "name", pod.Name)
			if contains(inamespace, pod.Namespace) {
				slog.Info("Ignoring as Pod belongs to Ignored Namespaces", "namespaces", inamespace)
				continue
			}

			// Serialize the entire Pod metadata to JSON
			metadataJSON, err := json.Marshal(pod)
			if err != nil {
				slog.Error("Failed to serialize Pod metadata", "error", err)
				continue
			}

			// Publish notification to NATS
			err = natsConnect.Publish(nsubject, metadataJSON)
			if err != nil {
				slog.Error("Failed to publish message to NATS", "error", err, "subject", nsubject)
				continue
			}
			slog.Info("Published Pod metadata to NATS", "subject", nsubject, "metadata", string(metadataJSON))
		}
	}
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
