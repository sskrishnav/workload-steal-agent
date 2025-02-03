#!/bin/bash

echo "Build Docker image"
docker build -t workloadstealagent:alpha1 .

echo "Set the kubectl context to remote cluster"
kubectl cluster-info --context kind-remote
kubectl config use-context kind-remote

echo "Load Image to Kind cluster named 'remote'"
kind load docker-image --name remote workloadstealagent:alpha1

echo "Restarting Agent Deployment"
kubectl rollout restart deployment mutation-webhook-server -n hiro