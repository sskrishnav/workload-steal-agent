#!/bin/bash

echo "Creating certificates"
mkdir certs
openssl genrsa -out certs/tls.key 2048
openssl req -new -key certs/tls.key -out certs/tls.csr -subj "/CN=webhook-server.myorgops.svc"
echo "subjectAltName=DNS:webhook-server.hiro.svc" > ./subj.txt

openssl x509 -req -extfile subj.txt -in certs/tls.csr -signkey certs/tls.key -out certs/tls.crt

echo "Creating Webhook Server TLS Secret in Kubernetes"
kubectl create secret tls webhook-server-tls \
    --cert "certs/tls.crt" \
    --key "certs/tls.key" -n hiro

echo "Deploying Webhook Server"
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml

echo "Creating K8s Webhooks"
ENCODED_CA=$(cat certs/tls.crt | base64 | tr -d '\n')
# sed -e 's@${ENCODED_CA}@'"$ENCODED_CA"'@g' <"manifests/webhooks.yml" | kubectl create -f -
sed -e "s/<ENCODED_CA>/${ENCODED_CA}/g" <"webhook.yml" | kubectl create -f -
