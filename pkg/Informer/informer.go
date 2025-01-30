package informer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	nats "github.com/nats-io/nats.go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Config struct {
	NATSURL     string
	NATSSubject string
}

type notify struct {
	nconfig Config
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

func New(config Config) (*notify, error) {
	clientset, err := getClientSet()
	if err != nil {
		return nil, err
	}
	return &notify{
		cli:     clientset,
		nconfig: config,
	}, nil
}

func (n *notify) Start(stopChan chan<- bool) error {
	defer func() { stopChan <- true }()

	// Configure structured logging with slog
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Connect to NATS server
	natsConnect, err := nats.Connect(n.nconfig.NATSURL)
	if err != nil {
		slog.Error("Failed to connect to NATS server: ", "error", err)
		return err
	}
	defer natsConnect.Close()
	slog.Info("Connected to NATS server")

	fmt.Println("Watching for Pod events")
	watch, err := n.cli.CoreV1().Pods("").Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to watch pods: ", "error", err)
		return err
	}
	defer watch.Stop()

	slog.Info("Listening for Pod creation events...")

	for event := range watch.ResultChan() {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			continue
		}

		if event.Type == "ADDED" {
			slog.Info("Pod create event: %s/%s\n", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

			// Serialize the entire Pod metadata to JSON
			metadataJSON, err := json.Marshal(pod.ObjectMeta)
			if err != nil {
				slog.Error("Failed to serialize Pod metadata", "error", err)
				continue
			}

			// Publish notification to NATS
			err = natsConnect.Publish(n.nconfig.NATSSubject, metadataJSON)
			if err != nil {
				slog.Error("Failed to publish message to NATS", "error", err, "subject", n.nconfig.NATSSubject)
				continue
			}
			slog.Info("Published Pod metadata to NATS", "subject", n.nconfig.NATSSubject, "metadata", string(metadataJSON))
		}
	}
	return nil
}
