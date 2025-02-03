#!/bin/bash

echo "Delete and Create a 'kind' cluster with name 'remote'"
kind delete cluster --name remote
kind create cluster --name remote

echo "Set the kubectl context to remote cluster"
kubectl cluster-info --context kind-remote
kubectl config use-context kind-remote
