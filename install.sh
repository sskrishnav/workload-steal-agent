#!/bin/bash

echo "Build Docker image"
docker build -t workloadstealagent:alpha1 .

echo "Set the kubectl context to remote cluster"
kubectl cluster-info --context kind-remote

echo "Load Image to Kind cluster named 'remote'"
kind load docker-image --name remote workloadstealagent:alpha1

echo "Create 'hiro' namespace if it doesn't exist"
kubectl get namespace | grep -q "hiro" || kubectl create namespace hiro

echo "Creating certificates"
mkdir certs

# openssl genrsa -out certs/tls.key 2048
# openssl req -new -key certs/tls.key -out certs/tls.csr -subj "/CN=validation-webhook-server.hiro.svc"
# echo "subjectAltName=DNS:validation-webhook-server.hiro.svc" > ./subj.txt
# openssl x509 -req -extfile subj.txt -in certs/tls.csr -signkey certs/tls.key -out certs/tls.crt

# echo "Creating Validation Webhook Server TLS Secret in Kubernetes"
# kubectl create secret tls validation-webhook-server-tls \
#     --cert "certs/tls.crt" \
#     --key "certs/tls.key" -n hiro

openssl genrsa -out certs/tls.key 2048
openssl req -new -key certs/tls.key -out certs/tls.csr -subj "/CN=mutation-webhook-server.hiro.svc"
echo "subjectAltName=DNS:mutation-webhook-server.hiro.svc" > ./subj.txt
openssl x509 -req -extfile subj.txt -in certs/tls.csr -signkey certs/tls.key -out certs/tls.crt

echo "Creating Mutation Webhook Server TLS Secret in Kubernetes"
kubectl create secret tls mutation-webhook-server-tls \
    --cert "certs/tls.crt" \
    --key "certs/tls.key" -n hiro

# openssl genrsa -out certs/tls.key 2048
# openssl req -new -key certs/tls.key -out certs/tls.csr -subj "/CN=webhook-server.hiro.svc"
# echo "subjectAltName=DNS:webhook-server.hiro.svc" > ./subj.txt
# openssl x509 -req -extfile subj.txt -in certs/tls.csr -signkey certs/tls.key -out certs/tls.crt

# echo "Creating Validation and Mutation Webhook Server TLS Secret in Kubernetes"
# kubectl create secret tls webhook-server-tls \
#     --cert "certs/tls.crt" \
#     --key "certs/tls.key" -n hiro

echo "Deploying Webhook Server"
kubectl apply -f deploy/deployment.yaml
kubectl apply -f deploy/service.yaml

echo "Creating K8s Webhooks"
ENCODED_CA=$(cat certs/tls.crt | base64 | tr -d '\n')
# sed -e 's@${ENCODED_CA}@'"$ENCODED_CA"'@g' <"manifests/webhooks.yml" | kubectl create -f -
sed -e "s/<ENCODED_CA>/${ENCODED_CA}/g" <"deploy/webhook.yaml" | kubectl apply -f -
