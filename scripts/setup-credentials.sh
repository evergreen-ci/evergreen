#!/bin/bash

(
    for cluster in "staging" "prod"; do
        export KUBECONFIG=~/.kube/config.$cluster
        kanopy-oidc kube setup $cluster > ~/.kube/config.$cluster
        kanopy-oidc kube login
        kubectl config set-context $(kubectl config current-context) --namespace=devprod-evergreen
    done
)
